package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"bearstack/internal/photos"
)

func (s *Server) handlePersonFolder(w http.ResponseWriter, r *http.Request) {
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodPost {
		if !s.parseFaceForm(w, r) {
			return
		}
		revision, e := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if e != nil {
			s.faceError(w, r, e)
			return
		}
		var target int64
		if raw := r.PostForm.Get("target"); raw != "" {
			target, e = strconv.ParseInt(raw, 10, 64)
			if e != nil {
				s.faceError(w, r, e)
				return
			}
		}
		err = s.photos.ApplyPersonFolderAction(r.Context(), id, photos.PersonFolderAction{Directory: r.PostForm.Get("directory"), Action: r.PostForm.Get("action"), Revision: revision, TargetID: target, Name: r.PostForm.Get("name")})
		if err != nil {
			if errors.Is(err, photos.ErrLabelConflict) {
				s.renderError(w, r, http.StatusConflict, err)
			} else {
				s.faceError(w, r, err)
			}
			return
		}
		if wantsJSON(r) {
			_ = writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		destination := "/photos/people/" + strconv.FormatInt(id, 10) + "/folder"
		if _, err := s.photos.PersonFolders(r.Context(), id, 1); errors.Is(err, sql.ErrNoRows) {
			destination = "/photos/people"
		}
		redirectWithNotice(w, r, destination, "Ordneraktion gespeichert.")
		return
	}
	result, err := s.photos.PersonFolders(r.Context(), id, boundedInt(r.URL.Query().Get("page"), 1, 1, 1000000))
	if r.URL.Query().Get("format") == "fragment" {
		if err == nil && result.Page > 1 && len(result.Folders) == 0 && !result.HasNext {
			result, err = s.photos.PersonFolders(r.Context(), id, 1)
		}
		if errors.Is(err, sql.ErrNoRows) {
			result, err = photos.PersonFolderPage{PersonID: id, Page: 1}, nil
		}
		if err != nil {
			s.faceError(w, r, err)
			return
		}
		s.renderPartial(w, r, "person_folder_content", PageData{PersonFolders: result})
		return
	}
	if err != nil {
		s.faceError(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "json" {
		_ = writeJSON(w, http.StatusOK, result)
		return
	}
	s.render(w, r, "person_folder.html", PageData{PeopleSection: "people", Title: "Ordner einer Person", Active: "photos", Assets: PageAssets{Explicit: true}, PersonFolders: result, Notice: r.URL.Query().Get("notice")})
}
