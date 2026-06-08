// Datei enthaelt Wartungsoperationen wie Aufraeumen, Integritaetspruefungen und abgeleitete Aktualisierungen.
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"bearstack/internal/document"
)

func (r *Repository) UpdateThumbnailPath(ctx context.Context, id int64, thumbnailPath string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET thumbnail_path = ?
		WHERE id = ?`, strings.TrimSpace(thumbnailPath), id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) ThumbnailCandidates(ctx context.Context, limit int) ([]document.Document, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, original_name, stored_path, thumbnail_path, mime_type
			FROM documents
			WHERE deleted_at IS NULL
			  AND (
			      mime_type IN ('application/pdf', 'image/jpeg', 'image/png', 'image/gif')
			      OR lower(mime_type) LIKE 'text/plain%'
			      OR lower(mime_type) LIKE 'text/markdown%'
			      OR lower(mime_type) IN (
			          'text/rtf',
			          'application/rtf',
			          'application/msword',
			          'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
			          'application/vnd.apple.pages'
			      )
			      OR lower(original_name) LIKE '%.txt'
			      OR lower(original_name) LIKE '%.md'
			      OR lower(original_name) LIKE '%.rtf'
			      OR lower(original_name) LIKE '%.doc'
			      OR lower(original_name) LIKE '%.docx'
			      OR lower(original_name) LIKE '%.pages'
			  )
			  AND thumbnail_path = ''
		ORDER BY uploaded_at ASC, id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []document.Document
	for rows.Next() {
		var doc document.Document
		if err := rows.Scan(&doc.ID, &doc.OriginalName, &doc.StoredPath, &doc.ThumbnailPath, &doc.MIMEType); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (r *Repository) PostImportPendingDocuments(ctx context.Context, limit int) ([]document.Document, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, original_name, stored_path, thumbnail_path, mime_type
		FROM documents
		WHERE deleted_at IS NULL
		  AND post_import_pending = 1
		ORDER BY post_import_attempted_at ASC,
		         uploaded_at ASC,
		         id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []document.Document
	for rows.Next() {
		var doc document.Document
		if err := rows.Scan(&doc.ID, &doc.OriginalName, &doc.StoredPath, &doc.ThumbnailPath, &doc.MIMEType); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (r *Repository) MarkPostImportComplete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET post_import_pending = 0
		WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) MarkPostImportAttempted(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET post_import_attempted_at = ?, post_import_attempts = post_import_attempts + 1
		WHERE id = ?`, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) GetDocumentFile(ctx context.Context, id int64) (document.Document, error) {
	var doc document.Document
	var deletedAt sql.NullString
	var updatedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, original_name, stored_path, thumbnail_path, title, mime_type,
		       size_bytes, sha256, updated_at, deleted_at
		FROM documents
		WHERE id = ?`, id).Scan(
		&doc.ID,
		&doc.OriginalName,
		&doc.StoredPath,
		&doc.ThumbnailPath,
		&doc.Title,
		&doc.MIMEType,
		&doc.SizeBytes,
		&doc.SHA256,
		&updatedAt,
		&deletedAt,
	)
	if err != nil {
		return document.Document{}, err
	}
	parsedUpdate, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return document.Document{}, err
	}
	doc.UpdatedAt = parsedUpdate
	if deletedAt.Valid {
		parsedDelete, err := time.Parse(time.RFC3339, deletedAt.String)
		if err != nil {
			return document.Document{}, err
		}
		doc.DeletedAt = &parsedDelete
	}
	return doc, nil
}

func (r *Repository) UpdateSearchText(ctx context.Context, id int64, contentText, contentTextSource string, searchVersion int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	docs, err := summaryDocumentsByIDTx(ctx, tx, []int64{id})
	if err != nil {
		return err
	}
	doc, ok := docs[id]
	if !ok {
		return sql.ErrNoRows
	}
	customValues, err := customValuesForDocumentTx(ctx, tx, id)
	if err != nil {
		return err
	}
	doc.CustomValues = customValues
	doc.ContentText = contentText
	doc.ContentTextSource = document.NormalizeContentTextSource(contentTextSource, contentText)
	doc.SearchVersion = searchVersion

	result, err := tx.ExecContext(ctx, `
		UPDATE documents
		SET content_text = ?, content_text_source = ?, search_version = ?
		WHERE id = ?`, contentText, doc.ContentTextSource, searchVersion, id)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	if err := replaceSearchIndex(ctx, tx, id, doc); err != nil {
		return err
	}
	return tx.Commit()
}
