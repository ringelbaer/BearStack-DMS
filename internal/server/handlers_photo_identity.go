package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func (s *Server) handlePhotoIdentities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	status, err := s.photos.PhotoIdentities(r.Context(), after)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, status)
		return
	}
	s.render(w, r, "photo_identity.html", PageData{Title: "Fotoordner und Aufbewahrung", Active: "settings", SettingsTab: "photos", PhotoIdentities: status, Notice: r.URL.Query().Get("notice")})
}

func (s *Server) handleResolvePhotoIdentity(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	revision, err := faceID(r.PostForm.Get("revision"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	setAuditTarget(r, r.PathValue("id"))
	switch r.PostForm.Get("action") {
	case "relocate":
		err = s.photos.ResolvePhotoRelocation(r.Context(), id, revision, r.PostForm.Get("target"))
	case "purge":
		err = s.photos.ForgetRetainedPhoto(r.Context(), id, revision)
	default:
		err = photos.ErrLabelInvalid
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	s.invalidatePhotoStatisticsCache()
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	redirectWithNotice(w, r, "/settings/photos/identities", "Zuordnung und Aufbewahrung aktualisiert.")
}

func (s *Server) handleFaceSourceReviews(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	items, err := s.photos.PendingFaceReviews(r.Context(), after)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, map[string]any{"faces": items})
		return
	}
	s.render(w, r, "face_source_reviews.html", PageData{Title: "Geänderte Fotos prüfen", Active: "photos", FaceSourceReviews: items, Notice: r.URL.Query().Get("notice")})
}

func (s *Server) handleFaceSourceReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if r.Method == http.MethodGet {
		data, err := s.photos.FaceSourceReview(r.Context(), id)
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		if wantsJSON(r) {
			_ = writeJSON(w, http.StatusOK, data)
			return
		}
		s.render(w, r, "face_source_review.html", PageData{Title: "Gesicht am geänderten Foto prüfen", Active: "photos", FaceSourceReview: data})
		return
	}
	if !s.parseFaceForm(w, r) {
		return
	}
	var box photos.FaceRegion
	for _, field := range []struct {
		name  string
		value *float64
	}{{"x", &box.X}, {"y", &box.Y}, {"width", &box.Width}, {"height", &box.Height}} {
		*field.value, err = strconv.ParseFloat(r.PostForm.Get(field.name), 64)
		if err != nil {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	release, err := s.acquireFaceAnalysis(ctx)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	defer release()
	review, err := s.photos.FaceSourceReview(ctx, id)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if review.Revision != r.PostForm.Get("revision") {
		s.labelError(w, r, photos.ErrLabelConflict)
		return
	}
	var result *facerec.Result
	if client, clientErr := s.faceClient(); clientErr == nil {
		defer client.HTTP.CloseIdleConnections()
		data, err := s.photos.FaceImage(ctx, review.Face.Path)
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		analyzed, err := client.Analyze(ctx, data)
		if err != nil {
			_ = writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Gesichtserkennung fehlgeschlagen. Die bisherigen Daten bleiben erhalten."})
			return
		}
		result = &analyzed
	}
	setAuditTarget(r, strconv.FormatInt(id, 10))
	if err = s.photos.ConfirmFaceSource(ctx, id, r.PostForm.Get("revision"), box, r.PostForm.Get("name"), result); err != nil {
		s.labelError(w, r, err)
		return
	}
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	redirectWithNotice(w, r, "/photos/faces/review", "Gesicht bestätigt. Nur frisch berechnete, geeignete Merkmale werden für die Erkennung verwendet.")
}

func (s *Server) runPhotoIdentityBackfill(ctx context.Context) {
	if s.photos == nil {
		return
	}
	var cursor int64
	for ctx.Err() == nil {
		var err error
		cursor, err = s.photos.BackfillPhotoIdentities(ctx, cursor)
		if err != nil && ctx.Err() == nil && s.log != nil {
			s.log.Warn("photo fingerprint backfill", "error", err)
		}
		delay := time.Second
		if cursor == 0 || err != nil {
			delay = time.Minute
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
