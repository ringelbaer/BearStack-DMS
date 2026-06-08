// Datei stellt Handler fuer OCR-Start, OCR-Status und OCR-Ergebnisse bereit.
package server

import (
	"errors"
	"net/http"
	"time"

	"bearstack/internal/document"
)

func (s *Server) handleOCR(w http.ResponseWriter, r *http.Request) {
	doc, err := s.fileDocumentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if doc.IsDeleted() {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("OCR ist für gelöschte Dokumente nicht verfügbar"), documentURL(doc.ID, formReturnURL(r), ""))
		return
	}
	lang, label, err := tesseractLanguage(r.PathValue("lang"))
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, documentURL(doc.ID, formReturnURL(r), ""))
		return
	}
	if _, err := s.prepareOCRDocument(doc); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, documentAuditTargetFor(doc))

	job, created, err := s.repo.EnqueueOCRJob(r.Context(), doc.ID, lang, label)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if !created {
		redirect(w, r, documentURL(doc.ID, formReturnURL(r), "OCR läuft bereits. Der Status wird auf dieser Seite angezeigt."))
		return
	}
	s.enqueueOCRJob(job.ID)

	redirect(w, r, documentURL(doc.ID, formReturnURL(r), "OCR "+label+" wurde eingereiht. Der Status wird auf dieser Seite angezeigt."))
}

func (s *Server) handleOCRStatus(w http.ResponseWriter, r *http.Request) {
	doc, err := s.fileDocumentFromRequest(r)
	if err != nil {
		s.renderJSONError(w, http.StatusNotFound, "Dokument nicht gefunden")
		return
	}
	job, ok, err := s.repo.LatestOCRJobForDocument(r.Context(), doc.ID)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		if err := writeJSON(w, http.StatusOK, struct {
			Job any `json:"job"`
		}{Job: nil}); err != nil {
			s.log.Warn("ocr status response failed", "document_id", doc.ID, "error", err)
		}
		return
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Job ocrJobResponse `json:"job"`
	}{Job: ocrJobResponseFrom(job)}); err != nil {
		s.log.Warn("ocr status response failed", "document_id", doc.ID, "job_id", job.ID, "error", err)
	}
}

type ocrJobResponse struct {
	ID              int64  `json:"id"`
	DocumentID      int64  `json:"document_id"`
	Language        string `json:"language"`
	LanguageLabel   string `json:"language_label"`
	Status          string `json:"status"`
	StatusText      string `json:"status_text"`
	Active          bool   `json:"active"`
	Terminal        bool   `json:"terminal"`
	CurrentPage     int    `json:"current_page"`
	TotalPages      int    `json:"total_pages"`
	ProgressPercent int    `json:"progress_percent"`
	TextLength      int    `json:"text_length"`
	Message         string `json:"message,omitempty"`
	Error           string `json:"error,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	StartedAt       string `json:"started_at,omitempty"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

func ocrJobResponseFrom(job document.OCRJob) ocrJobResponse {
	response := ocrJobResponse{
		ID:              job.ID,
		DocumentID:      job.DocumentID,
		Language:        job.Language,
		LanguageLabel:   job.LanguageLabel,
		Status:          job.Status,
		StatusText:      job.StatusText(),
		Active:          job.Active(),
		Terminal:        job.Terminal(),
		CurrentPage:     job.CurrentPage,
		TotalPages:      job.TotalPages,
		ProgressPercent: job.ProgressPercent(),
		TextLength:      job.TextLength,
		Message:         job.Message,
		Error:           job.Error,
		CreatedAt:       job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       job.UpdatedAt.Format(time.RFC3339),
	}
	if job.StartedAt != nil {
		response.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.FinishedAt != nil {
		response.FinishedAt = job.FinishedAt.Format(time.RFC3339)
	}
	return response
}
