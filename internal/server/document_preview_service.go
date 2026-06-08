// Datei erzeugt abgeleitete PDF-Vorschauen fuer Office- und Textdokumente.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"bearstack/internal/document"
	"bearstack/internal/documentconvert"
	"bearstack/internal/fsutil"
)

var officePreviewJobs = make(chan struct{}, 1)

func (s *Server) ensureDocumentOfficePreview(ctx context.Context, doc document.Document) (string, error) {
	if previewer, ok := s.thumbnailService().(interface {
		EnsureOfficePreview(context.Context, document.Document) (string, error)
	}); ok {
		return previewer.EnsureOfficePreview(ctx, doc)
	}
	return ensureDocumentOfficePreview(ctx, s.store, doc)
}

func (t thumbnailService) EnsureOfficePreview(ctx context.Context, doc document.Document) (string, error) {
	return ensureDocumentOfficePreview(ctx, t.store, doc)
}

func ensureDocumentOfficePreview(ctx context.Context, store interface {
	Resolve(string) (string, error)
	EnsureDir(string) (string, error)
}, doc document.Document) (string, error) {
	if doc.ID <= 0 {
		return "", fmt.Errorf("document id is missing")
	}
	previewRel := documentOfficePreviewPath(doc.ID)
	previewAbs, err := store.Resolve(previewRel)
	if err != nil {
		return "", err
	}
	if fsutil.FileHasContent(previewAbs) {
		return previewAbs, nil
	}
	release, err := acquireOfficePreviewJob(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	if fsutil.FileHasContent(previewAbs) {
		return previewAbs, nil
	}
	if _, err := store.EnsureDir(filepath.ToSlash(filepath.Dir(previewRel))); err != nil {
		return "", err
	}
	source, err := store.Resolve(doc.StoredPath)
	if err != nil {
		return "", err
	}
	if documentconvert.IsPlainTextDocument(doc.OriginalName, doc.MIMEType) {
		err = documentconvert.ConvertPlainTextToPDF(source, previewAbs)
	} else {
		err = documentconvert.ConvertToPDF(ctx, source, previewAbs)
	}
	if err != nil {
		_ = os.Remove(previewAbs)
		return "", err
	}
	return previewAbs, nil
}

func acquireOfficePreviewJob(ctx context.Context) (func(), error) {
	select {
	case officePreviewJobs <- struct{}{}:
		return func() { <-officePreviewJobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func documentOfficePreviewPath(id int64) string {
	return filepath.ToSlash(filepath.Join(".previews", fmt.Sprintf("%d.pdf", id)))
}
