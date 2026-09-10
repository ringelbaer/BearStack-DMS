package server

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	if id == 0 && r.URL.Query().Get("format") == "suggestions" {
		w.Header().Set("Cache-Control", "private, no-store")
		result, err := s.photos.SuggestPeople(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		_ = writeJSON(w, http.StatusOK, result)
		return
	}
	page := boundedInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	unknownOnly := id == 0 && r.URL.Query().Get("unknown") == "1"
	knownOnly := r.URL.Query().Get("known") == "1"
	ignoredOnly := r.URL.Query().Get("ignored") == "1"
	if id == 0 && r.URL.Query().Has("filter") {
		switch r.URL.Query().Get("filter") {
		case "all", "known", "unknown", "ignored":
			unknownOnly = r.URL.Query().Get("filter") == "unknown"
			knownOnly = r.URL.Query().Get("filter") == "known"
			ignoredOnly = r.URL.Query().Get("filter") == "ignored"
		default:
			s.faceError(w, r, errors.New("ungültiger Personenfilter"))
			return
		}
	}
	var result photos.PeoplePage
	if id == 0 && ignoredOnly && !unknownOnly {
		result, err = s.photos.IgnoredFaces(r.Context(), page, r.URL.Query().Get("q"), knownOnly)
	} else {
		result, err = s.photos.People(r.Context(), id, page, r.URL.Query().Get("q"), knownOnly, unknownOnly)
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("format") == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		_ = writeJSON(w, http.StatusOK, result)
		return
	}
	data := PageData{Title: "Personen", Active: "photos", Assets: photoPageAssets(false), People: result, Notice: r.URL.Query().Get("notice")}
	if id != 0 {
		data.PhotoSettings, err = s.photoSettings(r.Context())
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	s.render(w, r, "people.html", data)
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
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(b)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "face.jpg", time.Time{}, bytes.NewReader(b))
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
	if wantsJSON(r) {
		w.Header().Set("Cache-Control", "no-store")
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
		var additional []int64
		for _, raw := range r.PostForm["person_id"] {
			id, parseErr := faceID(raw)
			if parseErr != nil {
				err = parseErr
				break
			}
			additional = append(additional, id)
		}
		if err == nil {
			if r.PostForm.Has("new_name") {
				err = s.photos.MergePeopleNamed(r.Context(), source, target, r.PostForm.Get("new_name"), additional...)
			} else {
				err = s.photos.MergePeople(r.Context(), source, target, additional...)
			}
		}
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	if wantsJSON(r) {
		w.Header().Set("Cache-Control", "no-store")
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	if action != "move" && action != "ignore" && action != "restore" {
		err = errors.New("ungültige Aktion")
	}
	if r.FormValue("ignored") == "1" && action == "move" && target == 0 && strings.TrimSpace(r.FormValue("name")) == "" {
		err = errors.New("Bitte einen Namen eingeben")
	}
	if err == nil {
		if action == "restore" {
			err = s.photos.RestoreFaces(r.Context(), ids, target, r.FormValue("name"))
		} else {
			err = s.photos.EditFaces(r.Context(), ids, target, action == "ignore", r.FormValue("name"))
		}
	}
	if err != nil {
		if errors.Is(err, photos.ErrLabelConflict) {
			s.labelError(w, r, err)
			return
		}
		s.faceError(w, r, err)
		return
	}
	if wantsJSON(r) {
		w.Header().Set("Cache-Control", "no-store")
		_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	destination := "/photos/people"
	if r.FormValue("ignored") == "1" {
		query := url.Values{"ignored": {"1"}, "q": {r.FormValue("q")}, "page": {strconv.Itoa(boundedInt(r.FormValue("page"), 1, 1, 1000000))}}
		if r.FormValue("known") == "1" {
			query.Set("known", "1")
		}
		destination += "?" + query.Encode()
	}
	redirectWithNotice(w, r, destination, "Gesichtszuordnungen gespeichert.")
}
func (s *Server) handleFaceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.faceSettings(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	var status photos.FaceStatus
	if r.URL.Query().Get("format") == "json" && r.URL.Query().Get("progress") == "1" {
		status, err = s.photos.FaceProgress(r.Context())
	} else {
		status, err = s.photos.FaceStatus(r.Context())
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	s.faceWorker.mu.Lock()
	view := FaceSettingsView{Settings: settings, Status: status, Running: s.faceWorker.running, Error: s.faceWorker.lastError, Configured: s.cfg.Photos.FaceServiceURL != "" && s.cfg.Photos.FaceServiceToken != ""}
	view.ReconciliationRunning = s.faceWorker.reconcileRunning
	view.ReconciliationError = s.faceWorker.reconcileError
	s.faceWorker.mu.Unlock()
	view.Reconciliation, err = s.photos.FaceReconciliationStatus(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
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
	previous, err := s.faceSettings(r.Context())
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	settings.ReconcileEnabled = previous.ReconcileEnabled
	if r.PostForm.Has("reconcile_enabled") {
		raw := r.PostForm.Get("reconcile_enabled")
		if raw != "0" && raw != "1" {
			s.faceError(w, r, errors.New("ungültige Einstellung für den Gesichtsabgleich"))
			return
		}
		settings.ReconcileEnabled = raw == "1"
	}
	if raw := r.FormValue("reference_limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > photos.MaxFaceReferenceLimit {
			s.faceError(w, r, errors.New("Referenzen pro Person müssen zwischen 1 und 100 liegen"))
			return
		}
		settings.ReferenceLimit = limit
	}
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
	if !settings.ReconcileEnabled {
		s.stopFaceReconciliationRun()
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
	case "reconcile", "reconcile-resume", "reconcile-pause":
		settings.ReconcileEnabled = r.PathValue("action") != "reconcile-pause"
		err = s.saveFaceSettings(r.Context(), settings)
		if err == nil && r.PathValue("action") == "reconcile" {
			err = s.photos.ScheduleFaceReconciliation(r.Context())
		}
		if err == nil && settings.ReconcileEnabled {
			s.startFaceReconciliationRun()
		} else if !settings.ReconcileEnabled {
			s.stopFaceReconciliationRun()
		}
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
		if status, confirmationErr := s.passwordConfirmationFailure(w, r, r.FormValue("password")); confirmationErr != nil {
			s.renderErrorWithReturn(w, r, status, confirmationErr, "/settings/photos/faces")
			return
		}
		settings.Enabled = false
		settings.ReconcileEnabled = false
		err = s.saveFaceSettings(r.Context(), settings)
		if err == nil {
			s.stopFaceRun()
			s.stopFaceReconciliationRun()
			s.faceWorker.run.Lock()
			s.faceWorker.reconcileRun.Lock()
			release, lockErr := s.acquireFaceAnalysis(r.Context())
			if lockErr != nil {
				err = lockErr
			} else {
				err = s.photos.ClearFaces(r.Context())
				release()
			}
			s.faceWorker.reconcileRun.Unlock()
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
