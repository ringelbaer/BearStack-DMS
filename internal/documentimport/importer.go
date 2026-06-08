// Datei importiert Dokumentdateien in den Speicher und extrahiert dabei grundlegende Metadaten.
package documentimport

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/documentconvert"
	"bearstack/internal/storage"
	"bearstack/internal/textmeta"
)

type Repository interface {
	FindActiveByChecksum(context.Context, string) (document.Document, bool, error)
	CreateDocument(context.Context, document.Document) (int64, error)
	UpdateSearchText(context.Context, int64, string, string, int) error
}

type Store interface {
	Commit(storage.Candidate, time.Time) (string, error)
	Delete(string) error
	RemoveTemp(storage.Candidate)
	Resolve(string) (string, error)
}

type Thumbnailer interface {
	Ensure(context.Context, document.Document) error
}

type Importer struct {
	Repo        Repository
	Store       Store
	Log         *slog.Logger
	AfterCreate func(document.Document)
}

type Result struct {
	Created   *Created
	Duplicate *Duplicate
	Error     error
}

type Created struct {
	Document document.Document
}

type Duplicate struct {
	Filename string
	Existing document.Document
}

func NewImporter(repo Repository, store Store, log *slog.Logger, afterCreate func(document.Document)) Importer {
	return Importer{
		Repo:        repo,
		Store:       store,
		Log:         log,
		AfterCreate: afterCreate,
	}
}

func (i Importer) ImportCandidate(ctx context.Context, candidate storage.Candidate, uploadWay string) Result {
	return i.ImportCandidateWithTags(ctx, candidate, uploadWay, nil)
}

func (i Importer) ImportCandidateWithTags(ctx context.Context, candidate storage.Candidate, uploadWay string, tags []string) Result {
	existing, ok, err := i.Repo.FindActiveByChecksum(ctx, candidate.SHA256)
	if err != nil {
		i.Store.RemoveTemp(candidate)
		return Result{Error: err}
	}
	if ok {
		i.Store.RemoveTemp(candidate)
		return Result{Duplicate: &Duplicate{
			Filename: candidate.OriginalName,
			Existing: existing,
		}}
	}

	title, documentDate := textmeta.FromFilename(candidate.OriginalName)
	now := time.Now().UTC()
	storedPath, err := i.Store.Commit(candidate, now)
	if err != nil {
		i.Store.RemoveTemp(candidate)
		return Result{Error: err}
	}

	doc := document.Document{
		OriginalName:      candidate.OriginalName,
		StoredPath:        storedPath,
		UploadWay:         uploadWay,
		Title:             title,
		Tags:              append([]string(nil), tags...),
		MIMEType:          candidate.MIMEType,
		SizeBytes:         candidate.SizeBytes,
		SHA256:            candidate.SHA256,
		DocumentDate:      documentDate,
		UploadedAt:        now,
		UpdatedAt:         now,
		ContentTextSource: document.ContentTextSourceNone,
		SearchVersion:     document.CurrentSearchVersion,
	}
	id, err := i.Repo.CreateDocument(ctx, doc)
	if err != nil {
		if deleteErr := i.Store.Delete(storedPath); deleteErr != nil {
			logWarn(i.Log, "failed to cleanup stored file after document creation error", "path", storedPath, "error", deleteErr)
		}
		return Result{Error: err}
	}
	doc.ID = id
	if i.AfterCreate != nil {
		i.AfterCreate(doc)
	}

	return Result{Created: &Created{Document: doc}}
}

func ExtractDocumentText(path, mimeType string) (string, string, error) {
	if mimeType == "application/pdf" {
		text, err := textmeta.ExtractPDFText(path, 10<<20)
		if err == nil && strings.TrimSpace(text) != "" {
			return text, document.ContentTextSourcePDF, nil
		}
		if err != nil {
			fallback, fallbackErr := extractRawText(path)
			if fallbackErr != nil {
				return "", document.ContentTextSourceNone, err
			}
			if strings.TrimSpace(fallback) == "" {
				return "", document.ContentTextSourceNone, nil
			}
			return fallback, document.ContentTextSourceRaw, nil
		}
		return "", document.ContentTextSourceNone, nil
	}
	if strings.HasPrefix(mimeType, "image/") {
		return "", document.ContentTextSourceNone, nil
	}
	if documentconvert.IsLibreOfficeDocument(path, mimeType) {
		text, err := documentconvert.ExtractText(context.Background(), path)
		if err != nil {
			return "", document.ContentTextSourceNone, err
		}
		if strings.TrimSpace(text) == "" {
			return "", document.ContentTextSourceNone, nil
		}
		return text, document.ContentTextSourceFile, nil
	}
	if documentconvert.IsPlainTextDocument(path, mimeType) {
		text, err := extractRawText(path)
		if err != nil {
			return "", document.ContentTextSourceNone, err
		}
		if strings.TrimSpace(text) == "" {
			return "", document.ContentTextSourceNone, nil
		}
		return text, document.ContentTextSourceFile, nil
	}
	text, err := extractRawText(path)
	if err != nil {
		return "", document.ContentTextSourceNone, err
	}
	if strings.TrimSpace(text) == "" {
		return "", document.ContentTextSourceNone, nil
	}
	return text, document.ContentTextSourceRaw, nil
}

func extractRawText(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return textmeta.ExtractPlainText(file, 2<<20), nil
}

func logWarn(log *slog.Logger, message string, args ...any) {
	if log != nil {
		log.Warn(message, args...)
	}
}
