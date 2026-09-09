package server

import (
	"bearstack/internal/photos"
	"net/http"
)

func (s *Server) handleFacePersonSuggestions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	out, err := s.photos.SuggestPeopleForFace(r.Context(), id)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, out)
}
