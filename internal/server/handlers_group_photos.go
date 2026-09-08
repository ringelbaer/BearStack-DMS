package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"bearstack/internal/photos"
)

func groupPhotoMinimum(raw string) (int, error) {
	if raw == "" {
		return photos.DefaultGroupPhotoMinimum, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > photos.MaxGroupPhotoMinimum {
		return 0, photos.ErrLabelInvalid
	}
	return value, nil
}

func (s *Server) handleGroupPhotoImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	face, err := s.photos.Face(r.Context(), id)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	// Reuse the gallery cache and its EXIF-oriented, proportional preview. One
	// current photo is loaded; original files need not be transferred at full size.
	path, err := s.photos.Thumbnail(r.Context(), face.Path, 1600)
	if err == nil {
		if _, err = s.photos.Face(r.Context(), id); err != nil {
			s.labelError(w, r, err)
			return
		}
		if err := s.servePhotoThumbnailFile(w, r, path, photoMediaCacheNoStore); err == nil {
			return
		}
	}
	// Deployments without a thumbnail backend can still use the editor.
	if _, err = s.photos.Face(r.Context(), id); err != nil {
		s.labelError(w, r, err)
		return
	}
	s.servePhotoMedia(w, r, face.Path, photoMediaCacheNoStore)
}

func (s *Server) handleGroupPhotos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	minimum, err := groupPhotoMinimum(r.URL.Query().Get("min"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	page := photos.GroupPhotosPage{Minimum: minimum}
	if r.URL.Query().Has("path") {
		var photo photos.GroupPhoto
		photo, err = s.photos.GroupPhoto(r.Context(), r.URL.Query().Get("path"))
		page.Photo = &photo
	} else {
		page.Photo, err = s.photos.NextGroupPhoto(r.Context(), r.URL.Query().Get("after"), minimum)
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if wantsJSON(r) || r.URL.Query().Get("format") == "json" {
		_ = writeJSON(w, http.StatusOK, page)
		return
	}
	s.render(w, r, "group_photos.html", PageData{Title: "Gruppenbilder", Active: "photos", GroupPhotos: page})
}

func (s *Server) handleGroupPhotoIgnore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	minimum, err := groupPhotoMinimum(r.PostForm.Get("min"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	path := r.PostForm.Get("path")
	setAuditTarget(r, path)
	var count int
	_, single := r.PostForm["face_id"]
	if single {
		id, parseErr := faceID(r.PostForm.Get("face_id"))
		if parseErr != nil || len(r.PostForm["face_id"]) != 1 {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
		count, err = s.photos.IgnoreGroupPhotoFace(r.Context(), path, r.PostForm.Get("revision"), id)
	} else {
		count, err = s.photos.IgnoreGroupPhoto(r.Context(), path, r.PostForm.Get("revision"))
	}
	if errors.Is(err, photos.ErrGroupPhotoChanged) {
		_ = writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "conflict"})
		return
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if wantsJSON(r) {
		// A separate GET refreshes this photo or advances the queue. If it fails,
		// the client retries that GET without repeating the successful mutation.
		_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": count})
		return
	}
	query := url.Values{"min": {strconv.Itoa(minimum)}, "after": {path}}
	if single {
		query.Del("after")
		query.Set("path", path)
	}
	http.Redirect(w, r, "/photos/people/groups?"+query.Encode(), http.StatusSeeOther)
}
