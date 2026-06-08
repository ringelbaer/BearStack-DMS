// Datei mappt SQL-Ergebniszeilen auf Dokumentmodelle und gemeinsame Row-Scanner.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

const repositoryBatchSize = 500

func baseSelect() string {
	return documentSelect("d.content_text", true)
}

func summarySelect() string {
	return documentSelect("''", false)
}

func documentSelect(contentTextExpr string, includeCounts bool) string {
	counts := `0 AS duplicate_count, 0 AS linked_count`
	if includeCounts {
		counts = `
				(
					SELECT COUNT(*)
					FROM documents dup
					WHERE dup.sha256 = d.sha256
					  AND dup.id != d.id
					  AND dup.deleted_at IS NULL
				) AS duplicate_count,
				(
					SELECT COUNT(*)
					FROM document_links dl
					WHERE dl.source_document_id = d.id
					   OR dl.target_document_id = d.id
				) AS linked_count`
	}
	return `
			SELECT
				d.id, d.original_name, d.stored_path, d.thumbnail_path, d.upload_way, d.title, d.description,
				d.mime_type, d.size_bytes, d.sha256, d.document_date,
				d.uploaded_at, d.updated_at, d.deleted_at, ` + contentTextExpr + ` AS content_text, d.content_text_source,
				d.search_version,
				` + counts + `
				FROM documents d`
}

func scanDocuments(rows *sql.Rows) ([]document.Document, error) {
	return scanDocumentRows(rows, true)
}

func scanSummaryDocuments(rows *sql.Rows) ([]document.Document, error) {
	return scanDocumentRows(rows, false)
}

func scanDocumentRows(rows *sql.Rows, contentLoaded bool) ([]document.Document, error) {
	var docs []document.Document
	for rows.Next() {
		var doc document.Document
		var documentDate, deletedAt sql.NullString
		var uploadedAt, updatedAt string

		if err := rows.Scan(
			&doc.ID,
			&doc.OriginalName,
			&doc.StoredPath,
			&doc.ThumbnailPath,
			&doc.UploadWay,
			&doc.Title,
			&doc.Description,
			&doc.MIMEType,
			&doc.SizeBytes,
			&doc.SHA256,
			&documentDate,
			&uploadedAt,
			&updatedAt,
			&deletedAt,
			&doc.ContentText,
			&doc.ContentTextSource,
			&doc.SearchVersion,
			&doc.DuplicateCount,
			&doc.LinkedCount,
		); err != nil {
			return nil, err
		}
		doc.UploadWay = document.NormalizeUploadWay(doc.UploadWay)
		doc.ContentTextSource = normalizeScannedContentTextSource(doc.ContentTextSource, doc.ContentText, contentLoaded)

		if documentDate.Valid {
			parsed, err := time.Parse("2006-01-02", documentDate.String)
			if err == nil {
				doc.DocumentDate = &parsed
			}
		}
		parsedUpload, err := time.Parse(time.RFC3339, uploadedAt)
		if err != nil {
			return nil, err
		}
		doc.UploadedAt = parsedUpload
		parsedUpdate, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		doc.UpdatedAt = parsedUpdate
		if deletedAt.Valid {
			parsedDelete, err := time.Parse(time.RFC3339, deletedAt.String)
			if err == nil {
				doc.DeletedAt = &parsedDelete
			}
		}
		doc.CustomValues = map[int64]string{}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func normalizeScannedContentTextSource(value, contentText string, contentLoaded bool) string {
	if contentLoaded {
		return document.NormalizeContentTextSource(value, contentText)
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case document.ContentTextSourcePDF:
		return document.ContentTextSourcePDF
	case document.ContentTextSourceFile:
		return document.ContentTextSourceFile
	case document.ContentTextSourceRaw:
		return document.ContentTextSourceRaw
	case document.ContentTextSourceOCR:
		return document.ContentTextSourceOCR
	case document.ContentTextSourceNone:
		return document.ContentTextSourceNone
	case document.ContentTextSourceUnknown:
		return document.ContentTextSourceUnknown
	default:
		return document.ContentTextSourceUnknown
	}
}

func documentIDPlaceholders(docs []document.Document) (string, []any) {
	args := make([]any, len(docs))
	for i := range docs {
		args[i] = docs[i].ID
	}
	return sqlutil.Placeholders(len(docs)), args
}

func int64IDPlaceholders(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return sqlutil.Placeholders(len(ids)), args
}

func forDocumentBatches(docs []document.Document, exec func(placeholders string, args []any) error) error {
	for start := 0; start < len(docs); start += repositoryBatchSize {
		end := start + repositoryBatchSize
		if end > len(docs) {
			end = len(docs)
		}
		placeholders, args := documentIDPlaceholders(docs[start:end])
		if err := exec(placeholders, args); err != nil {
			return err
		}
	}
	return nil
}

func forIDBatches(ids []int64, exec func(placeholders string, args []any) error) error {
	ids = uniqueInt64(ids)
	for start := 0; start < len(ids); start += repositoryBatchSize {
		end := start + repositoryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		placeholders, args := int64IDPlaceholders(ids[start:end])
		if err := exec(placeholders, args); err != nil {
			return err
		}
	}
	return nil
}

type documentTagQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (r *Repository) attachTags(ctx context.Context, docs []document.Document) error {
	return attachTagsWithQuerier(ctx, r.db, docs)
}

func attachTagsWithQuerier(ctx context.Context, queryer documentTagQuerier, docs []document.Document) error {
	if len(docs) == 0 {
		return nil
	}
	index := make(map[int64]int, len(docs))
	for i := range docs {
		index[docs[i].ID] = i
	}

	for start := 0; start < len(docs); start += repositoryBatchSize {
		end := start + repositoryBatchSize
		if end > len(docs) {
			end = len(docs)
		}
		placeholders, args := documentIDPlaceholders(docs[start:end])
		rows, err := queryer.QueryContext(ctx, `
			SELECT dt.document_id, t.name, t.delete_protected
			FROM document_tags dt
			JOIN tags t ON t.id = dt.tag_id
			WHERE dt.document_id IN (`+placeholders+`)
			ORDER BY t.name`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var tag string
			var deleteProtected bool
			if err := rows.Scan(&id, &tag, &deleteProtected); err != nil {
				_ = rows.Close()
				return err
			}
			docs[index[id]].Tags = append(docs[index[id]].Tags, tag)
			docs[index[id]].DeleteProtected = docs[index[id]].DeleteProtected || deleteProtected
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) attachSummaryCounts(ctx context.Context, docs []document.Document) error {
	if len(docs) == 0 {
		return nil
	}
	index := make(map[int64]int, len(docs))
	for i := range docs {
		index[docs[i].ID] = i
	}
	if err := forDocumentBatches(docs, func(placeholders string, args []any) error {
		rows, err := r.db.QueryContext(ctx, `
			SELECT d.id, COUNT(dup.id)
			FROM documents d
			LEFT JOIN documents dup ON dup.sha256 = d.sha256
				AND dup.id != d.id
				AND dup.deleted_at IS NULL
			WHERE d.id IN (`+placeholders+`)
			GROUP BY d.id`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var count int
			if err := rows.Scan(&id, &count); err != nil {
				_ = rows.Close()
				return err
			}
			if pos, ok := index[id]; ok {
				docs[pos].DuplicateCount = count
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		return rows.Close()
	}); err != nil {
		return err
	}
	return forDocumentBatches(docs, func(placeholders string, args []any) error {
		queryArgs := append(append([]any(nil), args...), args...)
		rows, err := r.db.QueryContext(ctx, `
			SELECT document_id, SUM(link_count)
			FROM (
				SELECT source_document_id AS document_id, COUNT(*) AS link_count
				FROM document_links
				WHERE source_document_id IN (`+placeholders+`)
				GROUP BY source_document_id
				UNION ALL
				SELECT target_document_id AS document_id, COUNT(*) AS link_count
				FROM document_links
				WHERE target_document_id IN (`+placeholders+`)
				GROUP BY target_document_id
			)
			GROUP BY document_id`, queryArgs...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var count int
			if err := rows.Scan(&id, &count); err != nil {
				_ = rows.Close()
				return err
			}
			if pos, ok := index[id]; ok {
				docs[pos].LinkedCount = count
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		return rows.Close()
	})
}

func getDocumentTx(ctx context.Context, tx *sql.Tx, id int64) (document.Document, error) {
	rows, err := tx.QueryContext(ctx, baseSelect()+` WHERE d.id = ?`, id)
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
	return docs[0], nil
}

func documentsByIDTx(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]document.Document, error) {
	return documentsByIDSelectTx(ctx, tx, ids, baseSelect(), true)
}

func summaryDocumentsByIDTx(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]document.Document, error) {
	return documentsByIDSelectTx(ctx, tx, ids, summarySelect(), false)
}

func touchDocumentsTx(ctx context.Context, tx *sql.Tx, ids []int64, updatedAt string) error {
	return forIDBatches(ids, func(placeholders string, args []any) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET search_version = ?, updated_at = ?
			WHERE id IN (`+placeholders+`)`, append([]any{document.CurrentSearchVersion, updatedAt}, args...)...)
		return err
	})
}

func insertDocumentTagsTx(ctx context.Context, tx *sql.Tx, docIDs, tagIDs []int64) error {
	if len(docIDs) == 0 || len(tagIDs) == 0 {
		return nil
	}
	const maxPairs = 250
	values := make([]string, 0, maxPairs)
	args := make([]any, 0, maxPairs*2)
	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO document_tags(document_id, tag_id)
			VALUES `+strings.Join(values, ","), args...)
		values = values[:0]
		args = args[:0]
		return err
	}
	for _, docID := range docIDs {
		for _, tagID := range tagIDs {
			values = append(values, "(?, ?)")
			args = append(args, docID, tagID)
			if len(values) == maxPairs {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}

func deleteDocumentTagsTx(ctx context.Context, tx *sql.Tx, docIDs, tagIDs []int64) error {
	if len(docIDs) == 0 || len(tagIDs) == 0 {
		return nil
	}
	tagArgs := make([]any, len(tagIDs))
	for i, tagID := range tagIDs {
		tagArgs[i] = tagID
	}
	tagPlaceholders := sqlutil.Placeholders(len(tagIDs))
	return forIDBatches(docIDs, func(docPlaceholders string, docArgs []any) error {
		args := append(append([]any{}, docArgs...), tagArgs...)
		_, err := tx.ExecContext(ctx, `
			DELETE FROM document_tags
			WHERE document_id IN (`+docPlaceholders+`)
			  AND tag_id IN (`+tagPlaceholders+`)`, args...)
		return err
	})
}

func documentsByIDSelectTx(ctx context.Context, tx *sql.Tx, ids []int64, selectSQL string, attachCustomValues bool) (map[int64]document.Document, error) {
	ids = uniqueInt64(ids)
	if len(ids) == 0 {
		return map[int64]document.Document{}, nil
	}

	docsByID := make(map[int64]document.Document, len(ids))
	if err := forIDBatches(ids, func(placeholders string, args []any) error {
		rows, err := tx.QueryContext(ctx, selectSQL+`
			WHERE d.id IN (`+placeholders+`)`, args...)
		if err != nil {
			return err
		}
		docs, err := scanDocuments(rows)
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err := attachTagsTx(ctx, tx, docs); err != nil {
			return err
		}
		if attachCustomValues {
			if err := attachCustomValuesTx(ctx, tx, docs); err != nil {
				return err
			}
		}
		for _, doc := range docs {
			docsByID[doc.ID] = doc
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return docsByID, nil
}

func attachTagsTx(ctx context.Context, tx *sql.Tx, docs []document.Document) error {
	return attachTagsWithQuerier(ctx, tx, docs)
}

func documentHasDeleteProtectedTagTx(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
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

func reindexDocumentsByIDTx(ctx context.Context, tx *sql.Tx, ids []int64, updatedAt string) error {
	ids = uniqueInt64(ids)
	if len(ids) == 0 {
		return nil
	}
	docs, err := documentsByIDTx(ctx, tx, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		doc, ok := docs[id]
		if !ok {
			continue
		}
		doc.SearchVersion = document.CurrentSearchVersion
		result, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET search_version = ?, updated_at = ?
			WHERE id = ?`, doc.SearchVersion, updatedAt, id)
		if err != nil {
			return err
		}
		if err := requireAffected(result); err != nil {
			return err
		}
		if err := replaceSearchIndex(ctx, tx, id, doc); err != nil {
			return err
		}
	}
	return nil
}

func uniqueInt64(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func uniqueSortedInt64(values []int64) []int64 {
	unique := uniqueInt64(values)
	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
	return unique
}
