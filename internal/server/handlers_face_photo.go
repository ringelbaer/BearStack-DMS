package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func (s *Server) acquireFaceAnalysis(ctx context.Context) (func(), error) {
	s.faceWorker.analysisOnce.Do(func() { s.faceWorker.analysis = make(chan struct{}, 1) })
	select {
	case s.faceWorker.analysis <- struct{}{}:
		return func() { <-s.faceWorker.analysis }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) handleAnalyzePhotoFaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	client, err := s.faceClient()
	if err != nil {
		_ = writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Der Gesichtserkennungsdienst ist nicht konfiguriert."})
		return
	}
	defer client.HTTP.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	release, err := s.acquireFaceAnalysis(ctx)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	defer release()
	job, err := s.photos.PrepareFacePhoto(ctx, r.PostForm.Get("path"), facerec.Model)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if err = s.analyzeFaceJob(ctx, client, job); err != nil {
		if ctx.Err() == nil {
			_ = s.photos.FailFaceJob(ctx, job, "Bild konnte nicht analysiert werden")
		}
		_ = writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Gesichtserkennung fehlgeschlagen. Bitte den Erkennungsdienst prüfen und erneut versuchen."})
		return
	}
	photo, err := s.photos.PhotoFaces(ctx, job.Path)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "photo": photo})
}

func (s *Server) handlePhotoFaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	photo, err := s.photos.PhotoFaces(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"photo": photo})
}

func (s *Server) handleUnignorePhotoFaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !s.parseFaceForm(w, r) {
		return
	}
	path := r.PostForm.Get("path")
	setAuditTarget(r, path)
	count, err := s.photos.UnignorePhotoFaces(r.Context(), path, r.PostForm.Get("revision"))
	if errors.Is(err, photos.ErrGroupPhotoChanged) {
		_ = writeJSON(w, http.StatusConflict, map[string]string{"error": "Das Foto wurde inzwischen geändert. Bitte die Gesichter aktualisieren und erneut prüfen.", "code": "conflict"})
		return
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored": count})
}
