// Datei verarbeitet Formularaktionen fuer Dokumentmetadaten, Felder, Tags und Workflows.
package server

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	"bearstack/internal/document"
)

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderMetadataError(w, r, http.StatusBadRequest, err)
		return
	}
	tags := normalizeTagValues(r.Form["tags"], r.FormValue("new_tags"))
	unknownTags, err := s.unknownDocumentTagsForRequest(r, tags)
	if err != nil {
		s.renderMetadataError(w, r, http.StatusInternalServerError, err)
		return
	}
	if len(unknownTags) > 0 {
		s.renderDocumentTagCreationForbidden(w, r, unknownTags)
		return
	}
	documentDate, err := parseOptionalDate(r.FormValue("document_date"))
	if err != nil {
		s.renderMetadataError(w, r, http.StatusBadRequest, errors.New("ungültiges Dokumentdatum"))
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderMetadataError(w, r, http.StatusInternalServerError, err)
		return
	}
	customValues := customValuesFromForm(r, fields)
	err = s.repo.UpdateMetadata(r.Context(), id, r.FormValue("title"), r.FormValue("description"), documentDate, tags, customValues)
	if err != nil {
		s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
		return
	}
	s.invalidateDocumentCountCache()
	if wantsJSON(r) {
		updated, err := s.repo.GetDocument(r.Context(), id)
		if err != nil {
			s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
			return
		}
		setAuditTarget(r, documentAuditTargetFor(updated))
		s.renderMetadataJSON(w, updated)
		return
	}
	if updated, err := s.repo.GetDocument(r.Context(), id); err == nil {
		setAuditTarget(r, documentAuditTargetFor(updated))
	}
	redirect(w, r, documentURL(id, r.FormValue("return"), "Metadaten gespeichert."))
}

func (s *Server) handleDocumentDate(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderMetadataError(w, r, http.StatusBadRequest, err)
		return
	}
	documentDate, err := parseOptionalDate(r.FormValue("document_date"))
	if err != nil {
		s.renderMetadataError(w, r, http.StatusBadRequest, errors.New("ungültiges Dateidatum"))
		return
	}
	if err := s.repo.UpdateDocumentDate(r.Context(), id, documentDate); err != nil {
		s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
		return
	}
	s.invalidateDocumentCountCache()
	updated, err := s.repo.GetDocument(r.Context(), id)
	if err != nil {
		s.renderMetadataError(w, r, httpStatusForDocumentError(err), err)
		return
	}
	setAuditTarget(r, documentAuditTargetFor(updated))
	s.renderDocumentDateJSON(w, updated)
}

func (s *Server) renderMetadataError(w http.ResponseWriter, r *http.Request, status int, err error) {
	if wantsJSON(r) {
		s.renderJSONError(w, status, err.Error())
		return
	}
	if status == http.StatusInternalServerError {
		s.renderHTTPError(w, r, err)
		return
	}
	returnURL := ""
	if id, idErr := idFromRequest(r); idErr == nil {
		returnURL = documentURL(id, formReturnURL(r), "")
	}
	s.renderErrorWithReturn(w, r, status, err, returnURL)
}

func (s *Server) renderMetadataJSON(w http.ResponseWriter, doc document.Document) {
	tags := doc.Tags
	if tags == nil {
		tags = []string{}
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Title             string   `json:"title"`
		DocumentDate      string   `json:"document_date"`
		DocumentDateInput string   `json:"document_date_input"`
		Tags              []string `json:"tags"`
		Notice            string   `json:"notice"`
	}{
		Title:             doc.Title,
		DocumentDate:      formatDate(doc.DocumentDate),
		DocumentDateInput: formatDateInput(doc.DocumentDate),
		Tags:              tags,
		Notice:            "Metadaten gespeichert.",
	}); err != nil {
		s.log.Warn("metadata response failed", "document_id", doc.ID, "error", err)
	}
}

func (s *Server) renderDocumentDateJSON(w http.ResponseWriter, doc document.Document) {
	if err := writeJSON(w, http.StatusOK, struct {
		DocumentDate      string `json:"document_date"`
		DocumentDateInput string `json:"document_date_input"`
		Notice            string `json:"notice"`
	}{
		DocumentDate:      formatDate(doc.DocumentDate),
		DocumentDateInput: formatDateInput(doc.DocumentDate),
		Notice:            "Dateidatum gespeichert.",
	}); err != nil {
		s.log.Warn("document date response failed", "document_id", doc.ID, "error", err)
	}
}

func httpStatusForDocumentError(err error) int {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func (s *Server) handleDocumentTags(w http.ResponseWriter, r *http.Request) {
	doc, err := s.documentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if doc.IsDeleted() {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Tags können für gelöschte Dokumente nicht bearbeitet werden"), documentURL(doc.ID, formReturnURL(r), ""))
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	tags := normalizeTagValues(r.Form["tags"], "")
	unknownTags, err := s.unknownDocumentTagsForRequest(r, tags)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if len(unknownTags) > 0 {
		s.renderDocumentTagCreationForbidden(w, r, unknownTags)
		return
	}
	if err := s.repo.UpdateMetadata(r.Context(), doc.ID, doc.Title, doc.Description, doc.DocumentDate, tags, doc.CustomValues); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	setAuditTarget(r, documentAuditTargetFor(doc))
	if err := writeJSON(w, http.StatusOK, struct {
		Tags []string `json:"tags"`
	}{Tags: tags}); err != nil {
		s.log.Warn("tag update response failed", "document_id", doc.ID, "error", err)
	}
}
