// Datei erzeugt und transportiert Request-IDs fuer Logging und Fehlernachverfolgung.
package server

import (
	"database/sql"
	"net/http"
	"strconv"

	"bearstack/internal/document"
)

func (s *Server) documentFromRequest(r *http.Request) (document.Document, error) {
	id, err := idFromRequest(r)
	if err != nil {
		return document.Document{}, err
	}
	return s.repo.GetDocument(r.Context(), id)
}

func (s *Server) fileDocumentFromRequest(r *http.Request) (document.Document, error) {
	id, err := idFromRequest(r)
	if err != nil {
		return document.Document{}, err
	}
	return s.repo.GetDocumentFile(r.Context(), id)
}

func (s *Server) tagFromRequest(r *http.Request) (document.Tag, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return document.Tag{}, sql.ErrNoRows
	}
	return s.repo.GetTag(r.Context(), id)
}

func idFromRequest(r *http.Request) (int64, error) {
	return positiveIDFromPath(r, "id")
}

func positiveIDFromPath(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, sql.ErrNoRows
	}
	return id, nil
}

func linkedIDFromRequest(r *http.Request) (int64, error) {
	return positiveIDFromPath(r, "linkedID")
}
