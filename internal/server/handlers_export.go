// Datei stellt Export-Handler fuer Dokumentdaten und Dateien bereit.
package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/storage"
)

type exportDocumentFile struct {
	Path string
	Info os.FileInfo
	Name string
}

const (
	maxExportDocuments = 500
	maxExportBytes     = 1 << 30

	exportDownloadCookieName = "bearstack_export_download"
	exportDownloadTokenParam = "download_token"
)

var (
	errExportTooManyDocuments = errors.New("zu viele Dokumente ausgewählt; bitte weniger Dokumente exportieren")
	errExportTooLarge         = errors.New("Export ist zu groß; bitte weniger Dokumente auswählen")
)

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	returnURL := exportReturnURL(r)
	ids, err := exportIDs(r)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, returnURL)
		return
	}
	ids = uniqueDocumentIDs(ids)
	if len(ids) == 0 {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("keine Dokumente ausgewählt"), returnURL)
		return
	}
	if len(ids) > maxExportDocuments {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errExportTooManyDocuments, returnURL)
		return
	}

	docs, err := s.repo.ListByIDs(r.Context(), ids)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if len(docs) == 0 {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("keine exportierbaren Dokumente gefunden"), returnURL)
		return
	}

	includeMetadata := r.URL.Query().Get("metadata") == "1"
	var metadata []byte
	if includeMetadata {
		linkedDocuments, err := s.exportLinkedDocuments(r.Context(), docs)
		if err != nil {
			s.renderHTTPError(w, r, err)
			return
		}
		metadata, err = exportMetadataPayload(docs, linkedDocuments)
		if err != nil {
			s.renderHTTPError(w, r, err)
			return
		}
	}

	files, err := s.prepareExportFiles(docs)
	if err != nil {
		if errors.Is(err, errExportTooLarge) {
			s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, returnURL)
			return
		}
		s.renderHTTPError(w, r, err)
		return
	}

	filename := "bearstack-export-" + time.Now().Format("20060102-150405") + ".zip"
	writeExportDownloadCookie(w, r, r.URL.Query().Get(exportDownloadTokenParam))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition("attachment", filename))

	if err := writeExportZip(w, files, metadata); err != nil {
		s.log.Warn("export stream failed", "error", err)
	}
}

func writeExportDownloadCookie(w http.ResponseWriter, r *http.Request, token string) {
	if !validExportDownloadToken(token) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     exportDownloadCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(time.Minute),
		MaxAge:   60,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestUsesHTTPS(r),
	})
}

func validExportDownloadToken(token string) bool {
	if len(token) < 8 || len(token) > 96 {
		return false
	}
	for _, char := range token {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func exportReturnURL(r *http.Request) string {
	if returnURL := formReturnURL(r); returnURL != "/" {
		return returnURL
	}
	return "/documents"
}

func (s *Server) exportLinkedDocuments(ctx context.Context, docs []document.Document) (map[int64][]document.Document, error) {
	ids := make([]int64, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
	}
	return s.repo.LinkedDocumentsForDocuments(ctx, ids)
}

func (s *Server) prepareExportFiles(docs []document.Document) ([]exportDocumentFile, error) {
	files := make([]exportDocumentFile, 0, len(docs))
	var totalSize int64
	for _, doc := range docs {
		path, err := s.store.Resolve(doc.StoredPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		totalSize += info.Size()
		if totalSize > maxExportBytes {
			return nil, errExportTooLarge
		}
		files = append(files, exportDocumentFile{
			Path: path,
			Info: info,
			Name: fmt.Sprintf("%d-%s", doc.ID, storage.SafeFilename(doc.OriginalName)),
		})
	}
	return files, nil
}

func writeExportZip(w io.Writer, files []exportDocumentFile, metadata []byte) error {
	zw := zip.NewWriter(w)
	for _, file := range files {
		if err := addPreparedDocumentToZip(zw, file); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if metadata != nil {
		if err := addBytesToZip(zw, "metadata.json", metadata); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func addPreparedDocumentToZip(zw *zip.Writer, item exportDocumentFile) error {
	file, err := os.Open(item.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := zip.FileInfoHeader(item.Info)
	if err != nil {
		return err
	}
	header.Name = item.Name
	header.Method = zip.Deflate
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}

func exportMetadataPayload(docs []document.Document, linkedDocuments map[int64][]document.Document) ([]byte, error) {
	type exportLinkedDoc struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		OriginalName string `json:"original_name"`
	}
	type exportDoc struct {
		ID              int64             `json:"id"`
		OriginalName    string            `json:"original_name"`
		Title           string            `json:"title"`
		Description     string            `json:"description"`
		Tags            []string          `json:"tags"`
		MIMEType        string            `json:"mime_type"`
		SizeBytes       int64             `json:"size_bytes"`
		SHA256          string            `json:"sha256"`
		UploadWay       string            `json:"upload_way"`
		DocumentDate    string            `json:"document_date,omitempty"`
		UploadedAt      string            `json:"uploaded_at"`
		LinkedDocuments []exportLinkedDoc `json:"linked_documents"`
	}
	payload := make([]exportDoc, 0, len(docs))
	for _, doc := range docs {
		item := exportDoc{
			ID:              doc.ID,
			OriginalName:    doc.OriginalName,
			Title:           doc.Title,
			Description:     doc.Description,
			Tags:            doc.Tags,
			MIMEType:        doc.MIMEType,
			SizeBytes:       doc.SizeBytes,
			SHA256:          doc.SHA256,
			UploadWay:       doc.UploadWay,
			UploadedAt:      doc.UploadedAt.Format(time.RFC3339),
			LinkedDocuments: []exportLinkedDoc{},
		}
		for _, linked := range linkedDocuments[doc.ID] {
			item.LinkedDocuments = append(item.LinkedDocuments, exportLinkedDoc{
				ID:           linked.ID,
				Title:        linked.Title,
				OriginalName: linked.OriginalName,
			})
		}
		if doc.DocumentDate != nil {
			item.DocumentDate = doc.DocumentDate.Format("2006-01-02")
		}
		payload = append(payload, item)
	}

	return json.MarshalIndent(payload, "", "  ")
}

func addBytesToZip(zw *zip.Writer, name string, payload []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}
