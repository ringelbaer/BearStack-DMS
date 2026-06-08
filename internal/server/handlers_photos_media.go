// Datei verarbeitet Foto-Medienauslieferung und Media-Info-API-Antworten.
package server

import (
	"encoding/json"
	"net/http"
	"os"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoMedia(w http.ResponseWriter, r *http.Request) {
	s.servePhotoMedia(w, r, r.URL.Query().Get("path"), photoMediaCacheDefault)
}

func (s *Server) servePhotoMedia(w http.ResponseWriter, r *http.Request, photoPath string, cacheMode photoMediaCacheMode) {
	if s.blockAdminOnlyPhotoMedia(w, r, photoPath) {
		return
	}
	media, err := s.photos.MediaContext(r.Context(), photoPath)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	path, err := s.photos.Resolve(media.Path)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", media.MIMEType)
	setPhotoResponseCacheControl(w, r, 86400, cacheMode)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, media.Name, info.ModTime(), file)
}

func (s *Server) handlePhotoMediaInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handlePhotoMediaInfoBatch(w, r)
		return
	}
	photoPath := r.URL.Query().Get("path")
	if s.blockAdminOnlyPhotoMedia(w, r, photoPath) {
		return
	}
	media, err := s.photos.MediaContext(r.Context(), photoPath)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	mediaView := photoMediaView(media, settings)
	if mediaView.ThumbURL != "" {
		ready := s.photos.CachedThumbnailsReadyForMediaContext(r.Context(), []photos.Media{media}, settings.ThumbnailSize)
		mediaView.ThumbReady = ready[media.Path]
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Media photoMediaAPIResponse `json:"media"`
	}{Media: photoMediaAPIResponseFrom(mediaView)}); err != nil {
		s.log.Warn("photo media info response failed", "path", photoPath, "error", err)
	}
}

type photoMediaInfoBatchRequest struct {
	Paths []string `json:"paths"`
}

func (s *Server) handlePhotoMediaInfoBatch(w http.ResponseWriter, r *http.Request) {
	var payload photoMediaInfoBatchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil {
		s.renderJSONError(w, http.StatusBadRequest, "ungültige Medien-Anfrage")
		return
	}
	if len(payload.Paths) > 100 {
		payload.Paths = payload.Paths[:100]
	}
	paths := make([]string, 0, len(payload.Paths))
	seen := map[string]struct{}{}
	for _, path := range payload.Paths {
		clean, err := photos.CleanPath(path)
		if err != nil {
			s.renderJSONError(w, http.StatusBadRequest, "ungültiger Fotopfad")
			return
		}
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}
	if err := s.photoAccessPolicy(s.requestIsPhotoAdmin(r)).RequireMediaBatch(paths); s.blockPhotoAccess(w, r, err) {
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	mediaItems := make([]photos.Media, 0, len(paths))
	mediaViews := make([]PhotoMediaView, 0, len(paths))
	for _, path := range paths {
		media, err := s.photos.MediaContext(r.Context(), path)
		if err != nil {
			s.renderPhotoError(w, r, err)
			return
		}
		mediaItems = append(mediaItems, media)
		mediaViews = append(mediaViews, photoMediaView(media, settings))
	}
	ready := s.photos.CachedThumbnailsReadyForMediaContext(r.Context(), mediaItems, settings.ThumbnailSize)
	for i := range mediaViews {
		mediaViews[i].ThumbReady = ready[mediaViews[i].Path]
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Media []photoMediaAPIResponse `json:"media"`
	}{Media: photoMediaAPIResponsesFrom(mediaViews)}); err != nil {
		s.log.Warn("photo media info batch response failed", "error", err)
	}
}
