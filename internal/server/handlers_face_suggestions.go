package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/photos"
)

func (s *Server) handleFacePersonSuggestions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "application/x-ndjson") {
		out, err := s.photos.SuggestPeopleForFace(r.Context(), id)
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		_ = writeJSON(w, http.StatusOK, out)
		return
	}
	control := http.NewResponseController(w)
	started := false
	send := func(people photos.PeopleSuggestions, done bool, message string) error {
		// A stalled browser must not hold the shared reference cache indefinitely.
		_ = control.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		event := struct {
			photos.PeopleSuggestions
			Done  bool   `json:"done"`
			Error string `json:"error,omitempty"`
		}{people, done, message}
		if err := json.NewEncoder(w).Encode(event); err != nil {
			return err
		}
		return control.Flush()
	}
	out, err := s.photos.SuggestPeopleForFaceStream(r.Context(), id, func(people photos.PeopleSuggestions) error { return send(people, false, "") })
	if err != nil {
		if !started {
			s.labelError(w, r, err)
		} else if r.Context().Err() == nil {
			_ = send(photos.PeopleSuggestions{People: []photos.PersonSuggestion{}}, true, "Gesichtsabgleich abgebrochen. Bitte erneut versuchen.")
		}
		return
	}
	_ = send(out, true, "")
}
