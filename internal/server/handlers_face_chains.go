package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/photos"
)

func (s *Server) handleFaceChains(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	thresholds, err := s.photos.FaceThresholds(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	s.render(w, r, "face_chains.html", PageData{Title: "Gesichtsketten prüfen", Active: "photos", Assets: photoPageAssets(false), FaceChainSimilarity: thresholds.SuggestionSimilarity})
}

func decodeFaceChainRequest(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	var reader io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			return photos.ErrLabelInvalid
		}
		if len(r.PostForm["payload"]) != 1 {
			return photos.ErrLabelInvalid
		}
		reader = strings.NewReader(r.PostForm.Get("payload"))
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return photos.ErrLabelInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return photos.ErrLabelInvalid
	}
	return nil
}

func (s *Server) faceChainError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, photos.ErrFaceChainLarge) {
		_ = writeJSON(w, 422, map[string]string{"error": err.Error(), "code": "chain_too_large"})
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		_ = writeJSON(w, 503, map[string]string{"error": "Der Kettenabgleich dauert zu lange. Bitte eine höhere Mindestähnlichkeit oder weniger Sprünge wählen oder erneut versuchen.", "code": "timeout"})
		return
	}
	s.labelError(w, r, err)
}

func (s *Server) handleFaceChainSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var request photos.FaceChainSearch
	if err := decodeFaceChainRequest(w, r, &request); err != nil {
		s.labelError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := s.photos.NextFaceChain(ctx, request)
	if err != nil {
		s.faceChainError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handleFaceChainFaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var request photos.FaceChainPageRequest
	if err := decodeFaceChainRequest(w, r, &request); err != nil {
		s.labelError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := s.photos.FaceChainFaces(ctx, request)
	if err != nil {
		s.faceChainError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handleFaceChainAssign(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var request photos.FaceChainAssignment
	if err := decodeFaceChainRequest(w, r, &request); err != nil {
		s.labelError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := s.photos.AssignFaceChain(ctx, labelActor(r), request)
	if err != nil {
		s.faceChainError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, struct {
		OK      bool                `json:"ok"`
		Receipt photos.LabelReceipt `json:"receipt"`
	}{true, out})
}
