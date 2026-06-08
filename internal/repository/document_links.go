// Datei verwaltet Verknuepfungen zwischen Dokumenten und stellt Abfragen fuer Dokumentbeziehungen bereit.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"bearstack/internal/document"
)

func (r *Repository) LinkDocuments(ctx context.Context, ids []int64) error {
	ids = cleanDocumentLinkIDs(ids)
	if len(ids) < 2 {
		return errors.New("mindestens zwei Dokumente zum Verknüpfen auswählen")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	placeholders, args := int64IDPlaceholders(ids)
	var activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM documents
		WHERE deleted_at IS NULL
		  AND id IN (`+placeholders+`)`, args...).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount != len(ids) {
		return sql.ErrNoRows
	}

	now := formatTime(time.Now().UTC())
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO document_links(source_document_id, target_document_id, created_at)
				VALUES (?, ?, ?)`, ids[i], ids[j], now); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE documents
		SET updated_at = ?
		WHERE id IN (`+placeholders+`)`, append([]any{now}, args...)...); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) LinkedDocuments(ctx context.Context, id int64) ([]document.Document, error) {
	rows, err := r.db.QueryContext(ctx, summarySelect()+`
		JOIN (
			SELECT target_document_id AS linked_id
			FROM document_links
			WHERE source_document_id = ?
			UNION
			SELECT source_document_id AS linked_id
			FROM document_links
			WHERE target_document_id = ?
		) linked ON linked.linked_id = d.id
		ORDER BY COALESCE(d.document_date, substr(d.uploaded_at, 1, 10)) DESC, d.uploaded_at DESC, d.id DESC`, id, id)
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

func (r *Repository) LinkedDocumentsForDocuments(ctx context.Context, ids []int64) (map[int64][]document.Document, error) {
	ids = uniqueInt64(ids)
	linked := make(map[int64][]document.Document, len(ids))
	if len(ids) == 0 {
		return linked, nil
	}
	requested := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
		linked[id] = []document.Document{}
	}

	linkedIDs := make([]int64, 0)
	linkedSeen := map[int64]struct{}{}
	pairSeen := map[int64]map[int64]struct{}{}
	pairs := make(map[int64][]int64, len(ids))
	appendPair := func(requestID, linkedID int64) {
		seen := pairSeen[requestID]
		if seen == nil {
			seen = map[int64]struct{}{}
			pairSeen[requestID] = seen
		}
		if _, ok := seen[linkedID]; ok {
			return
		}
		seen[linkedID] = struct{}{}
		pairs[requestID] = append(pairs[requestID], linkedID)
		if _, ok := linkedSeen[linkedID]; ok {
			return
		}
		linkedSeen[linkedID] = struct{}{}
		linkedIDs = append(linkedIDs, linkedID)
	}
	if err := forIDBatches(ids, func(placeholders string, args []any) error {
		queryArgs := append(append([]any(nil), args...), args...)
		rows, err := r.db.QueryContext(ctx, `
			SELECT source_document_id, target_document_id
			FROM document_links
			WHERE source_document_id IN (`+placeholders+`)
			   OR target_document_id IN (`+placeholders+`)`, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sourceID, targetID int64
			if err := rows.Scan(&sourceID, &targetID); err != nil {
				return err
			}
			if _, ok := requested[sourceID]; ok {
				appendPair(sourceID, targetID)
			}
			if _, ok := requested[targetID]; ok {
				appendPair(targetID, sourceID)
			}
		}
		return rows.Err()
	}); err != nil {
		return nil, err
	}

	docsByID, err := r.documentsByIDsIncludingTrash(ctx, linkedIDs)
	if err != nil {
		return nil, err
	}
	for id, linkedDocIDs := range pairs {
		docs := make([]document.Document, 0, len(linkedDocIDs))
		for _, linkedID := range linkedDocIDs {
			if doc, ok := docsByID[linkedID]; ok {
				docs = append(docs, doc)
			}
		}
		sort.SliceStable(docs, func(i, j int) bool {
			return linkedDocumentLess(docs[i], docs[j])
		})
		linked[id] = docs
	}
	return linked, nil
}

func (r *Repository) documentsByIDsIncludingTrash(ctx context.Context, ids []int64) (map[int64]document.Document, error) {
	ids = uniqueInt64(ids)
	docsByID := make(map[int64]document.Document, len(ids))
	if len(ids) == 0 {
		return docsByID, nil
	}
	if err := forIDBatches(ids, func(placeholders string, args []any) error {
		rows, err := r.db.QueryContext(ctx, summarySelect()+`
			WHERE d.id IN (`+placeholders+`)`, args...)
		if err != nil {
			return err
		}
		docs, err := scanSummaryDocuments(rows)
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err := r.attachSummaryDetails(ctx, docs); err != nil {
			return err
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

func linkedDocumentLess(a, b document.Document) bool {
	aDate := linkedDocumentSortDate(a)
	bDate := linkedDocumentSortDate(b)
	if !aDate.Equal(bDate) {
		return aDate.After(bDate)
	}
	if !a.UploadedAt.Equal(b.UploadedAt) {
		return a.UploadedAt.After(b.UploadedAt)
	}
	return a.ID > b.ID
}

func linkedDocumentSortDate(doc document.Document) time.Time {
	if doc.DocumentDate != nil {
		return *doc.DocumentDate
	}
	return time.Date(doc.UploadedAt.Year(), doc.UploadedAt.Month(), doc.UploadedAt.Day(), 0, 0, 0, 0, time.UTC)
}

func (r *Repository) GroupedDocuments(ctx context.Context, id int64) ([]document.Document, error) {
	rows, err := r.db.QueryContext(ctx, summarySelect()+`
		JOIN (
			SELECT DISTINCT other_dt.document_id AS grouped_id
			FROM document_tags self_dt
			JOIN tags t ON t.id = self_dt.tag_id
			JOIN document_tags other_dt ON other_dt.tag_id = self_dt.tag_id
			WHERE self_dt.document_id = ?
			  AND other_dt.document_id != ?
			  AND t.group_mode = 1
		) grouped ON grouped.grouped_id = d.id
		ORDER BY COALESCE(d.document_date, substr(d.uploaded_at, 1, 10)) DESC, d.uploaded_at DESC, d.id DESC`, id, id)
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

func (r *Repository) UnlinkDocuments(ctx context.Context, id, linkedID int64) error {
	ids := cleanDocumentLinkIDs([]int64{id, linkedID})
	if len(ids) != 2 {
		return sql.ErrNoRows
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		DELETE FROM document_links
		WHERE source_document_id = ?
		  AND target_document_id = ?`, ids[0], ids[1])
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}

	now := formatTime(time.Now().UTC())
	placeholders, args := int64IDPlaceholders(ids)
	if _, err := tx.ExecContext(ctx, `
		UPDATE documents
		SET updated_at = ?
		WHERE id IN (`+placeholders+`)`, append([]any{now}, args...)...); err != nil {
		return err
	}
	return tx.Commit()
}

func cleanDocumentLinkIDs(ids []int64) []int64 {
	return uniqueSortedInt64(ids)
}
