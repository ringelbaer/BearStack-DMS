// Datei implementiert Massenaktionen fuer Dokumente wie Aktualisieren, Verschieben und Loeschen.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleLinkDocuments(w http.ResponseWriter, r *http.Request) {
	ids, err := formDocumentIDs(r)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, formReturnURL(r))
		return
	}
	ids = uniqueDocumentIDs(ids)
	if len(ids) < 2 {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("mindestens zwei Dokumente zum Verknüpfen auswählen"), formReturnURL(r))
		return
	}
	docs, err := s.repo.ListByIDs(r.Context(), ids)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if err := s.repo.LinkDocuments(r.Context(), ids); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, documentAuditTargetsFor(docs))
	redirectWithNotice(w, r, formReturnURL(r), fmt.Sprintf("%d Dokumente verknüpft.", len(ids)))
}

func (s *Server) handleAddDocumentTags(w http.ResponseWriter, r *http.Request) {
	s.handleBulkDocumentTags(w, r, s.repo.AddTagsToDocuments, true)
}

func (s *Server) handleRemoveDocumentTags(w http.ResponseWriter, r *http.Request) {
	s.handleBulkDocumentTags(w, r, s.repo.RemoveTagsFromDocuments, false)
}

func (s *Server) handleBulkDocumentFields(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	ids, err := formDocumentIDs(r)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, formReturnURL(r))
		return
	}
	ids = uniqueDocumentIDs(ids)
	if len(ids) == 0 {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("mindestens ein Dokument auswählen"), formReturnURL(r))
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	values := customValuesFromForm(r, fields)
	hasValue := false
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			hasValue = true
			break
		}
	}
	if !hasValue {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("mindestens ein Feld ausfüllen"), formReturnURL(r))
		return
	}

	updated, err := s.repo.SetCustomValuesForDocuments(r.Context(), ids, values)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	docs, err := s.repo.ListByIDs(r.Context(), ids)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if updated == 0 {
		setAuditTarget(r, "0 Dokumente")
	} else {
		setAuditTarget(r, documentAuditTargetsFor(docs))
	}
	redirectWithNotice(w, r, formReturnURL(r), fmt.Sprintf("%d Dokument(e) aktualisiert.", updated))
}

func (s *Server) handleBulkDocumentTags(w http.ResponseWriter, r *http.Request, update func(context.Context, []int64, []string) (int, error), requiresExistingTags bool) {
	ids, err := formDocumentIDs(r)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, formReturnURL(r))
		return
	}
	ids = uniqueDocumentIDs(ids)
	if len(ids) == 0 {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("mindestens ein Dokument auswählen"), formReturnURL(r))
		return
	}
	tags := normalizeTagValues(r.Form["tags"], "")
	if len(tags) == 0 {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("mindestens einen Tag auswählen"), formReturnURL(r))
		return
	}
	if requiresExistingTags {
		unknownTags, err := s.unknownDocumentTagsForRequest(r, tags)
		if err != nil {
			s.renderHTTPError(w, r, err)
			return
		}
		if len(unknownTags) > 0 {
			s.renderDocumentTagCreationForbidden(w, r, unknownTags)
			return
		}
	}
	updated, err := update(r.Context(), ids, tags)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	docs, err := s.repo.ListByIDs(r.Context(), ids)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if updated == 0 {
		setAuditTarget(r, "0 Dokumente")
	} else {
		setAuditTarget(r, documentAuditTargetsFor(docs))
	}
	if wantsJSONResponse(r) {
		if err := writeJSON(w, http.StatusOK, struct {
			Updated int      `json:"updated"`
			Tags    []string `json:"tags"`
		}{Updated: updated, Tags: tags}); err != nil {
			s.log.Warn("bulk tag response failed", "updated", updated, "error", err)
		}
		return
	}
	redirectWithNotice(w, r, formReturnURL(r), fmt.Sprintf("%d Dokument(e) aktualisiert.", updated))
}

func (s *Server) handleUnlinkDocument(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	linkedID, err := linkedIDFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	docs, err := s.repo.ListByIDs(r.Context(), []int64{id, linkedID})
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if err := s.repo.UnlinkDocuments(r.Context(), id, linkedID); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, documentAuditTargetsFor(docs))
	redirect(w, r, documentURL(id, formReturnURL(r), "Verknüpfung aufgehoben."))
}
