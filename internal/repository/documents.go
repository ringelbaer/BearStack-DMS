// Datei enthaelt zentrale Schreib- und Leseoperationen fuer Dokumentdatensaetze.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"bearstack/internal/document"
)

var ErrDeleteProtected = errors.New("document is delete protected")

const countDocumentFiltersBatchSize = 50

func replaceSearchIndex(ctx context.Context, tx *sql.Tx, docID int64, doc document.Document) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_search WHERE rowid = ?`, docID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO document_search(rowid, original_name, title, description, tags, search_text)
		VALUES (?, ?, ?, ?, ?, ?)`,
		docID,
		doc.OriginalName,
		doc.Title,
		doc.Description,
		strings.Join(doc.Tags, " "),
		searchIndexTextFor(doc),
	)
	return err
}

func updateSearchIndexTagsTx(ctx context.Context, tx *sql.Tx, docID int64, tags []string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE document_search
		SET tags = ?
		WHERE rowid = ?`,
		strings.Join(tags, " "),
		docID,
	)
	return err
}

func (r *Repository) CreateDocument(ctx context.Context, doc document.Document) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if doc.UploadedAt.IsZero() {
		doc.UploadedAt = now
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = now
	}
	if doc.SearchVersion == 0 {
		doc.SearchVersion = document.CurrentSearchVersion
	}
	doc.UploadWay = document.NormalizeUploadWay(doc.UploadWay)
	doc.ContentTextSource = document.NormalizeContentTextSource(doc.ContentTextSource, doc.ContentText)

	var documentDate any
	if doc.DocumentDate != nil {
		documentDate = doc.DocumentDate.Format("2006-01-02")
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO documents (
			original_name, stored_path, thumbnail_path, upload_way, title, description, mime_type,
			size_bytes, sha256, document_date, uploaded_at, updated_at, content_text,
			content_text_source, search_version, post_import_pending
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		doc.OriginalName,
		doc.StoredPath,
		doc.ThumbnailPath,
		doc.UploadWay,
		doc.Title,
		doc.Description,
		doc.MIMEType,
		doc.SizeBytes,
		doc.SHA256,
		documentDate,
		formatTime(doc.UploadedAt),
		formatTime(doc.UpdatedAt),
		doc.ContentText,
		doc.ContentTextSource,
		doc.SearchVersion,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	doc.ID = id
	doc.Tags, err = autoTagValuesForNewDocumentTx(ctx, tx, doc)
	if err != nil {
		return 0, err
	}
	if err := replaceTags(ctx, tx, id, doc.Tags); err != nil {
		return 0, err
	}
	if err := replaceSearchIndex(ctx, tx, id, doc); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) GetDocument(ctx context.Context, id int64) (document.Document, error) {
	rows, err := r.db.QueryContext(ctx, baseSelect()+` WHERE d.id = ?`, id)
	if err != nil {
		return document.Document{}, err
	}
	defer rows.Close()

	docs, err := scanDocuments(rows)
	if err != nil {
		return document.Document{}, err
	}
	if len(docs) == 0 {
		return document.Document{}, sql.ErrNoRows
	}
	if err := r.attachTags(ctx, docs); err != nil {
		return document.Document{}, err
	}
	if err := r.attachCustomValues(ctx, docs); err != nil {
		return document.Document{}, err
	}
	return docs[0], nil
}

func (r *Repository) FindActiveByChecksum(ctx context.Context, checksum string) (document.Document, bool, error) {
	rows, err := r.db.QueryContext(ctx, summarySelect()+` WHERE d.sha256 = ? AND d.deleted_at IS NULL ORDER BY d.uploaded_at ASC LIMIT 1`, checksum)
	if err != nil {
		return document.Document{}, false, err
	}
	defer rows.Close()

	docs, err := scanSummaryDocuments(rows)
	if err != nil {
		return document.Document{}, false, err
	}
	if len(docs) == 0 {
		return document.Document{}, false, nil
	}
	if err := r.attachSummaryDetails(ctx, docs); err != nil {
		return document.Document{}, false, err
	}
	return docs[0], true, nil
}

func (r *Repository) ListDocuments(ctx context.Context, filter document.ListFilter) ([]document.Document, error) {
	query, args := buildListQuery(filter)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs, err := scanSummaryDocuments(rows)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return docs, nil
	}
	if err := r.attachSummaryDetails(ctx, docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *Repository) CountDocuments(ctx context.Context, filter document.ListFilter) (int, error) {
	query, args := buildCountQuery(filter)
	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *Repository) CountDocumentFilters(ctx context.Context, filters []document.ListFilter) ([]int, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	counts := make([]int, len(filters))
	for start := 0; start < len(filters); start += countDocumentFiltersBatchSize {
		end := min(start+countDocumentFiltersBatchSize, len(filters))
		batchCounts, err := r.countDocumentFiltersBatch(ctx, filters[start:end])
		if err != nil {
			return nil, err
		}
		copy(counts[start:end], batchCounts)
	}
	return counts, nil
}

func (r *Repository) countDocumentFiltersBatch(ctx context.Context, filters []document.ListFilter) ([]int, error) {
	var query strings.Builder
	args := make([]any, 0, len(filters)*4)
	for i, filter := range filters {
		if i > 0 {
			query.WriteString("\nUNION ALL\n")
		}
		countQuery, countArgs := buildCountQuery(filter)
		query.WriteString("SELECT ? AS filter_index, (")
		query.WriteString(countQuery)
		query.WriteString(") AS count")
		args = append(args, i)
		args = append(args, countArgs...)
	}

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make([]int, len(filters))
	for rows.Next() {
		var index int
		var count int
		if err := rows.Scan(&index, &count); err != nil {
			return nil, err
		}
		if index < 0 || index >= len(counts) {
			return nil, fmt.Errorf("count query returned invalid filter index %d", index)
		}
		counts[index] = count
	}
	return counts, rows.Err()
}

func (r *Repository) DocumentDateYears(ctx context.Context, trash bool) ([]int, error) {
	deletedWhere := "deleted_at IS NULL"
	if trash {
		deletedWhere = "deleted_at IS NOT NULL"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(substr(document_date, 1, 4) AS INTEGER) AS year
		FROM documents
		WHERE `+deletedWhere+`
		  AND document_date IS NOT NULL
		  AND length(document_date) >= 4
		ORDER BY year DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var years []int
	for rows.Next() {
		var year int
		if err := rows.Scan(&year); err != nil {
			return nil, err
		}
		if year > 0 {
			years = append(years, year)
		}
	}
	return years, rows.Err()
}

func (r *Repository) DocumentDateMonths(ctx context.Context, trash bool, year int) ([]int, error) {
	if year <= 0 {
		return nil, nil
	}
	deletedWhere := "deleted_at IS NULL"
	if trash {
		deletedWhere = "deleted_at IS NOT NULL"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(substr(document_date, 6, 2) AS INTEGER) AS month
		FROM documents
		WHERE `+deletedWhere+`
		  AND document_date IS NOT NULL
		  AND length(document_date) >= 7
		  AND document_date >= ?
		  AND document_date < ?
		ORDER BY month ASC`,
		fmt.Sprintf("%04d-01-01", year),
		fmt.Sprintf("%04d-01-01", year+1),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []int
	for rows.Next() {
		var month int
		if err := rows.Scan(&month); err != nil {
			return nil, err
		}
		if month >= 1 && month <= 12 {
			months = append(months, month)
		}
	}
	return months, rows.Err()
}

func (r *Repository) ListByIDs(ctx context.Context, ids []int64) ([]document.Document, error) {
	if len(ids) == 0 {
		return []document.Document{}, nil
	}

	placeholders, args := int64IDPlaceholders(ids)
	rows, err := r.db.QueryContext(ctx, summarySelect()+`
		WHERE d.id IN (`+placeholders+`) AND d.deleted_at IS NULL
		ORDER BY d.uploaded_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs, err := scanSummaryDocuments(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachSummaryDetails(ctx, docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *Repository) attachSummaryDetails(ctx context.Context, docs []document.Document) error {
	if err := r.attachSummaryCounts(ctx, docs); err != nil {
		return err
	}
	if err := r.attachTags(ctx, docs); err != nil {
		return err
	}
	return r.attachCustomValues(ctx, docs)
}

func (r *Repository) UpdateMetadata(ctx context.Context, id int64, title, description string, documentDate *time.Time, tags []string, customValues map[int64]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	current, err := getDocumentTx(ctx, tx, id)
	if err != nil {
		return err
	}
	current = documentWithMetadataInput(current, title, description, documentDate, customValues)
	if err := updateDocumentMetadataTx(ctx, tx, current); err != nil {
		return err
	}
	if err := replaceTags(ctx, tx, id, tags); err != nil {
		return err
	}
	if err := replaceCustomValues(ctx, tx, id, current.CustomValues); err != nil {
		return err
	}
	current.Tags, err = tagsForDocumentTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := replaceSearchIndex(ctx, tx, id, current); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) AddTagsToDocuments(ctx context.Context, ids []int64, tags []string) (int, error) {
	tags = cleanTagNames(tags)
	if len(ids) == 0 || len(tags) == 0 {
		return 0, nil
	}
	ids = uniqueInt64(ids)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	docs, err := summaryDocumentsByIDTx(ctx, tx, ids)
	if err != nil {
		return 0, err
	}

	tagIDs, err := ensureTagIDsTx(ctx, tx, tags)
	if err != nil {
		return 0, err
	}

	now := formatTime(time.Now().UTC())
	activeIDs := activeDocumentIDsForTagAdd(docs, ids, tags)
	if len(activeIDs) == 0 {
		return 0, tx.Commit()
	}
	if err := insertDocumentTagsTx(ctx, tx, activeIDs, tagIDs); err != nil {
		return 0, err
	}
	if err := touchAndUpdateSearchTagsTx(ctx, tx, docs, activeIDs, now); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(activeIDs), nil
}

func (r *Repository) RemoveTagsFromDocuments(ctx context.Context, ids []int64, tags []string) (int, error) {
	tags = cleanTagNames(tags)
	if len(ids) == 0 || len(tags) == 0 {
		return 0, nil
	}
	ids = uniqueInt64(ids)

	remove := tagRemovalSet(tags)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	docs, err := summaryDocumentsByIDTx(ctx, tx, ids)
	if err != nil {
		return 0, err
	}
	tagIDs, err := tagIDsByNameTx(ctx, tx, tags)
	if err != nil {
		return 0, err
	}
	if len(tagIDs) == 0 {
		return 0, tx.Commit()
	}

	now := formatTime(time.Now().UTC())
	changedIDs := changedDocumentIDsForTagRemoval(docs, ids, remove)
	if len(changedIDs) == 0 {
		return 0, tx.Commit()
	}
	if err := deleteDocumentTagsTx(ctx, tx, changedIDs, tagIDs); err != nil {
		return 0, err
	}
	if err := touchAndUpdateSearchTagsTx(ctx, tx, docs, changedIDs, now); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(changedIDs), nil
}

func (r *Repository) UpdateDocumentDate(ctx context.Context, id int64, documentDate *time.Time) error {
	var documentDateValue any
	if documentDate != nil {
		documentDateValue = documentDate.Format("2006-01-02")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET document_date = ?, updated_at = ?
		WHERE id = ?`,
		documentDateValue,
		formatTime(time.Now().UTC()),
		id,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) SoftDelete(ctx context.Context, id int64) error {
	protected, err := r.DocumentHasDeleteProtectedTag(ctx, id)
	if err != nil {
		return err
	}
	if protected {
		return ErrDeleteProtected
	}
	now := formatTime(time.Now().UTC())
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) Restore(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET deleted_at = NULL, updated_at = ?
		WHERE id = ? AND deleted_at IS NOT NULL`, now, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DocumentHasDeleteProtectedTag(ctx context.Context, id int64) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM document_tags dt
		JOIN tags t ON t.id = dt.tag_id
		WHERE dt.document_id = ?
		  AND t.delete_protected = 1
		LIMIT 1`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) Purge(ctx context.Context, id int64) (document.Document, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return document.Document{}, err
	}
	defer tx.Rollback()

	doc, err := getDocumentTx(ctx, tx, id)
	if err != nil {
		return document.Document{}, err
	}
	if doc.DeletedAt == nil {
		return document.Document{}, errors.New("document must be in trash before purge")
	}
	protected, err := documentHasDeleteProtectedTagTx(ctx, tx, id)
	if err != nil {
		return document.Document{}, err
	}
	if protected {
		return document.Document{}, ErrDeleteProtected
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_tags WHERE document_id = ?`, id); err != nil {
		return document.Document{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_search WHERE rowid = ?`, id); err != nil {
		return document.Document{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id)
	if err != nil {
		return document.Document{}, err
	}
	if err := requireAffected(result); err != nil {
		return document.Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return document.Document{}, err
	}
	return doc, nil
}

func (r *Repository) PurgeTrash(ctx context.Context) ([]document.Document, error) {
	return r.purgeTrash(ctx, nil)
}

func (r *Repository) PurgeTrashBefore(ctx context.Context, cutoff time.Time) ([]document.Document, error) {
	return r.purgeTrash(ctx, &cutoff)
}

func (r *Repository) purgeTrash(ctx context.Context, cutoff *time.Time) ([]document.Document, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	where := []string{
		"d.deleted_at IS NOT NULL",
		`NOT EXISTS (
			SELECT 1
			FROM document_tags pdt
			JOIN tags pt ON pt.id = pdt.tag_id
			WHERE pdt.document_id = d.id
			  AND pt.delete_protected = 1
		)`,
	}
	var args []any
	if cutoff != nil {
		where = append(where, "d.deleted_at <= ?")
		args = append(args, formatTime(*cutoff))
	}

	rows, err := tx.QueryContext(ctx, summarySelect()+`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY d.deleted_at ASC, d.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	docs, err := scanSummaryDocuments(rows)
	if closeErr := rows.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return docs, nil
	}

	if err := forDocumentBatches(docs, func(inClause string, args []any) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM document_tags WHERE document_id IN (`+inClause+`)`, args...); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM document_search WHERE rowid IN (`+inClause+`)`, args...); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE id IN (`+inClause+`)`, args...)
		if err != nil {
			return err
		}
		if err := requireAffected(result); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return docs, nil
}
