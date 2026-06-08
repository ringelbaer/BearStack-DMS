// Datei verarbeitet Foto-Thumbnails, Thumbnail-Status und Queue-Anstoesse.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoThumbnail(w http.ResponseWriter, r *http.Request) {
	size := parsePositiveInt(r.URL.Query().Get("size"), photos.DefaultThumbnailSize)
	photoPath := r.URL.Query().Get("path")
	s.servePhotoThumbnail(w, r, photoPath, size, photoMediaCacheDefault)
}

func (s *Server) servePhotoThumbnail(w http.ResponseWriter, r *http.Request, photoPath string, size int, cacheMode photoMediaCacheMode) {
	cachedPath, cached, err := s.photos.CachedThumbnailContext(r.Context(), photoPath, size, s.requestIsPhotoAdmin(r))
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return
	}
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	if cached {
		if err := s.servePhotoThumbnailFile(w, r, cachedPath, cacheMode); err == nil {
			return
		}
		// The index may report a generated thumbnail while the file was removed
		// externally. Regenerate on demand and retry once.
		if s.blockAdminOnlyPhotoMedia(w, r, photoPath) {
			return
		}
		path, err := s.photos.Thumbnail(r.Context(), photoPath, size)
		if err != nil {
			s.renderPhotoError(w, r, err)
			return
		}
		if err := s.servePhotoThumbnailFile(w, r, path, cacheMode); err != nil {
			s.renderHTTPError(w, r, err)
		}
		return
	}
	if s.blockAdminOnlyPhotoMedia(w, r, photoPath) {
		return
	}
	readyBefore := s.photos.CachedThumbnailReady(photoPath, size)
	path, err := s.photos.Thumbnail(r.Context(), photoPath, size)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	if !readyBefore {
		s.invalidatePhotoStatisticsCache()
	}
	if err := s.servePhotoThumbnailFile(w, r, path, cacheMode); err != nil {
		s.renderHTTPError(w, r, err)
	}
}

func (s *Server) servePhotoThumbnailFile(w http.ResponseWriter, r *http.Request, path string, cacheMode photoMediaCacheMode) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "image/webp")
	setPhotoResponseCacheControl(w, r, 604800, cacheMode)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
	return nil
}

func setPhotoResponseCacheControl(w http.ResponseWriter, r *http.Request, fallbackMaxAge int, cacheMode photoMediaCacheMode) {
	if cacheMode == photoMediaCacheNoStore {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	setPhotoMediaCacheControl(w, r, fallbackMaxAge)
}

func (s *Server) handlePhotoThumbnailStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handlePhotoThumbnailStatusBatch(w, r)
		return
	}
	size := parsePositiveInt(r.URL.Query().Get("size"), photos.DefaultThumbnailSize)
	photoPath := r.URL.Query().Get("path")
	if s.blockAdminOnlyPhotoMedia(w, r, photoPath) {
		return
	}
	ready, err := s.photos.ThumbnailReadyContext(r.Context(), photoPath, size)
	if err != nil {
		if photoSQLiteBusyError(err) {
			ready = false
		} else {
			s.renderPhotoError(w, r, err)
			return
		}
	}
	if !ready {
		queueCtx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
		queueErr := s.photos.QueueThumbnailContext(queueCtx, photoPath, size, photos.ThumbnailVisiblePriority())
		cancel()
		if queueErr != nil && !photoThumbnailStatusQueueError(queueErr) {
			s.renderPhotoError(w, r, queueErr)
			return
		}
		if queueErr == nil {
			if settings, err := s.photoSettings(r.Context()); err == nil {
				s.startPhotoThumbnailJobForSizes(settings, []int{size})
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	if err := writeJSON(w, http.StatusOK, struct {
		Ready bool `json:"ready"`
	}{Ready: ready}); err != nil {
		s.log.Warn("photo thumbnail status response failed", "path", photoPath, "error", err)
	}
}

type photoThumbnailStatusBatchRequest struct {
	Items []photoThumbnailStatusBatchItem `json:"items"`
}

type photoThumbnailStatusBatchItem struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

type photoThumbnailStatusAPIResponse struct {
	Path  string `json:"path"`
	Size  int    `json:"size"`
	Ready bool   `json:"ready"`
}

func (s *Server) handlePhotoThumbnailStatusBatch(w http.ResponseWriter, r *http.Request) {
	var payload photoThumbnailStatusBatchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil {
		s.renderJSONError(w, http.StatusBadRequest, "ungültige Thumbnail-Anfrage")
		return
	}
	if len(payload.Items) > 200 {
		payload.Items = payload.Items[:200]
	}
	requests := make([]photos.ThumbnailReadyRequest, 0, len(payload.Items))
	seen := map[photos.ThumbnailReadyRequest]struct{}{}
	checkPaths := make([]string, 0, len(payload.Items))
	seenPath := map[string]struct{}{}
	includeAdminOnly := s.requestPhotoAdminOnlyVisible(r)
	for _, item := range payload.Items {
		path := strings.TrimSpace(item.Path)
		if path == "" {
			continue
		}
		clean, err := photos.CleanPath(path)
		if err != nil {
			s.renderJSONError(w, http.StatusBadRequest, "ungültiger Fotopfad")
			return
		}
		if _, ok := seenPath[clean]; !ok {
			seenPath[clean] = struct{}{}
			checkPaths = append(checkPaths, clean)
		}
		request := photos.ThumbnailReadyRequest{Path: clean, Size: photos.NormalizeThumbnailSize(item.Size)}
		if _, ok := seen[request]; ok {
			continue
		}
		seen[request] = struct{}{}
		requests = append(requests, request)
	}
	if err := s.photoAccessPolicy(includeAdminOnly).RequireMediaBatch(checkPaths); s.blockPhotoAccess(w, r, err) {
		return
	}
	ready, err := s.photos.ThumbnailReadyBatchContext(r.Context(), requests, includeAdminOnly)
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return
	}
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	queuedSizes := map[int]struct{}{}
	queueCtx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()
	missing := make([]photos.ThumbnailReadyRequest, 0)
	for _, request := range requests {
		if ready[request] {
			continue
		}
		missing = append(missing, request)
		queuedSizes[request.Size] = struct{}{}
	}
	if len(missing) > 0 {
		queueErr := s.photos.QueueThumbnailsContext(queueCtx, missing, photos.ThumbnailVisiblePriority(), includeAdminOnly)
		if queueErr != nil {
			if !photoThumbnailStatusQueueError(queueErr) {
				s.renderPhotoError(w, r, queueErr)
				return
			}
			queuedSizes = map[int]struct{}{}
		}
	}
	if len(queuedSizes) > 0 {
		if settings, err := s.photoSettings(r.Context()); err == nil {
			sizes := make([]int, 0, len(queuedSizes))
			for size := range queuedSizes {
				sizes = append(sizes, size)
			}
			s.startPhotoThumbnailJobForSizes(settings, sizes)
		}
	}
	items := make([]photoThumbnailStatusAPIResponse, 0, len(requests))
	for _, request := range requests {
		items = append(items, photoThumbnailStatusAPIResponse{
			Path:  request.Path,
			Size:  request.Size,
			Ready: ready[request],
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	if err := writeJSON(w, http.StatusOK, struct {
		Items []photoThumbnailStatusAPIResponse `json:"items"`
	}{Items: items}); err != nil {
		s.log.Warn("photo thumbnail batch status response failed", "error", err)
	}
}

func photoThumbnailStatusQueueError(err error) bool {
	return photoSQLiteBusyError(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
