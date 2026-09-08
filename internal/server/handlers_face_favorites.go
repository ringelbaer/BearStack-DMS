package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"bearstack/internal/photos"
)

func (s *Server) handleFaceFavorite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if r.Method == http.MethodGet {
		face, err := s.photos.Face(r.Context(), id)
		if err == nil && face.Ignored {
			err = sql.ErrNoRows
		}
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		_ = writeJSON(w, http.StatusOK, photos.FaceFavorite{ID: face.ID, PersonID: face.PersonID, Favorite: face.Favorite})
		return
	}
	var input struct {
		PersonID int64 `json:"person_id"`
		Favorite *bool `json:"favorite"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.Method == http.MethodPut {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
	} else {
		if r.ParseForm() != nil {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
		input.PersonID, _ = strconv.ParseInt(r.PostForm.Get("person_id"), 10, 64)
		if raw := r.PostForm.Get("favorite"); raw == "1" || raw == "0" {
			favorite := raw == "1"
			input.Favorite = &favorite
		}
	}
	if input.PersonID <= 0 || input.Favorite == nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	setAuditTarget(r, "Gesicht:"+strconv.FormatInt(id, 10))
	out, err := s.photos.SetFaceFavorite(r.Context(), id, input.PersonID, *input.Favorite)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if r.Method == http.MethodPost {
		page := boundedInt(r.PostForm.Get("page"), 1, 1, 1000000)
		http.Redirect(w, r, "/photos/people/"+strconv.FormatInt(out.PersonID, 10)+"?page="+strconv.Itoa(page), http.StatusSeeOther)
		return
	}
	_ = writeJSON(w, http.StatusOK, out)
}
