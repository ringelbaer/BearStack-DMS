// Datei verarbeitet Datei-Uploads und stoesst Dokumentimport oder Medienablage an.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"bearstack/internal/document"
	"bearstack/internal/documentimport"
	"bearstack/internal/storage"
	"bearstack/internal/uploadlimit"
)

func (s *Server) handleUploadWeb(w http.ResponseWriter, r *http.Request) {
	outcome := s.processUpload(w, r, document.UploadWayWeb)
	setAuditTarget(r, uploadAuditTarget(outcome))
	if wantsJSONResponse(r) {
		status := http.StatusCreated
		if len(outcome.Uploaded) == 0 && len(outcome.Errors) > 0 {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, outcome)
		return
	}
	notice := uploadNotice(outcome)
	redirectWithNotice(w, r, "/documents", notice)
}

func (s *Server) handleUploadAPI(w http.ResponseWriter, r *http.Request) {
	outcome := s.processUpload(w, r, document.UploadWayAPI)
	setAuditTarget(r, uploadAuditTarget(outcome))
	status := http.StatusCreated
	if len(outcome.Uploaded) == 0 && len(outcome.Errors) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, outcome)
}

type uploadOutcome struct {
	Uploaded   []uploadItem      `json:"uploaded"`
	Duplicates []duplicateItem   `json:"duplicates"`
	Errors     []uploadErrorItem `json:"errors"`
}

type uploadItem struct {
	ID                     int64  `json:"id"`
	Filename               string `json:"filename"`
	Title                  string `json:"title"`
	UploadWay              string `json:"upload_way"`
	ContentTextSource      string `json:"content_text_source"`
	ContentTextSourceLabel string `json:"content_text_source_label"`
	SizeBytes              int64  `json:"size_bytes"`
	DocumentURL            string `json:"document_url"`
	DownloadURL            string `json:"download_url"`
	PreviewURL             string `json:"preview_url"`
	DocumentDate           string `json:"document_date,omitempty"`
}

type duplicateItem struct {
	Filename         string `json:"filename"`
	ExistingID       int64  `json:"existing_id"`
	ExistingFilename string `json:"existing_filename"`
	DocumentURL      string `json:"document_url"`
}

type uploadErrorItem struct {
	Filename string `json:"filename"`
	Error    string `json:"error"`
}

func uploadAuditTarget(outcome uploadOutcome) string {
	if len(outcome.Uploaded) == 0 {
		return "0 Dokumente"
	}
	const maxNamedDocuments = 3
	parts := make([]string, 0, min(len(outcome.Uploaded), maxNamedDocuments)+1)
	for i, item := range outcome.Uploaded {
		if i >= maxNamedDocuments {
			parts = append(parts, fmt.Sprintf("+%d weitere", len(outcome.Uploaded)-maxNamedDocuments))
			break
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.Filename
		}
		parts = append(parts, namedAuditTarget("Dokument", item.ID, title))
	}
	return strings.Join(parts, ", ")
}

func (s *Server) processUpload(w http.ResponseWriter, r *http.Request, uploadWay string) uploadOutcome {
	outcome := uploadOutcome{
		Uploaded:   []uploadItem{},
		Duplicates: []duplicateItem{},
		Errors:     []uploadErrorItem{},
	}

	r.Body = http.MaxBytesReader(w, r.Body, uploadlimit.EnvelopeLimit(s.cfg.MaxUploadBytes))
	reader, err := r.MultipartReader()
	if err != nil {
		outcome.Errors = append(outcome.Errors, uploadErrorItem{Error: "ungültiger Multipart-Upload"})
		return outcome
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.log.Warn("multipart read error", "error", err)
			outcome.Errors = append(outcome.Errors, uploadErrorItem{Error: "Fehler beim Lesen der Datei"})
			break
		}
		if part.FileName() == "" {
			drainPart(part)
			_ = part.Close()
			continue
		}
		s.processUploadPart(r.Context(), part, uploadWay, &outcome)
		_ = part.Close()
	}

	return outcome
}

func (s *Server) processUploadPart(ctx context.Context, part *multipart.Part, uploadWay string, outcome *uploadOutcome) {
	filename := filepath.Base(part.FileName())
	candidate, err := s.store.Receive(part, s.cfg.MaxUploadBytes)
	if err != nil {
		outcome.Errors = append(outcome.Errors, uploadErrorItem{Filename: filename, Error: friendlyUploadError(err)})
		return
	}

	result := s.documentImporter().ImportCandidate(ctx, candidate, uploadWay)
	if result.Created != nil {
		outcome.Uploaded = append(outcome.Uploaded, uploadItemFromDocument(result.Created.Document))
	}
	if result.Duplicate != nil {
		outcome.Duplicates = append(outcome.Duplicates, duplicateItemFromImport(*result.Duplicate))
	}
	if result.Error != nil {
		if s.log != nil {
			s.log.Warn("document import failed", "filename", filename, "error", result.Error)
		}
		outcome.Errors = append(outcome.Errors, uploadErrorItem{Filename: filename, Error: friendlyImportError(result.Error)})
	}
}

func uploadItemFromDocument(doc document.Document) uploadItem {
	item := uploadItem{
		ID:                     doc.ID,
		Filename:               doc.OriginalName,
		Title:                  doc.Title,
		UploadWay:              document.NormalizeUploadWay(doc.UploadWay),
		ContentTextSource:      doc.ContentTextSource,
		ContentTextSourceLabel: doc.ContentTextSourceLabel(),
		SizeBytes:              doc.SizeBytes,
		DocumentURL:            fmt.Sprintf("/documents/%d", doc.ID),
		DownloadURL:            fmt.Sprintf("/documents/%d/download", doc.ID),
		PreviewURL:             fmt.Sprintf("/documents/%d/preview", doc.ID),
	}
	if doc.DocumentDate != nil {
		item.DocumentDate = doc.DocumentDate.Format("2006-01-02")
	}
	return item
}

func duplicateItemFromImport(duplicate documentimport.Duplicate) duplicateItem {
	return duplicateItem{
		Filename:         duplicate.Filename,
		ExistingID:       duplicate.Existing.ID,
		ExistingFilename: duplicate.Existing.OriginalName,
		DocumentURL:      fmt.Sprintf("/documents/%d", duplicate.Existing.ID),
	}
}

func friendlyUploadError(err error) string {
	if errors.Is(err, storage.ErrFileTooLarge) {
		return "Datei überschreitet die konfigurierte Maximalgröße"
	}
	if errors.Is(err, storage.ErrInvalidFilename) {
		return "Dateiname ist ungültig"
	}
	if errors.Is(err, storage.ErrUnsupportedFileType) {
		return "Dateityp wird nicht unterstützt"
	}
	if isIncompleteRequestBodyError(err) {
		return "Upload unvollständig übertragen"
	}
	return "Datei konnte nicht verarbeitet werden"
}

func isIncompleteRequestBodyError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "content-length") && strings.Contains(text, "only wrote")
}

func friendlyImportError(err error) string {
	if err == nil {
		return ""
	}
	return "Dokument konnte nicht importiert werden"
}

func uploadNotice(outcome uploadOutcome) string {
	parts := make([]string, 0, 3)
	if len(outcome.Uploaded) > 0 {
		parts = append(parts, fmt.Sprintf("%d Dokument(e) hochgeladen", len(outcome.Uploaded)))
	}
	if len(outcome.Duplicates) > 0 {
		parts = append(parts, fmt.Sprintf("%d Duplikat(e) übersprungen", len(outcome.Duplicates)))
	}
	if len(outcome.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d Fehler", len(outcome.Errors)))
	}
	if len(parts) == 0 {
		return "Keine Dateien hochgeladen."
	}
	return strings.Join(parts, ", ") + "."
}

func drainPart(part *multipart.Part) {
	_, _ = io.Copy(io.Discard, part)
}

func wantsJSONResponse(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		r.Header.Get("X-Requested-With") == "XMLHttpRequest"
}
