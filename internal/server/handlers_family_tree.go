package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"bearstack/internal/photos"
)

func (s *Server) handleFamilyTreeSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodPost {
		if !s.parseFaceForm(w, r) {
			return
		}
		revision, err := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if err != nil {
			s.faceError(w, r, photos.ErrFamilyTreeSettings)
			return
		}
		ids := []int64{}
		for _, raw := range r.PostForm["person_id"] {
			id, err := faceID(raw)
			if err != nil {
				s.faceError(w, r, photos.ErrFamilyTreeSettings)
				return
			}
			ids = append(ids, id)
		}
		if err := s.photos.SetFamilyTreeRoots(r.Context(), ids, revision); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, photos.ErrFamilyTreeSettings) {
				status = http.StatusBadRequest
			} else if errors.Is(err, photos.ErrFamilyTreeConflict) {
				status = http.StatusConflict
			}
			s.renderError(w, r, status, err)
			return
		}
		redirectWithNotice(w, r, "/settings/photos/family-tree", "Stammbaum-Auswahl gespeichert.")
		return
	}
	settings, err := s.photos.FamilyTreeSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if r.URL.Query().Get("format") == "json" {
		_ = writeJSON(w, http.StatusOK, settings)
		return
	}
	s.render(w, r, "family_tree_settings.html", PageData{Title: "Stammbaum-Einstellungen", Active: "settings", SettingsTab: "family-tree", Assets: PageAssets{Explicit: true}, FamilyTreeSettings: settings, Notice: r.URL.Query().Get("notice")})
}

func (s *Server) handleFamilyTree(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("format") == "json" {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, err := s.photos.FamilyTrees(ctx)
		if err != nil {
			status, message := http.StatusInternalServerError, "Stammbäume konnten nicht geladen werden."
			switch {
			case errors.Is(err, sql.ErrNoRows):
				status, message = http.StatusNotFound, "Es ist kein Stammbaum eingerichtet."
			case errors.Is(err, photos.ErrFamilyTreeLarge):
				status, message = http.StatusUnprocessableEntity, err.Error()
			case errors.Is(err, context.DeadlineExceeded):
				status, message = http.StatusServiceUnavailable, "Das Laden dauert zu lange. Bitte erneut versuchen."
			}
			_ = writeJSON(w, status, map[string]string{"error": message})
			return
		}
		_ = writeJSON(w, http.StatusOK, result)
		return
	}
	enabled, err := s.photos.FamilyTreeEnabled(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !enabled {
		s.renderError(w, r, http.StatusNotFound, errors.New("Es ist kein Stammbaum eingerichtet."))
		return
	}
	s.render(w, r, "family_tree.html", PageData{Title: "Stammbäume", Active: "family-tree", Assets: PageAssets{Explicit: true}})
}
