package repository

import (
	"context"
	"database/sql"
	"time"

	"bearstack/internal/document"
)

// ListDocumentFiles retains the list's filters and ordering, including name
// collision order, but reads only file metadata. It never loads OCR text,
// descriptions, tags, custom values or duplicate/link counts.
func (r *Repository) ListDocumentFiles(ctx context.Context, filter document.ListFilter) ([]document.Document, error) {
	query, args := buildDocumentListQuery(filter, `SELECT d.id,d.original_name,d.stored_path,d.title,
 d.mime_type,d.size_bytes,d.sha256,d.document_date,d.uploaded_at,d.updated_at,d.deleted_at FROM documents d`)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []document.Document
	for rows.Next() {
		var doc document.Document
		var date, deleted sql.NullString
		var uploaded, updated string
		if err := rows.Scan(&doc.ID, &doc.OriginalName, &doc.StoredPath, &doc.Title,
			&doc.MIMEType, &doc.SizeBytes, &doc.SHA256, &date, &uploaded, &updated, &deleted); err != nil {
			return nil, err
		}
		if date.Valid {
			if parsed, err := time.Parse("2006-01-02", date.String); err == nil {
				doc.DocumentDate = &parsed
			}
		}
		if doc.UploadedAt, err = time.Parse(time.RFC3339, uploaded); err != nil {
			return nil, err
		}
		if doc.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
			return nil, err
		}
		if deleted.Valid {
			if parsed, err := time.Parse(time.RFC3339, deleted.String); err == nil {
				doc.DeletedAt = &parsed
			}
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}
