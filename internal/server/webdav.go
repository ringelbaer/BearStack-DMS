// Datei stellt WebDAV-Zugriff auf gespeicherte Dokumentdateien bereit.
package server

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/storage"
)

const (
	webDAVWellKnownPrefix = "/.well-known/webdav"
	webDAVAllowMethods    = "OPTIONS, PROPFIND, GET, HEAD, PUT"
)

func (s *Server) handleWebDAV(w http.ResponseWriter, r *http.Request) {
	recorder := newWebDAVStatusRecorder(w)
	w = recorder
	w.Header().Set("DAV", "1")
	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("Allow", webDAVAllowMethods)
		w.Header().Set("MS-Author-Via", "DAV")
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		s.handleWebDAVPropfind(w, r)
	case http.MethodGet, http.MethodHead:
		s.handleWebDAVRead(w, r)
	case http.MethodPut:
		s.handleWebDAVPut(w, r)
	case http.MethodDelete, "MKCOL", http.MethodPost, "PROPPATCH", "LOCK", "UNLOCK", http.MethodPatch:
		http.Error(w, "BearStack WebDAV is read-only", http.StatusForbidden)
	case "MOVE", "COPY":
		http.Error(w, "BearStack WebDAV is read-only", http.StatusForbidden)
	default:
		w.Header().Set("Allow", webDAVAllowMethods)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
	s.logWebDAVRequest(r, recorder.Status())
}

func (s *Server) handleWebDAVWellKnown(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	target := s.webDAVPath() + "/"
	if strings.HasPrefix(path, webDAVWellKnownPrefix+"/") {
		target += strings.TrimPrefix(path, webDAVWellKnownPrefix+"/")
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func isWebDAVWellKnownPath(path string) bool {
	return path == webDAVWellKnownPrefix || strings.HasPrefix(path, webDAVWellKnownPrefix+"/")
}

func (s *Server) handleWebDAVPropfind(w http.ResponseWriter, r *http.Request) {
	depth := strings.TrimSpace(r.Header.Get("Depth"))
	if depth == "" {
		depth = "1"
	}
	if depth != "0" && depth != "1" {
		http.Error(w, "Depth must be 0 or 1", http.StatusForbidden)
		return
	}

	segments, err := s.webDAVPathSegments(r)
	if err != nil {
		http.Error(w, "bad WebDAV path", http.StatusBadRequest)
		return
	}
	resolver := s.webDAVResolver()
	resource, err := resolver.Resolve(r.Context(), segments)
	if err != nil {
		s.writeWebDAVResolveError(w, r, err)
		return
	}

	resources := []webDAVResource{resource}
	if depth == "1" && resource.IsDir {
		children, err := resolver.Children(r.Context(), resource)
		if err != nil {
			s.writeWebDAVResolveError(w, r, err)
			return
		}
		resources = append(resources, children...)
	}

	if err := writeWebDAVMultiStatus(w, s.webDAVPath(), resources); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
}

func (s *Server) handleWebDAVRead(w http.ResponseWriter, r *http.Request) {
	segments, err := s.webDAVPathSegments(r)
	if err != nil {
		http.Error(w, "bad WebDAV path", http.StatusBadRequest)
		return
	}
	resource, err := s.webDAVResolver().Resolve(r.Context(), segments)
	if err != nil {
		s.writeWebDAVResolveError(w, r, err)
		return
	}
	if resource.IsDir {
		http.Error(w, "cannot read a WebDAV collection", http.StatusMethodNotAllowed)
		return
	}

	path, err := s.store.Resolve(resource.Document.StoredPath)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}

	contentType := webDAVDocumentContentType(resource.Document)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("ETag", webDAVETag(resource.Document))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, resource.Name, webDAVModifiedTime(resource.Document, info.ModTime()), file)
}

func (s *Server) handleWebDAVPut(w http.ResponseWriter, r *http.Request) {
	segments, err := s.webDAVPathSegments(r)
	if err != nil {
		http.Error(w, "bad WebDAV path", http.StatusBadRequest)
		return
	}
	if len(segments) == 0 || strings.HasSuffix(r.URL.EscapedPath(), "/") {
		http.Error(w, "PUT target must be a file path", http.StatusBadRequest)
		return
	}
	if isSearchFavoritesFolderName(segments[0]) {
		http.Error(w, "search favorites are read-only", http.StatusForbidden)
		return
	}

	resolver := s.webDAVResolver()
	if _, err := resolver.Resolve(r.Context(), segments); err == nil {
		s.logWebDAVPutConflict(r, "target_exists")
		http.Error(w, "resource already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.writeWebDAVResolveError(w, r, err)
		return
	}

	parentSegments := segments[:len(segments)-1]
	parent, err := resolver.Resolve(r.Context(), parentSegments)
	if err != nil {
		s.writeWebDAVResolveError(w, r, err)
		return
	}
	if !parent.IsDir {
		s.logWebDAVPutConflict(r, "parent_not_collection")
		http.Error(w, "parent is not a collection", http.StatusConflict)
		return
	}
	if len(parent.Segments) > 0 && isSearchFavoritesFolderName(parent.Segments[0]) {
		http.Error(w, "search favorites are read-only", http.StatusForbidden)
		return
	}

	tags := parent.Selection.Tags()
	unknownTags, err := s.unknownDocumentTagsForRequest(r, tags)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if len(unknownTags) > 0 {
		s.renderDocumentTagCreationForbidden(w, r, unknownTags)
		return
	}

	filename := filepath.Base(segments[len(segments)-1])
	candidate, err := s.store.ReceiveReader(filename, r.Body, s.cfg.MaxUploadBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrFileTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		s.logWebDAVPutRejected(r, status, err, "filename", filename)
		http.Error(w, friendlyUploadError(err), status)
		return
	}

	result := s.documentImporter().ImportCandidateWithTags(r.Context(), candidate, document.UploadWayWebDAV, tags)
	if result.Created != nil {
		setAuditTarget(r, documentAuditTargetFor(result.Created.Document))
		w.Header().Set("Location", webDAVHref(s.webDAVPath(), webDAVResource{
			Name:     segments[len(segments)-1],
			Segments: append([]string(nil), segments...),
			Document: result.Created.Document,
		}))
		w.WriteHeader(http.StatusCreated)
		return
	}
	if result.Duplicate != nil {
		s.logWebDAVPutConflict(r, "content_duplicate", "duplicate_document_id", result.Duplicate.Existing.ID)
		http.Error(w, "resource already exists", http.StatusConflict)
		return
	}
	if result.Error != nil {
		if s.log != nil {
			s.log.Warn("webdav import failed", "filename", filename, "error", result.Error)
		}
		s.renderError(w, r, http.StatusInternalServerError, errors.New(friendlyImportError(result.Error)))
		return
	}
	s.renderError(w, r, http.StatusInternalServerError, errors.New("Dokument konnte nicht importiert werden"))
}

func (s *Server) writeWebDAVResolveError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	s.renderHTTPError(w, r, err)
}

type webDAVStatusRecorder struct {
	http.ResponseWriter
	status int
}

func newWebDAVStatusRecorder(w http.ResponseWriter) *webDAVStatusRecorder {
	return &webDAVStatusRecorder{ResponseWriter: w}
}

func (r *webDAVStatusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *webDAVStatusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func (r *webDAVStatusRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (s *Server) logWebDAVRequest(r *http.Request, status int) {
	if s.log == nil {
		return
	}
	ua := strings.TrimSpace(r.UserAgent())
	traceAll := webDAVTraceEnabled()
	traceDolphinErrors := webDAVLooksLikeDolphin(ua) && status >= http.StatusBadRequest
	if !traceAll && !traceDolphinErrors {
		return
	}
	logArgs := []any{
		"method", r.Method,
		"status", status,
		"path", r.URL.Path,
		"escaped_path", r.URL.EscapedPath(),
		"raw_path", r.URL.RawPath,
		"destination", r.Header.Get("Destination"),
		"overwrite", r.Header.Get("Overwrite"),
		"depth", r.Header.Get("Depth"),
		"content_length", r.ContentLength,
		"transfer_encoding", strings.Join(r.TransferEncoding, ","),
		"user_agent", ua,
	}
	if traceDolphinErrors && !traceAll {
		s.log.Warn("webdav dolphin request failed", logArgs...)
		return
	}
	s.log.Info("webdav request trace", logArgs...)
}

func (s *Server) logWebDAVPutConflict(r *http.Request, reason string, extra ...any) {
	if s.log == nil {
		return
	}
	ua := strings.TrimSpace(r.UserAgent())
	if !webDAVTraceEnabled() && !webDAVLooksLikeDolphin(ua) {
		return
	}
	logArgs := []any{
		"reason", reason,
		"path", r.URL.Path,
		"escaped_path", r.URL.EscapedPath(),
		"raw_path", r.URL.RawPath,
		"destination", r.Header.Get("Destination"),
		"overwrite", r.Header.Get("Overwrite"),
		"user_agent", ua,
	}
	logArgs = append(logArgs, extra...)
	s.log.Warn("webdav put conflict", logArgs...)
}

func (s *Server) logWebDAVPutRejected(r *http.Request, status int, err error, extra ...any) {
	if s.log == nil || err == nil {
		return
	}
	ua := strings.TrimSpace(r.UserAgent())
	if !webDAVTraceEnabled() && !webDAVLooksLikeDolphin(ua) {
		return
	}
	reason := "upload_rejected"
	switch {
	case errors.Is(err, storage.ErrUnsupportedFileType):
		reason = "unsupported_file_type"
	case errors.Is(err, storage.ErrInvalidFilename):
		reason = "invalid_filename"
	case errors.Is(err, storage.ErrFileTooLarge):
		reason = "file_too_large"
	case isIncompleteRequestBodyError(err):
		reason = "incomplete_request_body"
	}
	logArgs := []any{
		"reason", reason,
		"status", status,
		"error", err.Error(),
		"path", r.URL.Path,
		"escaped_path", r.URL.EscapedPath(),
		"raw_path", r.URL.RawPath,
		"user_agent", ua,
	}
	logArgs = append(logArgs, extra...)
	s.log.Warn("webdav put rejected", logArgs...)
}

func webDAVLooksLikeDolphin(userAgent string) bool {
	userAgent = strings.ToLower(strings.TrimSpace(userAgent))
	return strings.Contains(userAgent, "dolphin") || strings.Contains(userAgent, "kio")
}

func webDAVTraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BEARSTACK_WEBDAV_TRACE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Server) webDAVPath() string {
	if s == nil {
		return config.DefaultWebDAVPath
	}
	path, err := config.NormalizeWebDAVPath(s.cfg.WebDAV.Path)
	if err != nil {
		return config.DefaultWebDAVPath
	}
	return path
}

func (s *Server) webDAVPathSegments(r *http.Request) ([]string, error) {
	return webDAVPathSegments(r, s.webDAVPath())
}

func webDAVPathSegments(r *http.Request, webDAVPrefix string) ([]string, error) {
	escaped := r.URL.EscapedPath()
	if escaped == webDAVPrefix || escaped == webDAVPrefix+"/" {
		return nil, nil
	}
	prefix := webDAVPrefix + "/"
	if !strings.HasPrefix(escaped, prefix) {
		return nil, errors.New("missing webdav prefix")
	}
	rest := strings.Trim(strings.TrimPrefix(escaped, prefix), "/")
	if rest == "" {
		return nil, nil
	}
	parts := strings.Split(rest, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		segment, err := url.PathUnescape(part)
		if err != nil {
			return nil, err
		}
		segment = strings.Join(strings.Fields(strings.TrimSpace(segment)), " ")
		if segment == "" {
			return nil, errors.New("empty path segment")
		}
		segments = append(segments, segment)
	}
	return segments, nil
}
