// Datei liefert Statistikseiten und Kennzahlen fuer Dokumente, Fotos und Systemzustand.
package server

import (
	"fmt"
	"net/http"
)

func (s *Server) handleStatistics(w http.ResponseWriter, r *http.Request) {
	stats, err := s.cachedDocumentStatistics(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := PageData{
		Title:      "Statistik",
		Active:     "statistics",
		Assets:     statisticsPageAssets(),
		Statistics: stats,
		Tags:       tags,
		TagStyles:  tagStyleMap(tags),
		Notice:     r.URL.Query().Get("notice"),
		ReturnURL:  currentReturnURL(r),
	}
	if s.photos != nil {
		settings, err := s.photoSettings(r.Context())
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		photoStats, err := s.cachedPhotoStatistics(r.Context())
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		data.PhotoSettings = settings
		data.PhotoIndexTelemetry = s.photos.IndexTelemetry()
		data.PhotoStatistics = photoStats
	}
	s.render(w, r, "statistics.html", data)
}

func (s *Server) handleProblemTextOCR(w http.ResponseWriter, r *http.Request) {
	lang, label, err := tesseractLanguage(r.PathValue("lang"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err)
		return
	}

	docs, err := s.repo.ProblemContentOCRCandidates(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}

	queued := 0
	active := 0
	skipped := 0
	for _, doc := range docs {
		if _, err := s.prepareOCRDocument(doc); err != nil {
			skipped++
			s.log.Warn("problem text ocr skipped unsupported document", "document_id", doc.ID, "error", err)
			continue
		}
		job, created, err := s.repo.EnqueueOCRJob(r.Context(), doc.ID, lang, label)
		if err != nil {
			skipped++
			s.log.Warn("problem text ocr enqueue failed", "document_id", doc.ID, "error", err)
			continue
		}
		if !created {
			active++
			continue
		}
		s.enqueueOCRJob(job.ID)
		queued++
	}

	notice := fmt.Sprintf("OCR %s für %d Datei(en) eingereiht.", label, queued)
	if active > 0 {
		notice += fmt.Sprintf(" %d bereits aktiv.", active)
	}
	if skipped > 0 {
		notice += fmt.Sprintf(" %d übersprungen.", skipped)
	}
	redirectWithNotice(w, r, "/statistics", notice)
}
