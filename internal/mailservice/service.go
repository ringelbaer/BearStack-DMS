// Package mailservice coordinates scheduled and manual mail imports.
package mailservice

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
	"bearstack/internal/storage"
)

const mailImportCheckInterval = time.Minute
const mailImportAuditDetailLimit = 900

type Repository interface {
	GetMailImportSettings(context.Context) (document.MailImportSettings, bool, error)
}
type Store interface {
	ReceiveReader(string, io.Reader, int64) (storage.Candidate, error)
	EnsureDir(string) (string, error)
}
type Importer interface {
	ImportCandidate(context.Context, storage.Candidate, string) documentimport.Result
	ImportCandidateWithOptions(context.Context, storage.Candidate, documentimport.ImportOptions) documentimport.Result
}
type AuditFunc func(context.Context, string, string, int)

type Service struct {
	maxUploadBytes int64
	repo           Repository
	store          Store
	log            *slog.Logger
	mu             sync.Mutex
	importer       Importer
	recordAudit    AuditFunc
	openMailbox    func(document.MailImportSettings, bool) (mailbox, error)
}

func New(maxUploadBytes int64, repo Repository, store Store, log *slog.Logger, importer Importer, recordAudit AuditFunc) *Service {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		maxUploadBytes: maxUploadBytes,
		repo:           repo,
		store:          store,
		log:            log,
		importer:       importer,
		recordAudit:    recordAudit,
		openMailbox:    openMailbox,
	}
}

type RunResult struct {
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

type MessageResult struct {
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

func (m *Service) Run(ctx context.Context) {
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

func (m *Service) runIfDue(ctx context.Context, lastRun *time.Time) {
	settings, _, err := m.repo.GetMailImportSettings(ctx)
	if err != nil {
		m.log.Warn("mail import settings failed", "error", err)
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
		m.log.Warn("mail import failed", "host", settings.Host, "mailbox", settings.Mailbox, "error", err)
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

func (m *Service) ImportPDFs(ctx context.Context, settings document.MailImportSettings) (RunResult, error) {
	settings = document.NormalizeMailImportSettings(settings)
	if err := document.ValidateMailImportSettings(settings); err != nil {
		return RunResult{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := m.openMailbox(settings, false)
	if err != nil {
		return RunResult{}, err
	}
	defer func() {
		if err := c.Logout(); err != nil {
			m.log.Warn("mail import logout failed", "error", err)
		}
	}()

	uids, err := c.UndeletedUIDs()
	if err != nil {
		return RunResult{}, err
	}

	var result RunResult
	result.Messages = len(uids)
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		messageResult, err := m.importIMAPMessage(ctx, c, uid, settings)
		if err != nil {
			result.Errors++
			m.log.Warn("mail import message failed", "uid", uid, "error", err)
			m.RecordAudit(ctx, "E-Mail-Verarbeitung fehlgeschlagen", fmt.Sprintf("UID %d: %s", uid, err), httpStatusError)
			continue
		}
		if messageResult.Rejected {
			result.Rejected++
			if err := c.DeleteMessage(uid); err != nil {
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
		if err := c.DeleteMessage(uid); err != nil {
			result.Errors++
			m.RecordAudit(ctx, "E-Mail-Löschen fehlgeschlagen", fmt.Sprintf("UID %d: %s", uid, err), httpStatusError)
			continue
		}
		result.Deleted++
		m.RecordAudit(ctx, "E-Mail verarbeitet", mailMessageAuditTarget(uid, messageResult), httpStatusOK)
	}

	return result, nil
}

func (m *Service) CheckSettings(ctx context.Context, settings document.MailImportSettings) error {
	settings = document.NormalizeMailImportSettings(settings)
	settings.Enabled = true
	if err := document.ValidateMailImportSettings(settings); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := m.openMailbox(settings, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Logout(); err != nil {
			m.log.Warn("mail import test logout failed", "error", err)
		}
	}()

	return nil
}

func (m *Service) importIMAPMessage(ctx context.Context, c mailbox, uid uint32, settings document.MailImportSettings) (MessageResult, error) {
	body, err := c.FetchMessage(uid)
	if err != nil {
		return MessageResult{}, err
	}
	return m.ImportMessage(ctx, body, settings.AllowedSenders)
}

func (m *Service) ImportMessage(ctx context.Context, r io.Reader, allowedSenders string) (MessageResult, error) {
	var result MessageResult
	message, err := mailimport.ImportAttachmentsFromMessage(r, allowedSenders, m.maxUploadBytes, func(att mailimport.Attachment) error {
		candidate, err := m.store.ReceiveReader(att.Filename, att.Reader, m.maxUploadBytes)
		if err != nil {
			result.Errors++
			result.Details = append(result.Details, att.Filename+": "+documentimport.UploadErrorMessage(err))
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
			m.log.Warn("mail attachment import failed", "filename", att.Filename, "error", importResult.Error)
			result.Details = append(result.Details, att.Filename+": "+documentimport.ImportErrorMessage(importResult.Error))
		default:
			result.Errors++
			result.Details = append(result.Details, att.Filename+": unbekannter Importstatus")
		}
		return nil
	}, func(att mailimport.Attachment) error {
		tempDir, err := m.store.EnsureDir("mailarchive-tmp")
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
			result.Details = append(result.Details, archive.Filename+": "+documentimport.UploadErrorMessage(err))
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
			m.log.Warn("mail archive import failed", "filename", att.Filename, "error", importResult.Error)
			result.Details = append(result.Details, archive.Filename+": "+documentimport.ImportErrorMessage(importResult.Error))
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

func (m *Service) RecordAudit(ctx context.Context, action, target string, status int) {
	if m.recordAudit != nil {
		m.recordAudit(ctx, action, target, status)
	}
}

func mailMessageAuditTarget(uid uint32, result MessageResult) string {
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
		if details := mailImportAuditDetails(result.Details); details != "" {
			parts = append(parts, "Details "+details)
		}
	}
	return strings.Join(parts, ": ")
}

func mailImportAuditDetails(details []string) string {
	clean := make([]string, 0, len(details))
	for _, detail := range details {
		detail = strings.TrimSpace(detail)
		if detail != "" {
			clean = append(clean, detail)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return truncateMailImportAuditDetail(strings.Join(clean, "; "))
}

func truncateMailImportAuditDetail(value string) string {
	if len(value) <= mailImportAuditDetailLimit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= mailImportAuditDetailLimit {
		return value
	}
	return string(runes[:mailImportAuditDetailLimit]) + "..."
}
