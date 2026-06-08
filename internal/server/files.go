// Datei liefert gespeicherte Dateien aus und behandelt sichere Download- und Vorschaupfade.
package server

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"bearstack/internal/document"
)

func (s *Server) serveStoredFile(w http.ResponseWriter, r *http.Request, doc document.Document, disposition string) {
	path, err := s.store.Resolve(doc.StoredPath)
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

	contentType := doc.MIMEType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(doc.OriginalName))
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", contentDisposition(disposition, doc.OriginalName))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, doc.OriginalName, info.ModTime(), file)
}

func (s *Server) serveFilePath(w http.ResponseWriter, r *http.Request, path, filename, contentType, disposition string) {
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
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", contentDisposition(disposition, filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
}
