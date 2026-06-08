// Datei findet und verwaltet moegliche Dokumentduplikate anhand gespeicherter Dateimerkmale.
package repository

import (
	"context"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

func (r *Repository) DuplicateGroups(ctx context.Context) ([]document.DuplicateGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sha256, COUNT(*), SUM(size_bytes)
		FROM documents
		WHERE deleted_at IS NULL
		GROUP BY sha256
		HAVING COUNT(*) > 1
		ORDER BY COUNT(*) DESC, MAX(uploaded_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []document.DuplicateGroup
	for rows.Next() {
		var group document.DuplicateGroup
		if err := rows.Scan(&group.SHA256, &group.Count, &group.TotalBytes); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	checksums := make([]string, len(groups))
	for i, group := range groups {
		checksums[i] = group.SHA256
	}
	docsByChecksum, err := r.documentsByChecksums(ctx, checksums)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].Documents = docsByChecksum[groups[i].SHA256]
	}
	return groups, nil
}

func (r *Repository) documentsByChecksums(ctx context.Context, checksums []string) (map[string][]document.Document, error) {
	docsByChecksum := make(map[string][]document.Document, len(checksums))
	if len(checksums) == 0 {
		return docsByChecksum, nil
	}

	var docs []document.Document
	for start := 0; start < len(checksums); start += repositoryBatchSize {
		end := start + repositoryBatchSize
		if end > len(checksums) {
			end = len(checksums)
		}
		batch := checksums[start:end]
		args := make([]any, len(batch))
		for i, checksum := range batch {
			args[i] = checksum
		}
		placeholders := sqlutil.Placeholders(len(batch))
		rows, err := r.db.QueryContext(ctx, summarySelect()+`
			WHERE d.sha256 IN (`+placeholders+`)
			  AND d.deleted_at IS NULL
			ORDER BY d.sha256 ASC, d.uploaded_at DESC`, args...)
		if err != nil {
			return nil, err
		}
		batchDocs, err := scanSummaryDocuments(rows)
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, batchDocs...)
	}
	if err := r.attachSummaryDetails(ctx, docs); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		docsByChecksum[doc.SHA256] = append(docsByChecksum[doc.SHA256], doc)
	}
	return docsByChecksum, nil
}
