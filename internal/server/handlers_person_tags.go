package server

import "net/http"

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
