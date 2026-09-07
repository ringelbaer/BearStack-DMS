package repository

import (
	"context"
	"database/sql"

	"bearstack/internal/document"
)

// FileDeletion survives removal of the document row and application restarts.
type FileDeletion struct {
	DocumentID    int64
	StoredPath    string
	ThumbnailPath string
	Staged        bool
}

func (r *Repository) ensureDocumentFileDeletionTable(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS document_file_deletions (
		document_id INTEGER PRIMARY KEY,
		stored_path TEXT NOT NULL,
		thumbnail_path TEXT NOT NULL,
		staged INTEGER NOT NULL DEFAULT 0 CHECK(staged IN (0, 1))
	)`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_document_file_deletions_path ON document_file_deletions(stored_path)`)
	return err
}

func enqueueFileDeletionTx(ctx context.Context, tx *sql.Tx, doc document.Document) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO document_file_deletions(document_id, stored_path, thumbnail_path) VALUES (?, ?, ?)`, doc.ID, doc.StoredPath, doc.ThumbnailPath)
	return err
}

func (r *Repository) PendingFileDeletions(ctx context.Context, afterID int64, limit int) ([]FileDeletion, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT document_id, stored_path, thumbnail_path, staged FROM document_file_deletions WHERE document_id > ? ORDER BY document_id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []FileDeletion
	for rows.Next() {
		var job FileDeletion
		if err := rows.Scan(&job.DocumentID, &job.StoredPath, &job.ThumbnailPath, &job.Staged); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Repository) FileDeletion(ctx context.Context, id int64) (FileDeletion, error) {
	var job FileDeletion
	err := r.db.QueryRowContext(ctx, `SELECT document_id, stored_path, thumbnail_path, staged FROM document_file_deletions WHERE document_id = ?`, id).Scan(&job.DocumentID, &job.StoredPath, &job.ThumbnailPath, &job.Staged)
	return job, err
}

func (r *Repository) StageFileDeletion(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE document_file_deletions SET staged = 1 WHERE document_id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) CompleteFileDeletion(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM document_file_deletions WHERE document_id = ? AND staged = 1`, id)
	return err
}

// Pending paths must not be reused between unlinking a file and acknowledging
// its deletion. Otherwise a retry could remove a newly imported document.
func (r *Repository) StoredPathPendingDeletion(ctx context.Context, path string) (bool, error) {
	var pending bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM document_file_deletions WHERE stored_path = ?)`, path).Scan(&pending)
	return pending, err
}
