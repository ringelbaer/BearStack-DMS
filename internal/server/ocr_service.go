package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"bearstack/internal/document"
	"bearstack/internal/documentocr"
	"bearstack/internal/ocrservice"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type ocrProgressFunc = documentocr.ProgressFunc

func newOCRService(repo *repository.Repository, store *storage.Store, log *slog.Logger, invalidateDocumentCountCache func(), recordAuditLog func(context.Context, document.AuditLogEntry)) *ocrservice.Service {
	var repositoryPort ocrservice.Repository
	if repo != nil {
		repositoryPort = repo
	}
	var storagePort ocrservice.Store
	if store != nil {
		storagePort = store
	}
	return ocrservice.New(repositoryPort, storagePort, log, nil, invalidateDocumentCountCache, func(ctx context.Context, action string, job document.OCRJob, doc document.Document, status int, detail string) {
		parts := []string{documentAuditTargetFor(doc)}
		if job.LanguageLabel != "" {
			parts = append(parts, "OCR "+job.LanguageLabel)
		}
		if detail != "" {
			parts = append(parts, detail)
		}
		if recordAuditLog == nil {
			return
		}
		recordAuditLog(ctx, document.AuditLogEntry{
			Actor:  "system",
			Method: "OCR",
			Path:   fmt.Sprintf("/documents/%d", doc.ID),
			Route:  "documents/ocr",
			Action: action,
			Target: strings.Join(parts, ": "),
			Status: status,
		})
	})
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
