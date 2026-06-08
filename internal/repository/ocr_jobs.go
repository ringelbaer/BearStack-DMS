// Datei verwaltet OCR-Jobs, deren Status und Wiederaufnahme in der Datenbank.
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"bearstack/internal/document"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *Repository) EnqueueOCRJob(ctx context.Context, documentID int64, language, languageLabel string) (document.OCRJob, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return document.OCRJob{}, false, err
	}
	defer tx.Rollback()

	job, err := getActiveOCRJobForDocumentTx(ctx, tx, documentID)
	if err == nil {
		return job, false, tx.Commit()
	}
	if err != nil && !errorsIsNoRows(err) {
		return document.OCRJob{}, false, err
	}

	now := time.Now().UTC()
	language = strings.TrimSpace(language)
	languageLabel = strings.TrimSpace(languageLabel)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO ocr_jobs(document_id, language, language_label, status, message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		documentID,
		language,
		languageLabel,
		document.OCRJobStatusQueued,
		"OCR wartet auf Ausführung.",
		formatTime(now),
		formatTime(now),
	)
	if err != nil {
		return document.OCRJob{}, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return document.OCRJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return document.OCRJob{}, false, err
	}
	return document.OCRJob{
		ID:            id,
		DocumentID:    documentID,
		Language:      language,
		LanguageLabel: languageLabel,
		Status:        document.OCRJobStatusQueued,
		Message:       "OCR wartet auf Ausführung.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}, true, nil
}

func (r *Repository) GetOCRJob(ctx context.Context, id int64) (document.OCRJob, error) {
	return scanOCRJob(r.db.QueryRowContext(ctx, ocrJobSelect()+` WHERE id = ?`, id))
}

func (r *Repository) QueuedOCRJobIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id
		FROM ocr_jobs
		WHERE status = ?
		ORDER BY created_at ASC, id ASC
		LIMIT ?`, document.OCRJobStatusQueued, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) LatestOCRJobForDocument(ctx context.Context, documentID int64) (document.OCRJob, bool, error) {
	job, err := scanOCRJob(r.db.QueryRowContext(ctx, ocrJobSelect()+`
		WHERE document_id = ?
		ORDER BY updated_at DESC, id DESC
		LIMIT 1`, documentID))
	if err != nil {
		if errorsIsNoRows(err) {
			return document.OCRJob{}, false, nil
		}
		return document.OCRJob{}, false, err
	}
	return job, true, nil
}

func (r *Repository) LatestRelevantOCRJobsForDocuments(ctx context.Context, documentIDs []int64) (map[int64]*document.OCRJob, error) {
	documentIDs = uniqueInt64(documentIDs)
	if len(documentIDs) == 0 {
		return map[int64]*document.OCRJob{}, nil
	}
	placeholders, args := int64IDPlaceholders(documentIDs)
	args = append(args,
		document.OCRJobStatusQueued,
		document.OCRJobStatusRunning,
		document.OCRJobStatusFailed,
	)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, language, language_label, status, current_page, total_pages,
		       text_length, message, error, created_at, started_at, finished_at, updated_at
		FROM (
			SELECT o.*,
			       ROW_NUMBER() OVER (PARTITION BY o.document_id ORDER BY o.updated_at DESC, o.id DESC) AS rn
			FROM ocr_jobs o
			WHERE o.document_id IN (`+placeholders+`)
		) latest
		WHERE rn = 1
		  AND status IN (?, ?, ?)
		ORDER BY document_id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make(map[int64]*document.OCRJob, len(documentIDs))
	for rows.Next() {
		job, err := scanOCRJob(rows)
		if err != nil {
			return nil, err
		}
		jobCopy := job
		jobs[job.DocumentID] = &jobCopy
	}
	return jobs, rows.Err()
}

func (r *Repository) StartOCRJob(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET status = ?, started_at = COALESCE(started_at, ?), message = ?, error = '', updated_at = ?
		WHERE id = ?
		  AND status IN (?, ?)`,
		document.OCRJobStatusRunning,
		formatTime(now),
		"OCR wird vorbereitet.",
		formatTime(now),
		id,
		document.OCRJobStatusQueued,
		document.OCRJobStatusRunning,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) UpdateOCRJobMessage(ctx context.Context, id int64, message string) error {
	message = truncateString(strings.TrimSpace(message), 1000)
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET message = ?, updated_at = ?
		WHERE id = ?
		  AND status = ?`,
		message,
		formatTime(now),
		id,
		document.OCRJobStatusRunning,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) UpdateOCRJobProgressMessage(ctx context.Context, id int64, currentPage, totalPages int, message string) error {
	if currentPage < 0 {
		currentPage = 0
	}
	if totalPages < 0 {
		totalPages = 0
	}
	if totalPages > 0 && currentPage > totalPages {
		currentPage = totalPages
	}
	message = truncateString(strings.TrimSpace(message), 1000)
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET current_page = ?, total_pages = ?,
		    message = CASE WHEN ? = '' THEN message ELSE ? END,
		    updated_at = ?
		WHERE id = ?
		  AND status = ?`,
		currentPage,
		totalPages,
		message,
		message,
		formatTime(now),
		id,
		document.OCRJobStatusRunning,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) CompleteOCRJob(ctx context.Context, id int64, textLength int) error {
	if textLength < 0 {
		textLength = 0
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET status = ?, text_length = ?, message = ?, error = '', finished_at = ?, updated_at = ?
		WHERE id = ?`,
		document.OCRJobStatusCompleted,
		textLength,
		"OCR abgeschlossen.",
		formatTime(now),
		formatTime(now),
		id,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) FailOCRJob(ctx context.Context, id int64, message string) error {
	return r.finishOCRJobWithStatus(ctx, id, document.OCRJobStatusFailed, message)
}

func (r *Repository) InterruptOCRJob(ctx context.Context, id int64, message string) error {
	return r.finishOCRJobWithStatus(ctx, id, document.OCRJobStatusInterrupted, message)
}

func (r *Repository) InterruptActiveOCRJobs(ctx context.Context, message string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET status = ?, message = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE status IN (?, ?)`,
		document.OCRJobStatusInterrupted,
		truncateString(strings.TrimSpace(message), 1000),
		truncateString(strings.TrimSpace(message), 1000),
		formatTime(now),
		formatTime(now),
		document.OCRJobStatusQueued,
		document.OCRJobStatusRunning,
	)
	return err
}

func (r *Repository) finishOCRJobWithStatus(ctx context.Context, id int64, status, message string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ocr_jobs
		SET status = ?, message = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ?`,
		status,
		truncateString(strings.TrimSpace(message), 1000),
		truncateString(strings.TrimSpace(message), 1000),
		formatTime(now),
		formatTime(now),
		id,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func getActiveOCRJobForDocumentTx(ctx context.Context, tx *sql.Tx, documentID int64) (document.OCRJob, error) {
	return scanOCRJob(tx.QueryRowContext(ctx, ocrJobSelect()+`
		WHERE document_id = ?
		  AND status IN (?, ?)
		ORDER BY updated_at DESC, id DESC
		LIMIT 1`,
		documentID,
		document.OCRJobStatusQueued,
		document.OCRJobStatusRunning,
	))
}

func ocrJobSelect() string {
	return `SELECT id, document_id, language, language_label, status, current_page, total_pages,
		text_length, message, error, created_at, started_at, finished_at, updated_at
		FROM ocr_jobs`
}

func scanOCRJob(row rowScanner) (document.OCRJob, error) {
	var job document.OCRJob
	var createdAt string
	var startedAt sql.NullString
	var finishedAt sql.NullString
	var updatedAt string
	if err := row.Scan(
		&job.ID,
		&job.DocumentID,
		&job.Language,
		&job.LanguageLabel,
		&job.Status,
		&job.CurrentPage,
		&job.TotalPages,
		&job.TextLength,
		&job.Message,
		&job.Error,
		&createdAt,
		&startedAt,
		&finishedAt,
		&updatedAt,
	); err != nil {
		return document.OCRJob{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return document.OCRJob{}, err
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return document.OCRJob{}, err
	}
	job.CreatedAt = parsedCreatedAt
	job.UpdatedAt = parsedUpdatedAt
	if startedAt.Valid {
		parsed, err := time.Parse(time.RFC3339, startedAt.String)
		if err != nil {
			return document.OCRJob{}, err
		}
		job.StartedAt = &parsed
	}
	if finishedAt.Valid {
		parsed, err := time.Parse(time.RFC3339, finishedAt.String)
		if err != nil {
			return document.OCRJob{}, err
		}
		job.FinishedAt = &parsed
	}
	return job, nil
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}
