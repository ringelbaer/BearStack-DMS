// Datei kapselt OCR-Ausfuehrung, Jobsteuerung und Textuebernahme fuer Dokumente.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"bearstack/internal/document"
	"bearstack/internal/documentocr"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type ocrProgressFunc = documentocr.ProgressFunc

type ocrExecutor interface {
	CheckAvailable(mimeType string) error
	Document(ctx context.Context, source, mimeType, lang string, progress documentocr.ProgressFunc) (string, error)
}

type ocrService struct {
	repo                         *repository.Repository
	store                        *storage.Store
	log                          *slog.Logger
	wake                         chan struct{}
	engine                       ocrExecutor
	invalidateDocumentCountCache func()
	recordAuditLog               func(context.Context, document.AuditLogEntry)
}

const (
	ocrQueuePollInterval = 5 * time.Second
	ocrQueueBatchSize    = 32
)

func (s *Server) ocrService() ocrRunner {
	if s.apps.documents.ocr == nil {
		s.apps.documents.ocr = newOCRService(s.repo, s.store, s.log, make(chan struct{}, 1), s.invalidateDocumentCountCache, s.recordAuditLog)
	}
	return s.apps.documents.ocr
}

func newOCRService(repo *repository.Repository, store *storage.Store, log *slog.Logger, wake chan struct{}, invalidateDocumentCountCache func(), recordAuditLog func(context.Context, document.AuditLogEntry)) *ocrService {
	return &ocrService{
		repo:                         repo,
		store:                        store,
		log:                          log,
		wake:                         wake,
		engine:                       documentocr.LocalEngine{},
		invalidateDocumentCountCache: invalidateDocumentCountCache,
		recordAuditLog:               recordAuditLog,
	}
}

func (o ocrService) RunQueue(ctx context.Context) {
	if o.repo == nil {
		return
	}
	if err := o.repo.InterruptActiveOCRJobs(ctx, "BearStack wurde beendet, bevor der OCR-Vorgang abgeschlossen wurde."); err != nil {
		logWarn(o.log, "ocr startup cleanup failed", "error", err)
	}
	ticker := time.NewTicker(ocrQueuePollInterval)
	defer ticker.Stop()

	for {
		processed, err := o.runPendingJobs(ctx, ocrQueueBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logWarn(o.log, "ocr queue scan failed", "error", err)
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

func (o ocrService) Enqueue(jobID int64) {
	if jobID <= 0 || o.wake == nil {
		return
	}
	select {
	case o.wake <- struct{}{}:
	default:
		// queue wake signal already pending
	}
}

func (o ocrService) runPendingJobs(ctx context.Context, limit int) (int, error) {
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

func (o ocrService) Document(ctx context.Context, doc document.Document, lang string, progress ocrProgressFunc) (string, error) {
	source, err := o.resolveDocumentSource(doc)
	if err != nil {
		return "", err
	}
	return o.engineOrDefault().Document(ctx, source, doc.MIMEType, lang, progress)
}

func (o ocrService) PrepareDocument(doc document.Document) (string, error) {
	if err := o.engineOrDefault().CheckAvailable(doc.MIMEType); err != nil {
		return "", err
	}
	return o.resolveDocumentSource(doc)
}

func (o ocrService) engineOrDefault() ocrExecutor {
	if o.engine != nil {
		return o.engine
	}
	return documentocr.LocalEngine{}
}

func (o ocrService) resolveDocumentSource(doc document.Document) (string, error) {
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

func (o ocrService) RunJob(ctx context.Context, jobID int64) {
	if o.repo == nil {
		return
	}
	job, err := o.repo.GetOCRJob(ctx, jobID)
	if err != nil {
		logWarn(o.log, "ocr job not found", "job_id", jobID, "error", err)
		return
	}
	doc, err := o.repo.GetDocumentFile(ctx, job.DocumentID)
	if err != nil {
		logWarn(o.log, "ocr document lookup failed", "job_id", job.ID, "document_id", job.DocumentID, "error", err)
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
		logWarn(o.log, "ocr job start failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
		return
	}
	job.Status = document.OCRJobStatusRunning
	logInfo(o.log, "ocr started", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel)
	o.RecordAudit(ctx, "OCR gestartet", job, doc, httpStatusOK, "")

	lastLoggedPage := -1
	lastLoggedTotal := -1
	progress := func(currentPage, totalPages int, message string) error {
		if err := o.repo.UpdateOCRJobProgressMessage(ctx, job.ID, currentPage, totalPages, message); err != nil {
			return err
		}
		if shouldLogOCRProgress(currentPage, totalPages, lastLoggedPage, lastLoggedTotal, message) {
			logInfo(o.log, "ocr progress", "job_id", job.ID, "document_id", doc.ID, "current_page", currentPage, "total_pages", totalPages, "message", message)
			lastLoggedPage = currentPage
			lastLoggedTotal = totalPages
		}
		return nil
	}
	text, err := o.Document(ctx, doc, job.Language, progress)
	if err != nil {
		logWarn(o.log, "ocr failed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel, "error", err)
		if errors.Is(err, context.Canceled) {
			_ = o.repo.InterruptOCRJob(context.Background(), job.ID, "OCR wurde durch das Beenden von BearStack unterbrochen.")
			o.RecordAudit(context.Background(), "OCR unterbrochen", job, doc, httpStatusError, "")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			message := "OCR-Zeitlimit nach 20 Minuten überschritten."
			if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, message); updateErr != nil {
				logWarn(o.log, "ocr job failure update failed", "job_id", job.ID, "error", updateErr)
			}
			o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, message)
			return
		}
		if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, err.Error()); updateErr != nil {
			logWarn(o.log, "ocr job failure update failed", "job_id", job.ID, "error", updateErr)
		}
		o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, err.Error())
		return
	}
	if err := o.repo.UpdateOCRJobMessage(ctx, job.ID, "Textinhalt wird gespeichert."); err != nil {
		logWarn(o.log, "ocr job message update failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
	}
	if err := o.repo.UpdateSearchText(ctx, doc.ID, text, document.ContentTextSourceOCR, document.CurrentSearchVersion); err != nil {
		logWarn(o.log, "ocr text update failed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel, "error", err)
		if updateErr := o.repo.FailOCRJob(context.Background(), job.ID, err.Error()); updateErr != nil {
			logWarn(o.log, "ocr job failure update failed", "job_id", job.ID, "error", updateErr)
		}
		o.RecordAudit(context.Background(), "OCR fehlgeschlagen", job, doc, httpStatusError, err.Error())
		return
	}
	if o.invalidateDocumentCountCache != nil {
		o.invalidateDocumentCountCache()
	}
	if err := o.repo.CompleteOCRJob(ctx, job.ID, utf8.RuneCountInString(text)); err != nil {
		logWarn(o.log, "ocr job completion update failed", "job_id", job.ID, "document_id", doc.ID, "error", err)
		return
	}
	logInfo(o.log, "ocr completed", "job_id", job.ID, "document_id", doc.ID, "language", job.LanguageLabel)
	o.RecordAudit(ctx, "OCR abgeschlossen", job, doc, httpStatusOK, "")
}

func (o ocrService) RecordAudit(ctx context.Context, action string, job document.OCRJob, doc document.Document, status int, detail string) {
	parts := []string{documentAuditTargetFor(doc)}
	if job.LanguageLabel != "" {
		parts = append(parts, "OCR "+job.LanguageLabel)
	}
	if detail != "" {
		parts = append(parts, detail)
	}
	if o.recordAuditLog == nil {
		return
	}
	o.recordAuditLog(ctx, document.AuditLogEntry{
		Actor:  "system",
		Method: "OCR",
		Path:   fmt.Sprintf("/documents/%d", doc.ID),
		Route:  "documents/ocr",
		Action: action,
		Target: strings.Join(parts, ": "),
		Status: status,
	})
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

func tesseractLanguage(value string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "de":
		return "deu", "de", nil
	case "eng":
		return "eng", "eng", nil
	default:
		return "", "", errors.New("ungültige OCR-Sprache")
	}
}
