package server

import (
	"fmt"
	"net/http"
)

func (s *Server) handlePersonTags(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	tags, err := s.photos.SetPersonTags(r.Context(), id, r.PostForm["tags"])
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, "person:"+r.PathValue("id"))
	w.Header().Set("Cache-Control", "no-store")
	_ = writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handlePeopleTagsAdd(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	ids := make([]int64, 0, len(r.PostForm["ids"]))
	for _, raw := range r.PostForm["ids"] {
		id, err := faceID(raw)
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		ids = append(ids, id)
	}
	count, err := s.photos.AddPeopleTags(r.Context(), ids, r.PostForm["tags"])
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, fmt.Sprintf("%d Personen", count))
	w.Header().Set("Cache-Control", "no-store")
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
}
