package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"bearstack/internal/photos"
)

func (s *Server) handleResetIgnoredDirectoryFaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	directory := r.PostForm.Get("path")
	setAuditTarget(r, directory)
	count, err := s.photos.ResetIgnoredDirectoryFaces(r.Context(), directory)
	if err != nil {
		if errors.Is(err, photos.ErrPathEscapesRoot()) {
			err = photos.ErrLabelInvalid
		}
		if wantsJSON(r) {
			s.labelError(w, r, err)
		} else if errors.Is(err, photos.ErrLabelInvalid) {
			s.renderError(w, r, http.StatusBadRequest, errors.New("Die Aktion ist erst in einem echten Fotoordner ab Ebene 2 verfügbar."))
		} else {
			s.renderPhotoError(w, r, err)
		}
		return
	}
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored": count})
		return
	}
	redirectWithNotice(w, r, photoPageURL(url.Values{"path": {directory}}),
		fmt.Sprintf("%d ignorierte Gesichter im Ordner und seinen Unterordnern zurückgesetzt. Benannte Gesichter bleiben unverändert.", count))
}
