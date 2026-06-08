// Datei fuehrt Nachverarbeitungsschritte fuer importierte Dokumente aus, etwa Text- und Metadatenaufbereitung.
package documentimport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/documentconvert"
	"bearstack/internal/textmeta"
)

const (
	postImportBatchSize    = 25
	postImportPollInterval = 30 * time.Second
)

type pendingPostImportRepository interface {
	PostImportPendingDocuments(context.Context, int) ([]document.Document, error)
	MarkPostImportComplete(context.Context, int64) error
}

type postImportAttemptRepository interface {
	MarkPostImportAttempted(context.Context, int64) error
}

type officePreviewer interface {
	EnsureOfficePreview(context.Context, document.Document) (string, error)
}

type PostProcessor struct {
	repo                     Repository
	store                    Store
	thumbnails               Thumbnailer
	log                      *slog.Logger
	invalidateDocumentCounts func()
	wake                     chan struct{}
}

func NewPostProcessor(repo Repository, store Store, thumbnails Thumbnailer, log *slog.Logger, invalidateDocumentCounts func()) *PostProcessor {
	return &PostProcessor{
		repo:                     repo,
		store:                    store,
		thumbnails:               thumbnails,
		log:                      log,
		invalidateDocumentCounts: invalidateDocumentCounts,
		wake:                     make(chan struct{}, 1),
	}
}

func (p *PostProcessor) Enqueue(doc document.Document) {
	if p == nil {
		return
	}
	if p.wake == nil {
		go func() {
			_ = p.Process(context.Background(), doc)
		}()
		return
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *PostProcessor) Run(ctx context.Context) {
	if p == nil {
		return
	}
	ticker := time.NewTicker(postImportPollInterval)
	defer ticker.Stop()
	for {
		attempted, err := p.ProcessPending(ctx, postImportBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logWarn(p.log, "document post-import pending scan failed", "error", err)
		}
		if err == nil && attempted > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-ticker.C:
		}
	}
}

func (p *PostProcessor) ProcessPending(ctx context.Context, limit int) (int, error) {
	if p == nil || p.repo == nil {
		return 0, nil
	}
	pendingRepo, ok := p.repo.(pendingPostImportRepository)
	if !ok {
		return 0, nil
	}
	docs, err := pendingRepo.PostImportPendingDocuments(ctx, limit)
	if err != nil {
		return 0, err
	}
	attemptRepo, _ := pendingRepo.(postImportAttemptRepository)
	errs := make([]error, 0)
	for _, doc := range docs {
		if attemptRepo != nil {
			if err := attemptRepo.MarkPostImportAttempted(ctx, doc.ID); err != nil {
				errs = append(errs, fmt.Errorf("mark document %d attempted: %w", doc.ID, err))
				continue
			}
		}
		if err := p.Process(ctx, doc); err != nil {
			errs = append(errs, fmt.Errorf("document %d: %w", doc.ID, err))
			continue
		}
		if err := pendingRepo.MarkPostImportComplete(ctx, doc.ID); err != nil {
			errs = append(errs, fmt.Errorf("mark document %d complete: %w", doc.ID, err))
		}
	}
	return len(docs), errors.Join(errs...)
}

func (p *PostProcessor) Process(ctx context.Context, doc document.Document) error {
	if p == nil {
		return nil
	}
	if documentconvert.IsLibreOfficeDocument(doc.OriginalName, doc.MIMEType) {
		return p.processOfficeDocument(ctx, doc)
	}
	return p.processGeneric(ctx, doc)
}

func (p *PostProcessor) processGeneric(ctx context.Context, doc document.Document) error {
	errs := make([]error, 0, 2)
	if err := p.updateSearchText(ctx, doc); err != nil {
		logWarn(p.log, "document text extraction failed", "id", doc.ID, "filename", doc.OriginalName, "error", err)
		errs = append(errs, err)
	}
	if p.thumbnails != nil {
		if err := p.thumbnails.Ensure(ctx, doc); err != nil {
			logWarn(p.log, "thumbnail generation failed", "id", doc.ID, "filename", doc.OriginalName, "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *PostProcessor) processOfficeDocument(ctx context.Context, doc document.Document) error {
	previewer, ok := p.thumbnails.(officePreviewer)
	if !ok {
		return p.processGeneric(ctx, doc)
	}
	previewPath, err := previewer.EnsureOfficePreview(ctx, doc)
	if err != nil {
		if errors.Is(err, documentconvert.ErrLibreOfficeUnavailable) {
			return nil
		}
		logWarn(p.log, "document preview generation failed", "id", doc.ID, "filename", doc.OriginalName, "error", err)
		return err
	}
	errs := make([]error, 0, 2)
	if err := p.updateSearchTextFromPDF(ctx, doc, previewPath, document.ContentTextSourceFile); err != nil {
		logWarn(p.log, "document text extraction failed", "id", doc.ID, "filename", doc.OriginalName, "error", err)
		errs = append(errs, err)
	}
	if p.thumbnails != nil {
		if err := p.thumbnails.Ensure(ctx, doc); err != nil {
			logWarn(p.log, "thumbnail generation failed", "id", doc.ID, "filename", doc.OriginalName, "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *PostProcessor) updateSearchText(ctx context.Context, doc document.Document) error {
	if p.repo == nil || p.store == nil {
		return nil
	}
	path, err := p.store.Resolve(doc.StoredPath)
	if err != nil {
		return err
	}
	text, source, err := ExtractDocumentText(path, doc.MIMEType)
	if err != nil {
		if errors.Is(err, documentconvert.ErrLibreOfficeUnavailable) {
			return nil
		}
		return err
	}
	if text == "" && source == document.ContentTextSourceNone {
		return nil
	}
	if err := p.repo.UpdateSearchText(ctx, doc.ID, text, source, document.CurrentSearchVersion); err != nil {
		return err
	}
	if p.invalidateDocumentCounts != nil {
		p.invalidateDocumentCounts()
	}
	return nil
}

func (p *PostProcessor) updateSearchTextFromPDF(ctx context.Context, doc document.Document, path, source string) error {
	if p.repo == nil {
		return nil
	}
	text, err := textmeta.ExtractPDFText(path, 10<<20)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := p.repo.UpdateSearchText(ctx, doc.ID, text, source, document.CurrentSearchVersion); err != nil {
		return err
	}
	if p.invalidateDocumentCounts != nil {
		p.invalidateDocumentCounts()
	}
	return nil
}
