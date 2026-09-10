package server

import (
	"bearstack/internal/photos"
	"net/http"
)

func (s *Server) handleLabelMergeGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	after, err := labelInt(r, "after", 1<<62)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	upper, err := labelInt(r, "upper", 1<<62)
	named := r.URL.Query().Get("include_named")
	if err != nil || r.URL.Query().Get("upper") == "" || (named != "" && named != "0" && named != "1") {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	out, err := s.photos.LabelMergeGroups(r.Context(), after, upper, named == "1")
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, out)
}
