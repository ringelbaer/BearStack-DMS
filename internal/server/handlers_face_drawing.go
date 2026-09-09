package server

import (
	"net/http"
	"strconv"

	"bearstack/internal/photos"
)

func (s *Server) handleFaceDrawingImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	data, revision, err := s.photos.FaceDrawingImage(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Photo-Source-Revision", revision)
	_, _ = w.Write(data)
}

func (s *Server) handleAddDrawnFace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	var box photos.FaceRegion
	for key, value := range map[string]*float64{"x": &box.X, "y": &box.Y, "width": &box.Width, "height": &box.Height} {
		var err error
		*value, err = strconv.ParseFloat(r.PostForm.Get(key), 64)
		if err != nil || len(r.PostForm[key]) != 1 {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
	}
	var target int64
	if raw := r.PostForm.Get("target"); raw != "" {
		var err error
		target, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
	}
	path := r.PostForm.Get("path")
	setAuditTarget(r, path)
	id, err := s.photos.AddDrawnFace(r.Context(), path, r.PostForm.Get("source_revision"), box, target, r.PostForm.Get("name"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "face_id": id})
}
