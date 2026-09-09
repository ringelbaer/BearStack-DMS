package server

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	"bearstack/internal/photos"
)

func (s *Server) faceMergeError(w http.ResponseWriter, r *http.Request, err error) {
	if wantsJSON(r) || r.URL.Query().Get("format") == "json" {
		s.labelError(w, r, err)
		return
	}
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, photos.ErrLabelConflict):
		status = http.StatusConflict
		err = errors.New("Die Personengruppen haben sich geändert. Bitte die Vorschläge erneut laden und prüfen.")
	case errors.Is(err, photos.ErrLabelInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, photos.ErrAdminOnly()):
		status = http.StatusForbidden
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	}
	s.renderErrorWithReturn(w, r, status, err, "/photos/people/merge-suggestions")
}

func (s *Server) handleFaceMergeSuggestions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	suggestions, err := s.photos.FaceMergeSuggestions(r.Context(), 60)
	if err != nil {
		s.faceMergeError(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "json" || wantsJSON(r) {
		if suggestions == nil {
			suggestions = []photos.FaceMergeSuggestion{}
		}
		_ = writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
		return
	}
	s.render(w, r, "face_merges.html", PageData{Title: "Ähnliche Personengruppen", Active: "photos", Assets: photoPageAssets(false), FaceMergeSuggestions: suggestions, Notice: r.URL.Query().Get("notice")})
}

func (s *Server) handleFaceMergeSuggestionAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	id, err := faceID(r.PathValue("id"))
	sourceRevision, sourceErr := faceID(r.PostForm.Get("source_revision"))
	targetRevision, targetErr := faceID(r.PostForm.Get("target_revision"))
	if err != nil || sourceErr != nil || targetErr != nil {
		s.faceMergeError(w, r, photos.ErrLabelInvalid)
		return
	}
	switch r.PathValue("action") {
	case "accept":
		err = s.photos.AcceptFaceMergeSuggestion(r.Context(), id, sourceRevision, targetRevision)
	case "reject":
		err = s.photos.RejectFaceMergeSuggestion(r.Context(), id, sourceRevision, targetRevision)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.faceMergeError(w, r, err)
		return
	}
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	notice := "Personengruppen zusammengeführt."
	if r.PathValue("action") == "reject" {
		notice = "Die Gruppen bleiben getrennt."
	}
	redirectWithNotice(w, r, "/photos/people/merge-suggestions", notice)
}
