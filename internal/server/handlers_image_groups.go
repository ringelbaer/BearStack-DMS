package server

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"bearstack/internal/photos"
)

type ImageGroupMemberView struct {
	PhotoMediaView
	EntityID    int64
	DisplayPath string
	Missing     bool
	Primary     bool
}
type ImageGroupView struct {
	ID       int64
	Revision int64
	Members  []ImageGroupMemberView
}
type imageGroupMemberResponse struct {
	EntityID    int64  `json:"entity_id"`
	Path        string `json:"path"`
	DisplayPath string `json:"display_path"`
	Primary     bool   `json:"primary"`
	Missing     bool   `json:"missing"`
}
type imageGroupResponse struct {
	ID       int64                      `json:"id"`
	Revision int64                      `json:"revision"`
	Members  []imageGroupMemberResponse `json:"members"`
}

func imageGroupURL(id int64) string { return "/photos/image-groups/" + strconv.FormatInt(id, 10) }
func (s *Server) imageGroupError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "Die Bildgruppe konnte nicht geladen oder gespeichert werden."
	switch {
	case errors.Is(err, photos.ErrImageGroupConflict):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, photos.ErrImageGroupInvalid), errors.Is(err, photos.ErrPathEscapesRoot()):
		status, message = http.StatusBadRequest, photos.ErrImageGroupInvalid.Error()
	case errors.Is(err, photos.ErrAdminOnly()):
		status, message = http.StatusForbidden, "Keine Berechtigung für alle betroffenen Bilder."
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, os.ErrNotExist):
		status, message = http.StatusNotFound, "Bild oder Bildgruppe nicht gefunden."
	}
	if status == http.StatusInternalServerError {
		s.log.Error("image group request failed", "error", err)
	}
	if wantsJSON(r) {
		s.renderJSONError(w, status, message)
	} else {
		s.renderError(w, r, status, errors.New(message))
	}
}
func (s *Server) handleCreateImageGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		s.imageGroupError(w, r, photos.ErrImageGroupInvalid)
		return
	}
	id, err := s.photos.CreateImageGroup(r.Context(), r.PostForm["ids"], r.PostForm.Get("primary"), s.requestIsPhotoAdmin(r))
	if err != nil {
		s.imageGroupError(w, r, err)
		return
	}
	setAuditTarget(r, "image-group:"+strconv.FormatInt(id, 10))
	if wantsJSON(r) {
		_ = writeJSON(w, http.StatusCreated, map[string]any{"id": id, "url": imageGroupURL(id)})
		return
	}
	http.Redirect(w, r, imageGroupURL(id), http.StatusSeeOther)
}
func (s *Server) handleImageGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		s.imageGroupError(w, r, os.ErrNotExist)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err = r.ParseForm(); err != nil {
			s.imageGroupError(w, r, photos.ErrImageGroupInvalid)
			return
		}
		revision, e := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if e != nil {
			s.imageGroupError(w, r, photos.ErrImageGroupConflict)
			return
		}
		entity := int64(0)
		if r.PostForm.Get("action") != "dissolve" {
			entity, e = strconv.ParseInt(r.PostForm.Get("entity_id"), 10, 64)
			if e != nil || entity < 1 {
				s.imageGroupError(w, r, photos.ErrImageGroupInvalid)
				return
			}
		}
		exists, e := s.photos.ApplyImageGroupAction(r.Context(), id, revision, entity, r.PostForm.Get("action"), s.requestIsPhotoAdmin(r))
		if e != nil {
			s.imageGroupError(w, r, e)
			return
		}
		destination := imageGroupURL(id)
		if !exists {
			destination = "/photos"
		}
		setAuditTarget(r, "image-group:"+strconv.FormatInt(id, 10))
		if wantsJSON(r) {
			_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true, "exists": exists, "url": destination})
			return
		}
		redirectWithNotice(w, r, destination, "Bildgruppe aktualisiert.")
		return
	}
	group, err := s.photos.ImageGroup(r.Context(), id, s.requestPhotoAdminOnlyVisible(r))
	if err != nil {
		s.imageGroupError(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "json" || wantsJSON(r) {
		response := imageGroupResponse{ID: group.ID, Revision: group.Revision, Members: []imageGroupMemberResponse{}}
		for _, m := range group.Members {
			response.Members = append(response.Members, imageGroupMemberResponse{EntityID: m.EntityID, Path: m.Media.Path, DisplayPath: m.DisplayPath, Primary: m.EntityID == group.PrimaryID, Missing: m.Missing})
		}
		_ = writeJSON(w, http.StatusOK, response)
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.imageGroupError(w, r, err)
		return
	}
	view := ImageGroupView{ID: group.ID, Revision: group.Revision}
	back := "/photos"
	for _, m := range group.Members {
		view.Members = append(view.Members, ImageGroupMemberView{PhotoMediaView: photoMediaView(m.Media, settings), EntityID: m.EntityID, DisplayPath: m.DisplayPath, Missing: m.Missing, Primary: m.EntityID == group.PrimaryID})
		if m.EntityID == group.PrimaryID {
			back = photoPageURL(url.Values{"path": {m.Media.Directory}})
		}
	}
	s.render(w, r, "image_group.html", PageData{Title: "Bildgruppe", Active: "photos", Assets: photoPageAssets(false), PhotoSettings: settings, ImageGroup: view, ReturnURL: back, Notice: r.URL.Query().Get("notice")})
}
