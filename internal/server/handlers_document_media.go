// Datei behandelt Medien- und Dateianfragen fuer Dokumentanhaenge und Vorschauen.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"

	"bearstack/internal/documentconvert"
)

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	doc, err := s.fileDocumentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.serveStoredFile(w, r, doc, "attachment")
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	doc, err := s.fileDocumentFromRequest(r)
	if err != nil {
		s.renderPreviewHTTPError(w, err)
		return
	}
	if doc.MIMEType == "application/pdf" || strings.HasPrefix(doc.MIMEType, "image/") {
		s.serveStoredFile(w, r, doc, "inline")
		return
	}
	if !documentconvert.IsPreviewDocument(doc.OriginalName, doc.MIMEType) {
		renderPreviewError(w, http.StatusUnsupportedMediaType, "Vorschau ist fuer diesen Dateityp nicht verfuegbar.")
		return
	}
	previewPath, err := s.ensureDocumentOfficePreview(r.Context(), doc)
	if err != nil {
		status := http.StatusInternalServerError
		message := "Vorschau konnte nicht erstellt werden."
		if errors.Is(err, documentconvert.ErrLibreOfficeUnavailable) {
			status = http.StatusServiceUnavailable
			message = "Vorschau ist aktuell nicht verfuegbar, weil LibreOffice fehlt."
		}
		renderPreviewError(w, status, message)
		return
	}
	s.serveFilePath(w, r, previewPath, doc.OriginalName+".pdf", "application/pdf", "inline")
}

func (s *Server) renderPreviewHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		renderPreviewError(w, http.StatusNotFound, "Dokument nicht gefunden.")
		return
	}
	renderPreviewError(w, http.StatusInternalServerError, "Dokument konnte nicht geladen werden.")
}

func renderPreviewError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Vorschau nicht verfuegbar</title>
<style>
:root { color-scheme: dark; }
body {
  align-items: center;
  background: #111820;
  color: #e8eef3;
  display: flex;
  font: 16px/1.5 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  justify-content: center;
  margin: 0;
  min-height: 100vh;
  padding: 24px;
}
main {
  max-width: 560px;
  text-align: center;
}
h1 {
  font-size: 1.1rem;
  margin: 0 0 8px;
}
p {
  color: #b8c4cf;
  margin: 0;
}
</style>
</head>
<body>
<main>
<h1>Vorschau nicht verfuegbar</h1>
<p>%s</p>
</main>
</body>
</html>`, html.EscapeString(message))
}

func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	doc, err := s.fileDocumentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if doc.ThumbnailPath == "" {
		if err := s.ensureDocumentThumbnail(r.Context(), doc); err != nil {
			s.log.Warn("thumbnail generation failed", "id", doc.ID, "error", err)
		}
		refreshed, err := s.repo.GetDocumentFile(r.Context(), doc.ID)
		if err != nil {
			s.renderHTTPError(w, r, err)
			return
		}
		doc = refreshed
	}
	if doc.ThumbnailPath == "" {
		s.renderError(w, r, http.StatusNotFound, errors.New("kein Vorschaubild verfügbar"))
		return
	}

	path, err := s.store.Resolve(doc.ThumbnailPath)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, fmt.Sprintf("%d-thumbnail.jpg", doc.ID), info.ModTime(), file)
}
