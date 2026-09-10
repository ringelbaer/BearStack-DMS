// Package ocrservice owns the OCR queue, job lifecycle and search-text updates.
package ocrservice

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"
	"unicode/utf8"

	"bearstack/internal/document"
	"bearstack/internal/documentocr"
)

type Repository interface {
	InterruptActiveOCRJobs(context.Context, string) error
	QueuedOCRJobIDs(context.Context, int) ([]int64, error)
	GetOCRJob(context.Context, int64) (document.OCRJob, error)
	GetDocumentFile(context.Context, int64) (document.Document, error)
	StartOCRJob(context.Context, int64) error
	FailOCRJob(context.Context, int64, string) error
	InterruptOCRJob(context.Context, int64, string) error
	UpdateOCRJobProgressMessage(context.Context, int64, int, int, string) error
	UpdateOCRJobMessage(context.Context, int64, string) error
	UpdateSearchText(context.Context, int64, string, string, int) error
	CompleteOCRJob(context.Context, int64, int) error
}
type Store interface{ Resolve(string) (string, error) }
type AuditFunc func(context.Context, string, document.OCRJob, document.Document, int, string)

const (
	httpStatusOK    = 200
	httpStatusError = 500
)

type Executor interface {
	CheckAvailable(mimeType string) error
	Document(ctx context.Context, source, mimeType, lang string, progress documentocr.ProgressFunc) (string, error)
}

type Service struct {
	repo                         Repository
	store                        Store
	log                          *slog.Logger
	wake                         chan struct{}
	engine                       Executor
	invalidateDocumentCountCache func()
	recordAudit                  AuditFunc
}

const (
	ocrQueuePollInterval = 5 * time.Second
	ocrQueueBatchSize    = 32
)

func New(repo Repository, store Store, log *slog.Logger, engine Executor, invalidateDocumentCountCache func(), recordAudit AuditFunc) *Service {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		repo:                         repo,
		store:                        store,
		log:                          log,
		wake:                         make(chan struct{}, 1),
		engine:                       engine,
		invalidateDocumentCountCache: invalidateDocumentCountCache,
		recordAudit:                  recordAudit,
	}
}

func (o *Service) RunQueue(ctx context.Context) {
	if o.repo == nil {
		return
	}
	if err := o.repo.InterruptActiveOCRJobs(ctx, "BearStack wurde beendet, bevor der OCR-Vorgang abgeschlossen wurde."); err != nil {
		o.log.Warn("ocr startup cleanup failed", "error", err)
	}
	ticker := time.NewTicker(ocrQueuePollInterval)
	defer ticker.Stop()

	for {
		processed, err := o.runPendingJobs(ctx, ocrQueueBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			o.log.Warn("ocr queue scan failed", "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		if processed > 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-o.wake:
		case <-ticker.C:
		}
	}
}

func (o *Service) Enqueue(jobID int64) {
	if jobID <= 0 || o.wake == nil {
		return
	}
	select {
	case o.wake <- struct{}{}:
	default:
		// queue wake signal already pending
	}
}

func (o *Service) runPendingJobs(ctx context.Context, limit int) (int, error) {
	if o.repo == nil {
		return 0, nil
	}
	ids, err := o.repo.QueuedOCRJobIDs(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, jobID := range ids {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		o.RunJob(ctx, jobID)
		processed++
	}
	return processed, nil
}

func (o *Service) Document(ctx context.Context, doc document.Document, lang string, progress documentocr.ProgressFunc) (string, error) {
	source, err := o.resolveDocumentSource(doc)
	if err != nil {
		return "", err
	}
	return o.engineOrDefault().Document(ctx, source, doc.MIMEType, lang, progress)
}

func (o *Service) PrepareDocument(doc document.Document) (string, error) {
	if err := o.engineOrDefault().CheckAvailable(doc.MIMEType); err != nil {
		return "", err
	}
	return o.resolveDocumentSource(doc)
}

func (o *Service) engineOrDefault() Executor {
	if o.engine != nil {
		return o.engine
	}
	return documentocr.LocalEngine{}
}

func (o *Service) resolveDocumentSource(doc document.Document) (string, error) {
	if o.store == nil {
		return "", errors.New("document storage is not configured")
	}
	source, err := o.store.Resolve(doc.StoredPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	return source, nil
}

func (o *Service) RunJob(ctx context.Context, jobID int64) {
	if o.repo == nil {
		return
	}
	job, err := o.repo.GetOCRJob(ctx, jobID)
	if err != nil {
		o.log.Warn("ocr job not found", "job_id", jobID, "error", err)
		return
	}
	doc, err := o.repo.GetDocumentFile(ctx, job.DocumentID)
	if err != nil {
		o.log.Warn("ocr document lookup failed", "job_id", job.ID, "document_id", job.DocumentID, "error", err)
		_ = o.repo.FailOCRJob(context.Background(), job.ID, err.Error())
		return
	}
	if doc.IsDeleted() {
		message := "OCR ist für gelöschte Dokumente nicht verfügbar"
		_ = o.repo.FailOCRJob(context.Background(), job.ID, message)
		o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, message)
		return
	}
	if err := o.repo.StartOCRJob(ctx, job.ID); err != nil {
		o.log.Warn("ocr job start failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
		return
	}
	job.Status = document.OCRJobStatusRunning
	o.log.Info("ocr started", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel)
	o.RecordAudit(ctx, "OCR gestartet", job, doc, httpStatusOK, "")

	lastLoggedPage := -1
	lastLoggedTotal := -1
	progress := func(currentPage, totalPages int, message string) error {
		if err := o.repo.UpdateOCRJobProgressMessage(ctx, job.ID, currentPage, totalPages, message); err != nil {
			return err
		}
		if shouldLogOCRProgress(currentPage, totalPages, lastLoggedPage, lastLoggedTotal, message) {
			o.log.Info("ocr progress", "job_id", job.ID, "document_id", doc.ID, "current_page", currentPage, "total_pages", totalPages, "message", message)
			lastLoggedPage = currentPage
			lastLoggedTotal = totalPages
		}
		return nil
	}
	text, err := o.Document(ctx, doc, job.Language, progress)
	if err != nil {
		o.log.Warn("ocr failed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel, "error", err)
		if errors.Is(err, context.Canceled) {
			_ = o.repo.InterruptOCRJob(context.Background(), job.ID, "OCR wurde durch das Beenden von BearStack unterbrochen.")
			o.RecordAudit(context.Background(), "OCR unterbrochen", job, doc, httpStatusError, "")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			message := "OCR-Zeitlimit nach 20 Minuten überschritten."
			if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, message); updateErr != nil {
				o.log.Warn("ocr job failure update failed", "job_id", job.ID, "error", updateErr)
			}
			o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, message)
			return
		}
		if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, err.Error()); updateErr != nil {
			o.log.Warn("ocr job failure update failed", "job_id", job.ID, "error", updateErr)
		}
		o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, err.Error())
		return
	}
	if err := o.repo.UpdateOCRJobMessage(ctx, job.ID, "Textinhalt wird gespeichert."); err != nil {
		o.log.Warn("ocr job message update failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
	}
	if err := o.repo.UpdateSearchText(ctx, doc.ID, text, document.ContentTextSourceOCR, document.CurrentSearchVersion); err != nil {
		o.log.Warn("ocr text update failed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel, "error", err)
		if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, err.Error()); updateErr != nil {
			o.log.Warn("ocr job failure update failed", "job_id", job.ID, "error", updateErr)
		}
		o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, err.Error())
		return
	}
	if o.invalidateDocumentCountCache != nil {
		o.invalidateDocumentCountCache()
	}
	if err := o.repo.CompleteOCRJob(ctx, job.ID, utf8.RuneCountInString(text)); err != nil {
		o.log.Warn("ocr job completion update failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
		return
	}
	o.log.Info("ocr completed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel)
	o.RecordAudit(ctx, "OCR abgeschlossen", job, doc, httpStatusOK, "")
}

func (o *Service) RecordAudit(ctx context.Context, action string, job document.OCRJob, doc document.Document, status int, detail string) {
	if o.recordAudit != nil {
		o.recordAudit(ctx, action, job, doc, status, detail)
	}
}

func shouldLogOCRProgress(currentPage, totalPages, lastLoggedPage, lastLoggedTotal int, message string) bool {
	if message == "" {
		return false
	}
	if currentPage == lastLoggedPage && totalPages == lastLoggedTotal {
		return false
	}
	if totalPages <= 0 {
		return true
	}
	if currentPage <= 0 {
		return true
	}
	if currentPage >= totalPages {
		return true
	}
	return currentPage%10 == 0
}
