// Datei koordiniert Mail-Importlaeufe im Server und protokolliert deren Ergebnis.
package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/documentimport"
	"bearstack/internal/mailarchive"
	"bearstack/internal/mailimport"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

const mailImportCheckInterval = time.Minute

type mailImportService struct {
	maxUploadBytes int64
	repo           *repository.Repository
	store          *storage.Store
	log            *slog.Logger
	mu             *sync.Mutex
	importer       documentImporter
	recordAuditLog func(context.Context, document.AuditLogEntry)
}

func (s *Server) mailImportService() mailImportRunner {
	if s.apps.mail.importer == nil {
		s.apps.mail.importer = newMailImportService(s.cfg.MaxUploadBytes, s.repo, s.store, s.log, s.documentImporter(), s.recordAuditLog)
	}
	return s.apps.mail.importer
}

func newMailImportService(maxUploadBytes int64, repo *repository.Repository, store *storage.Store, log *slog.Logger, importer documentImporter, recordAuditLog func(context.Context, document.AuditLogEntry)) *mailImportService {
	return &mailImportService{
		maxUploadBytes: maxUploadBytes,
		repo:           repo,
		store:          store,
		log:            log,
		mu:             &sync.Mutex{},
		importer:       importer,
		recordAuditLog: recordAuditLog,
	}
}

type mailImportRunResult struct {
	Messages   int
	Processed  int
	Deleted    int
	Uploaded   int
	Archived   int
	EMLs       int
	Duplicates int
	Rejected   int
	Errors     int
}

type mailMessageImportResult struct {
	Subject    string
	From       string
	PDFs       int
	EMLs       int
	Uploaded   int
	Archived   int
	Duplicates int
	Rejected   bool
	Errors     int
	Details    []string
}

func (m *mailImportService) Run(ctx context.Context) {
	var lastRun time.Time
	m.runIfDue(ctx, &lastRun)

	ticker := time.NewTicker(mailImportCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runIfDue(ctx, &lastRun)
		}
	}
}

func (m *mailImportService) runIfDue(ctx context.Context, lastRun *time.Time) {
	settings, _, err := m.repo.GetMailImportSettings(ctx)
	if err != nil {
		logWarn(m.log, "mail import settings failed", "error", err)
		m.RecordAudit(ctx, "E-Mail-Import fehlgeschlagen", "Einstellungen konnten nicht gelesen werden: "+err.Error(), httpStatusError)
		return
	}
	if !settings.Enabled {
		return
	}

	interval := time.Duration(settings.PollIntervalMinutes) * time.Minute
	if !lastRun.IsZero() && time.Since(*lastRun) < interval {
		return
	}
	*lastRun = time.Now()

	result, err := m.ImportPDFs(ctx, settings)
	if err != nil {
		logWarn(m.log, "mail import failed", "host", settings.Host, "mailbox", settings.Mailbox, "error", err)
		m.RecordAudit(ctx, "E-Mail-Import fehlgeschlagen", err.Error(), httpStatusError)
		return
	}
	if result.Processed > 0 || result.Rejected > 0 || result.Errors > 0 {
		target := fmt.Sprintf("%d Mail(s), %d PDF(s) importiert, %d E-Mail-Archiv(e), %d Duplikat(e), %d abgelehnt, %d gelöscht", result.Processed, result.Uploaded, result.Archived, result.Duplicates, result.Rejected, result.Deleted)
		status := httpStatusOK
		action := "E-Mail-Import abgeschlossen"
		if result.Errors > 0 {
			status = httpStatusError
			action = "E-Mail-Import mit Fehlern"
			target = fmt.Sprintf("%s, %d Fehler", target, result.Errors)
		}
		m.RecordAudit(ctx, action, target, status)
	}
}

func (m *mailImportService) ImportPDFs(ctx context.Context, settings document.MailImportSettings) (mailImportRunResult, error) {
	settings = repository.NormalizeMailImportSettings(settings)
	if err := repository.ValidateMailImportSettings(settings); err != nil {
		return mailImportRunResult{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := mailimport.OpenMailbox(settings, false)
	if err != nil {
		return mailImportRunResult{}, err
	}
	defer func() {
		if err := c.Logout(); err != nil {
			logWarn(m.log, "mail import logout failed", "error", err)
		}
	}()

	uids, err := mailimport.UndeletedUIDs(c)
	if err != nil {
		return mailImportRunResult{}, err
	}

	var result mailImportRunResult
	result.Messages = len(uids)
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		messageResult, err := m.importIMAPMessage(ctx, c, uid, settings)
		if err != nil {
			result.Errors++
			logWarn(m.log, "mail import message failed", "uid", uid, "error", err)
			m.RecordAudit(ctx, "E-Mail-Verarbeitung fehlgeschlagen", fmt.Sprintf("UID %d: %s", uid, err), httpStatusError)
			continue
		}
		if messageResult.Rejected {
			result.Rejected++
			if err := mailimport.DeleteMessage(c, uid); err != nil {
				result.Errors++
				m.RecordAudit(ctx, "E-Mail-Löschen fehlgeschlagen", fmt.Sprintf("UID %d: %s", uid, err), httpStatusError)
				continue
			}
			result.Deleted++
			m.RecordAudit(ctx, "E-Mail-Absender abgelehnt", mailMessageAuditTarget(uid, messageResult), httpStatusOK)
			continue
		}
		if messageResult.PDFs == 0 && messageResult.EMLs == 0 {
			continue
		}

		result.Processed++
		result.Uploaded += messageResult.Uploaded
		result.Archived += messageResult.Archived
		result.EMLs += messageResult.EMLs
		result.Duplicates += messageResult.Duplicates
		result.Errors += messageResult.Errors
		if messageResult.Errors > 0 {
			m.RecordAudit(ctx, "E-Mail-Verarbeitung fehlgeschlagen", mailMessageAuditTarget(uid, messageResult), httpStatusError)
			continue
		}
		if err := mailimport.DeleteMessage(c, uid); err != nil {
			result.Errors++
			m.RecordAudit(ctx, "E-Mail-Löschen fehlgeschlagen", fmt.Sprintf("UID %d: %s", uid, err), httpStatusError)
			continue
		}
		result.Deleted++
		m.RecordAudit(ctx, "E-Mail verarbeitet", mailMessageAuditTarget(uid, messageResult), httpStatusOK)
	}

	return result, nil
}

func (m *mailImportService) CheckSettings(ctx context.Context, settings document.MailImportSettings) error {
	settings = repository.NormalizeMailImportSettings(settings)
	settings.Enabled = true
	if err := repository.ValidateMailImportSettings(settings); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := mailimport.OpenMailbox(settings, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Logout(); err != nil {
			logWarn(m.log, "mail import test logout failed", "error", err)
		}
	}()

	return nil
}

func (m *mailImportService) importIMAPMessage(ctx context.Context, c *mailimport.Client, uid uint32, settings document.MailImportSettings) (mailMessageImportResult, error) {
	body, err := mailimport.FetchMessage(c, uid)
	if err != nil {
		return mailMessageImportResult{}, err
	}
	return m.importPDFsFromMail(ctx, body, settings.AllowedSenders)
}

func (s *Server) importPDFsFromMail(ctx context.Context, r io.Reader, allowedSenders string) (mailMessageImportResult, error) {
	return s.mailImportService().importPDFsFromMail(ctx, r, allowedSenders)
}

func (m *mailImportService) importPDFsFromMail(ctx context.Context, r io.Reader, allowedSenders string) (mailMessageImportResult, error) {
	var result mailMessageImportResult
	message, err := mailimport.ImportAttachmentsFromMessage(r, allowedSenders, m.maxUploadBytes, func(att mailimport.Attachment) error {
		candidate, err := m.store.ReceiveReader(att.Filename, att.Reader, m.maxUploadBytes)
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, att.Filename+": "+friendlyUploadError(err))
			return nil
		}

		importResult := m.importer.ImportCandidate(ctx, candidate, document.UploadWayMail)
		switch {
		case importResult.Created != nil:
			result.Uploaded++
			result.Details = append(result.Details, "importiert "+importResult.Created.Document.OriginalName)
		case importResult.Duplicate != nil:
			result.Duplicates++
			result.Details = append(result.Details, "Duplikat "+importResult.Duplicate.Filename)
		case importResult.Error != nil:
			result.Errors++
			logWarn(m.log, "mail attachment import failed", "filename", att.Filename, "error", importResult.Error)
			result.Details = append(result.Details, att.Filename+": "+friendlyImportError(importResult.Error))
		default:
			result.Errors++
			result.Details = append(result.Details, att.Filename+": unbekannter Importstatus")
		}
		return nil
	}, func(att mailimport.Attachment) error {
		tempDir, err := m.store.EnsureDir(".tmp")
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, att.Filename+": "+err.Error())
			return nil
		}
		archive, err := mailarchive.Build(ctx, att.Filename, att.Reader, mailarchive.Options{MaxBytes: m.maxUploadBytes, TempDir: tempDir})
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, att.Filename+": "+err.Error())
			return nil
		}
		defer archive.Cleanup()

		file, err := os.Open(archive.Path)
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, att.Filename+": "+err.Error())
			return nil
		}
		defer file.Close()

		candidate, err := m.store.ReceiveReader(archive.Filename, file, m.maxUploadBytes)
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, archive.Filename+": "+friendlyUploadError(err))
			return nil
		}

		importResult := m.importer.ImportCandidateWithOptions(ctx, candidate, documentimport.ImportOptions{
			UploadWay:    document.UploadWayMail,
			Title:        archive.Title,
			Description:  archive.Description,
			DocumentDate: archive.DocumentDate,
		})
		switch {
		case importResult.Created != nil:
			result.Archived++
			result.Details = append(result.Details, "archiviert "+importResult.Created.Document.OriginalName)
		case importResult.Duplicate != nil:
			result.Duplicates++
			result.Details = append(result.Details, "Duplikat "+importResult.Duplicate.Filename)
		case importResult.Error != nil:
			result.Errors++
			logWarn(m.log, "mail archive import failed", "filename", att.Filename, "error", importResult.Error)
			result.Details = append(result.Details, archive.Filename+": "+friendlyImportError(importResult.Error))
		default:
			result.Errors++
			result.Details = append(result.Details, archive.Filename+": unbekannter Importstatus")
		}
		return nil
	})
	result.Subject = message.Subject
	result.From = message.From
	result.PDFs = message.PDFs
	result.EMLs = message.EMLs
	result.Rejected = message.Rejected
	if err != nil {
		return result, err
	}
	return result, nil
}

const (
	httpStatusOK    = 200
	httpStatusError = 500
)

func (m *mailImportService) RecordAudit(ctx context.Context, action, target string, status int) {
	if m.recordAuditLog == nil {
		return
	}
	m.recordAuditLog(ctx, document.AuditLogEntry{
		Actor:  "system",
		Method: "IMAP",
		Path:   "/settings/mail-import",
		Route:  "settings/mail-import",
		Action: action,
		Target: target,
		Status: status,
	})
}

func mailMessageAuditTarget(uid uint32, result mailMessageImportResult) string {
	parts := []string{fmt.Sprintf("UID %d", uid)}
	if result.From != "" {
		parts = append(parts, "Von "+result.From)
	}
	if result.Subject != "" {
		parts = append(parts, result.Subject)
	}
	if result.Rejected {
		parts = append(parts, "Absender nicht erlaubt")
		return strings.Join(parts, ": ")
	}
	parts = append(parts, fmt.Sprintf("%d PDF(s), %d EML(s), %d PDF(s) importiert, %d E-Mail-Archiv(e), %d Duplikat(e)", result.PDFs, result.EMLs, result.Uploaded, result.Archived, result.Duplicates))
	if result.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d Fehler", result.Errors))
	}
	return strings.Join(parts, ": ")
}
