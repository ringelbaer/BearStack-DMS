package server

import (
	"errors"
	"net/http"
	"strconv"
)

func (s *Server) handlePersonParents(w http.ResponseWriter, r *http.Request) {
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	if r.Method == http.MethodGet {
		page, err := s.photos.People(r.Context(), id, 1, "", false, false)
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		_ = writeJSON(w, 200, page.Parents)
		return
	}

	if !s.parseFaceForm(w, r) {
		return
	}
	if !r.PostForm.Has("mother_id") || !r.PostForm.Has("father_id") {
		s.faceError(w, r, errors.New("Mutter und Vater müssen angegeben werden; 0 entfernt eine Zuordnung"))
		return
	}
	parse := func(key string) (int64, error) {
		v := r.PostForm.Get(key)
		if v == "" || v == "0" {
			return 0, nil
		}
		return faceID(v)
	}
	mother, err := parse("mother_id")
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	father, err := parse("father_id")
	if err == nil {
		err = s.photos.SetPersonParents(r.Context(), id, mother, father)
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	setAuditTarget(r, "person:"+strconv.FormatInt(id, 10))
	w.Header().Set("Cache-Control", "no-store")
	if wantsJSON(r) {
		_ = writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	redirectWithNotice(w, r, "/photos/people/"+strconv.FormatInt(id, 10), "Eltern gespeichert.")
}
