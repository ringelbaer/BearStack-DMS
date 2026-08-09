// Datei behandelt Tag-Verwaltung fuer Dokumente und zugehoerige UI-Aktionen.
package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"bearstack/internal/repository"
)

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	photoTags, err := s.listPhotoTagViews(r)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	tagTab := "documents"
	if s.photos != nil && r.URL.Query().Get("tab") == "photos" {
		tagTab = "photos"
	}
	s.render(w, r, "tags.html", PageData{
		Title:     "Tags",
		Active:    "tags",
		Tags:      tags,
		PhotoTags: photoTags,
		TagTab:    tagTab,
		Notice:    r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleSavePhotoTag(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."), photoTagsReturnURL)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	tag, err := s.photos.SaveTag(r.Context(), r.FormValue("name"), r.Form["color"]...)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, photoTagsReturnURL)
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, "Foto-Tag:"+tag.Name)
	redirectWithNotice(w, r, "/tags?tab=photos", "Foto-Tag gespeichert.")
}

func (s *Server) handleRenamePhotoTag(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."), photoTagsReturnURL)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	tag, err := s.photos.RenameTag(r.Context(), r.FormValue("old_name"), r.FormValue("name"), r.Form["color"]...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Foto-Tag nicht gefunden."), photoTagsReturnURL)
			return
		}
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, photoTagsReturnURL)
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, "Foto-Tag:"+tag.Name)
	redirectWithNotice(w, r, "/tags?tab=photos", "Foto-Tag gespeichert.")
}

func (s *Server) handleDeletePhotoTag(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."), photoTagsReturnURL)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	if status, confirmationErr := s.passwordConfirmationFailure(w, r, r.FormValue("password")); confirmationErr != nil {
		s.renderErrorWithReturn(w, r, status, confirmationErr, photoTagsReturnURL)
		return
	}
	deleted, err := s.photos.DeleteTag(r.Context(), r.FormValue("name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Foto-Tag nicht gefunden."), photoTagsReturnURL)
			return
		}
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, photoTagsReturnURL)
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, "Foto-Tag:"+deleted.Name)
	redirectWithNotice(w, r, "/tags?tab=photos", "Foto-Tag gelöscht.")
}

func (s *Server) handleAPITags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	displayMode := s.requestTagDisplayMode(r)
	writeJSON(w, http.StatusOK, struct {
		Tags []tagAPIResponse `json:"tags"`
	}{Tags: tagAPIResponsesFrom(tags, displayMode)})
}

func (s *Server) handleSaveTag(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	name := firstNormalizedTag(r.FormValue("name"))
	primaryTag, err := s.primaryTagFromRequest(r, false)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	groupMode := r.FormValue("group_mode") == "1"
	listHidden := r.FormValue("list_hidden") == "1"
	deleteProtected := r.FormValue("delete_protected") == "1"
	if name == "" {
		if wantsJSON(r) {
			s.renderJSONError(w, http.StatusBadRequest, "Tagname fehlt")
			return
		}
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Tagname fehlt"), "/tags")
		return
	}
	if wantsJSON(r) {
		tag, err := s.repo.GetTagByName(r.Context(), name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.renderHTTPError(w, r, err)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			id, err := s.repo.SaveTag(r.Context(), name, r.FormValue("description"), r.FormValue("color"), groupMode, listHidden, deleteProtected, primaryTag)
			if err != nil {
				s.renderHTTPError(w, r, err)
				return
			}
			tag, err = s.repo.GetTag(r.Context(), id)
			if err != nil {
				s.renderHTTPError(w, r, err)
				return
			}
		}
		setAuditTarget(r, tagAuditTargetFor(tag))
		s.renderTagJSON(w, tag, s.requestTagDisplayMode(r))
		return
	}
	id, err := s.repo.SaveTag(r.Context(), name, r.FormValue("description"), r.FormValue("color"), groupMode, listHidden, deleteProtected, primaryTag)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, namedAuditTarget("Tag", id, name))
	redirectWithNotice(w, r, fmt.Sprintf("/tags/%d", id), "Tag gespeichert.")
}

func (s *Server) handleTagDetail(w http.ResponseWriter, r *http.Request) {
	tag, err := s.tagFromRequest(r)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	rules, err := s.repo.ListTagRules(r.Context(), tag.ID)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	s.render(w, r, "tag_detail.html", PageData{
		Title:    "Tag " + tag.Name,
		Active:   "tags",
		Tag:      tag,
		TagRules: rules,
		Notice:   r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	tag, err := s.tagFromRequest(r)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	name := firstNormalizedTag(r.FormValue("name"))
	if name == "" {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, repository.ErrTagNameMissing, tagDetailReturnURL(tag.ID))
		return
	}
	setAuditTarget(r, tagAuditTargetFor(tag))
	primaryTag, err := s.primaryTagFromRequest(r, tag.PrimaryTag)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	updated, err := s.repo.RenameTag(r.Context(), tag.ID, name, r.FormValue("description"), r.FormValue("color"), r.FormValue("group_mode") == "1", r.FormValue("list_hidden") == "1", r.FormValue("delete_protected") == "1", primaryTag)
	if err != nil {
		if errors.Is(err, repository.ErrTagNameExists) || errors.Is(err, repository.ErrTagNameMissing) {
			s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, tagDetailReturnURL(tag.ID))
			return
		}
		s.renderTagHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	if updated.Name != tag.Name {
		setAuditTarget(r, namedAuditTarget("Tag", updated.ID, tag.Name+" -> "+updated.Name))
	} else {
		setAuditTarget(r, tagAuditTargetFor(updated))
	}
	redirectWithNotice(w, r, fmt.Sprintf("/tags/%d", updated.ID), "Tag gespeichert.")
}

func (s *Server) primaryTagFromRequest(r *http.Request, current bool) (bool, error) {
	enabled, err := s.documentCloudEnabled(r.Context())
	if err != nil {
		return false, err
	}
	if !enabled {
		return current, nil
	}
	return r.FormValue("primary_tag") == "1", nil
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	tag, err := s.tagFromRequest(r)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, tagAuditTargetFor(tag))
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	if status, confirmationErr := s.passwordConfirmationFailure(w, r, r.FormValue("password")); confirmationErr != nil {
		s.renderErrorWithReturn(w, r, status, confirmationErr, tagDetailReturnURL(tag.ID))
		return
	}
	deleted, err := s.repo.DeleteTag(r.Context(), tag.ID)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	redirectWithNotice(w, r, "/tags", fmt.Sprintf("Tag %s gelöscht.", deleted.Name))
}

func (s *Server) handleSaveTagRules(w http.ResponseWriter, r *http.Request) {
	tag, err := s.tagFromRequest(r)
	if err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, tagAuditTargetFor(tag))
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	rules, deleteIDs, err := tagRulesFromForm(r, tag.ID)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, tagDetailReturnURL(tag.ID))
		return
	}
	if err := s.repo.SaveTagRules(r.Context(), tag.ID, rules, deleteIDs); err != nil {
		s.renderTagHTTPError(w, r, err)
		return
	}
	redirectWithNotice(w, r, fmt.Sprintf("/tags/%d", tag.ID), "Regeln gespeichert. Neue Uploads verwenden die aktualisierten Regeln.")
}

const photoTagsReturnURL = "/tags?tab=photos"

func tagDetailReturnURL(id int64) string {
	return fmt.Sprintf("/tags/%d", id)
}

func (s *Server) requestTagDisplayMode(r *http.Request) string {
	if s == nil || s.repo == nil || r == nil {
		return tagDisplayModeLower
	}
	mode, err := s.tagDisplayMode(r.Context())
	if err != nil {
		return tagDisplayModeLower
	}
	return mode
}
