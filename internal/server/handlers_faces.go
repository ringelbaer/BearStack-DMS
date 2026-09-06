package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bearstack/internal/photos"
)

func faceID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("ungültige ID")
	}
	return id, nil
}
func (s *Server) faceError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	if errors.Is(err, photos.ErrAdminOnly()) {
		status = http.StatusForbidden
	}
	s.renderError(w, r, status, err)
}
func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	var id int64
	var err error
	if raw := r.PathValue("id"); raw != "" {
		id, err = faceID(raw)
		if err != nil {
			s.faceError(w, r, err)
			return
		}
	}
	page := boundedInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	result, err := s.photos.People(r.Context(), id, page, r.URL.Query().Get("q"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("format") == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		_ = writeJSON(w, http.StatusOK, result)
		return
	}
	s.render(w, r, "people.html", PageData{Title: "Personen", Active: "photos", Assets: photoPageAssets(false), People: result, Notice: r.URL.Query().Get("notice")})
}
func (s *Server) handleFaceThumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	b, err := s.photos.FaceThumbnail(r.Context(), id)
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}
func (s *Server) handlePersonRename(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	id, err := faceID(r.PathValue("id"))
	if err == nil {
		err = s.photos.RenamePerson(r.Context(), id, r.FormValue("name"))
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	redirectWithNotice(w, r, "/photos/people/"+strconv.FormatInt(id, 10), "Person gespeichert.")
}
func (s *Server) handlePersonMerge(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	source, err := faceID(r.PathValue("id"))
	var target int64
	if err == nil {
		target, err = faceID(r.FormValue("target"))
	}
	if err == nil {
		err = s.photos.MergePeople(r.Context(), source, target)
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	redirectWithNotice(w, r, "/photos/people/"+strconv.FormatInt(target, 10), "Personengruppen zusammengeführt.")
}
func (s *Server) parseFaceForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	return s.parseFormOrRenderError(w, r)
}
func (s *Server) handleFacesEdit(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, raw := range r.Form["face_id"] {
		id, err := faceID(raw)
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	var target int64
	var err error
	if raw := r.FormValue("target"); raw != "" && raw != "0" {
		target, err = faceID(raw)
	}
	action := r.FormValue("action")
	if action != "move" && action != "ignore" {
		err = errors.New("ungültige Aktion")
	}
	if err == nil {
		err = s.photos.EditFaces(r.Context(), ids, target, action == "ignore", r.FormValue("name"))
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	redirectWithNotice(w, r, "/photos/people", "Gesichtszuordnungen gespeichert.")
}
func (s *Server) handleFaceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.faceSettings(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	status, err := s.photos.FaceStatus(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	s.faceWorker.mu.Lock()
	view := FaceSettingsView{Settings: settings, Status: status, Running: s.faceWorker.running, Error: s.faceWorker.lastError, Configured: s.cfg.Photos.FaceServiceURL != "" && s.cfg.Photos.FaceServiceToken != ""}
	s.faceWorker.mu.Unlock()
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("format") == "json" {
		_ = writeJSON(w, http.StatusOK, view)
		return
	}
	s.render(w, r, "face_settings.html", PageData{Title: "Gesichtserkennung", Active: "settings", SettingsTab: "faces", FaceSettings: view, Notice: r.URL.Query().Get("notice")})
}
func (s *Server) handleSaveFaceSettings(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	settings := FaceSettings{Enabled: r.FormValue("enabled") == "1", BatchSize: boundedInt(r.FormValue("batch_size"), 100, 1, 1000), DelayMillis: boundedInt(r.FormValue("delay_millis"), 1000, 100, 60000), IntervalMinutes: boundedInt(r.FormValue("interval_minutes"), 15, 1, 1440)}
	if settings.Enabled {
		client, err := s.faceClient()
		if err == nil {
			err = client.Health(r.Context())
		}
		if err != nil {
			s.faceError(w, r, err)
			return
		}
	}
	if err := s.saveFaceSettings(r.Context(), settings); err != nil {
		s.faceError(w, r, err)
		return
	}
	if !settings.Enabled {
		s.stopFaceRun()
	} else {
		s.startFaceRun()
	}
	redirectWithNotice(w, r, "/settings/photos/faces", "Einstellungen gespeichert.")
}
func (s *Server) handleFaceControl(w http.ResponseWriter, r *http.Request) {
	if !s.parseFaceForm(w, r) {
		return
	}
	settings, err := s.faceSettings(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	switch r.PathValue("action") {
	case "pause":
		settings.Enabled = false
		err = s.saveFaceSettings(r.Context(), settings)
		s.stopFaceRun()
	case "resume":
		client, e := s.faceClient()
		err = e
		if err == nil {
			err = client.Health(r.Context())
		}
		if err == nil {
			settings.Enabled = true
			err = s.saveFaceSettings(r.Context(), settings)
		}
		if err == nil {
			s.startFaceRun()
		}
	case "retry":
		err = s.photos.RetryFaceJobs(r.Context())
		if err == nil && settings.Enabled {
			s.startFaceRun()
		}
	case "clear":
		if r.FormValue("confirm") != "delete" {
			s.faceError(w, r, errors.New("Löschen der Gesichtsdaten muss bestätigt werden"))
			return
		}
		settings.Enabled = false
		err = s.saveFaceSettings(r.Context(), settings)
		if err == nil {
			s.stopFaceRun()
			s.faceWorker.run.Lock()
			err = s.photos.ClearFaces(r.Context())
			s.faceWorker.run.Unlock()
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	redirectWithNotice(w, r, "/settings/photos/faces", "Aktion ausgeführt.")
}
