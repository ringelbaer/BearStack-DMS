package server

import (
	"context"
	"log/slog"

	"bearstack/internal/document"
	"bearstack/internal/mailservice"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type mailImportRunResult = mailservice.RunResult

const (
	httpStatusOK    = 200
	httpStatusError = 500
)

func newMailImportService(maxUploadBytes int64, repo *repository.Repository, store *storage.Store, log *slog.Logger, importer documentImporter, recordAuditLog func(context.Context, document.AuditLogEntry)) *mailservice.Service {
	var repositoryPort mailservice.Repository
	if repo != nil {
		repositoryPort = repo
	}
	var storagePort mailservice.Store
	if store != nil {
		storagePort = store
	}
	return mailservice.New(maxUploadBytes, repositoryPort, storagePort, log, importer, func(ctx context.Context, action, target string, status int) {
		if recordAuditLog != nil {
			recordAuditLog(ctx, document.AuditLogEntry{Actor: "system", Method: "IMAP", Path: "/settings/mail-import", Route: "settings/mail-import", Action: action, Target: target, Status: status})
		}
	})
}
