package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"bearstack/internal/photos"
)

func (s *Server) handlePersonDetails(w http.ResponseWriter, r *http.Request) {
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodGet {
		details, err := s.photos.PersonDetails(r.Context(), id)
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		_ = writeJSON(w, http.StatusOK, details)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var input photos.PersonDetailsInput
	if err = dec.Decode(&input); err == nil {
		var extra any
		if dec.Decode(&extra) != io.EOF {
			err = errors.New("nur ein JSON-Objekt erlaubt")
		}
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	if err = s.photos.SetPersonDetails(r.Context(), id, input); err != nil {
		if errors.Is(err, photos.ErrPersonDetailsConflict) {
			_ = writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.faceError(w, r, err)
		return
	}
	setAuditTarget(r, "person:"+r.PathValue("id"))
	_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
