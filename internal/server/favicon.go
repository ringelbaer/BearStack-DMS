// Datei verwaltet optionale, hochgeladene Favicons und liefert sie aus.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"bearstack/internal/uploadlimit"
)

const (
	customFaviconSettingKey = "custom_favicon"
	customFaviconRoute      = "/favicon/custom"
	customFaviconFormField  = "favicon"
	maxCustomFaviconBytes   = 256 << 10
)

var (
	errCustomFaviconMissing     = errors.New("custom favicon missing")
	errCustomFaviconEmpty       = errors.New("custom favicon is empty")
	errCustomFaviconTooLarge    = errors.New("custom favicon is too large")
	errCustomFaviconUnsupported = errors.New("custom favicon type is unsupported")
)

type CustomFavicon struct {
	Filename  string
	MIMEType  string
	Data      []byte
	SHA256    string
	SizeBytes int64
}

type customFaviconPayload struct {
	Filename  string `json:"filename"`
	MIMEType  string `json:"mime_type"`
	Data      string `json:"data"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type customFaviconCacheEntry struct {
	icon   CustomFavicon
	loaded bool
	exists bool
}

func (s *Server) customFavicon(ctx context.Context) (CustomFavicon, bool, error) {
	return s.settingsService().CustomFavicon(ctx)
}

func (s *Server) handleUploadFavicon(w http.ResponseWriter, r *http.Request) {
	icon, err := customFaviconFromRequest(w, r)
	if err != nil {
		redirectWithNotice(w, r, "/settings", friendlyCustomFaviconError(err))
		return
	}
	if err := s.settingsService().SaveCustomFavicon(r.Context(), icon); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, icon.Filename)
	redirectWithNotice(w, r, "/settings", "Favicon gespeichert.")
}

func (s *Server) handleResetFavicon(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	if err := s.settingsService().ClearCustomFavicon(r.Context()); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, "Standard-Favicon")
	redirectWithNotice(w, r, "/settings", "Standard-Favicon aktiviert.")
}

func (s *Server) handleCustomFavicon(w http.ResponseWriter, r *http.Request) {
	icon, ok, err := s.customFavicon(r.Context())
	if err != nil {
		if s.log != nil {
			s.log.Warn("custom favicon setting failed", "error", err)
		}
		s.serveDefaultFavicon(w, r)
		return
	}
	if !ok {
		s.serveDefaultFavicon(w, r)
		return
	}
	setCustomFaviconCacheControl(w, r, icon)
	w.Header().Set("Content-Type", icon.MIMEType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(icon.Data)))
	w.Header().Set("ETag", `"`+icon.SHA256+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}

func (s *Server) serveDefaultFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := webFS.ReadFile("static/bearstack.svg")
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func setCustomFaviconCacheControl(w http.ResponseWriter, r *http.Request, icon CustomFavicon) {
	version := strings.TrimSpace(r.URL.Query().Get("v"))
	if version != "" && version == customFaviconVersion(icon) {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
}

func customFaviconFromRequest(w http.ResponseWriter, r *http.Request) (CustomFavicon, error) {
	r.Body = http.MaxBytesReader(w, r.Body, uploadlimit.EnvelopeLimit(maxCustomFaviconBytes))
	file, header, err := r.FormFile(customFaviconFormField)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return CustomFavicon{}, errCustomFaviconMissing
		}
		return CustomFavicon{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxCustomFaviconBytes+1))
	if err != nil {
		return CustomFavicon{}, err
	}
	if int64(len(data)) > maxCustomFaviconBytes {
		return CustomFavicon{}, errCustomFaviconTooLarge
	}
	return newCustomFavicon(header.Filename, data)
}

func newCustomFavicon(filename string, data []byte) (CustomFavicon, error) {
	filename = cleanCustomFaviconFilename(filename)
	if len(data) == 0 {
		return CustomFavicon{}, errCustomFaviconEmpty
	}
	if int64(len(data)) > maxCustomFaviconBytes {
		return CustomFavicon{}, errCustomFaviconTooLarge
	}
	mimeType, ok := detectCustomFaviconMIME(data)
	if !ok {
		return CustomFavicon{}, errCustomFaviconUnsupported
	}
	sum := sha256.Sum256(data)
	return CustomFavicon{
		Filename:  filename,
		MIMEType:  mimeType,
		Data:      bytes.Clone(data),
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(data)),
	}, nil
}

func cleanCustomFaviconFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	filename = strings.Trim(filename, ". ")
	if filename == "" || filename == "." || filename == ".." {
		return "favicon"
	}
	if len(filename) > 180 {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		if len(ext) > 30 {
			ext = ""
		}
		maxBase := 180 - len(ext)
		if maxBase < 1 {
			maxBase = 1
		}
		if len(base) > maxBase {
			base = base[:maxBase]
		}
		filename = base + ext
	}
	return filename
}

func detectCustomFaviconMIME(data []byte) (string, bool) {
	if isICOMagic(data) {
		return "image/x-icon", true
	}
	switch http.DetectContentType(data) {
	case "image/png":
		return "image/png", true
	case "image/jpeg":
		return "image/jpeg", true
	case "image/gif":
		return "image/gif", true
	case "image/webp":
		return "image/webp", true
	default:
		return "", false
	}
}

func isICOMagic(data []byte) bool {
	return len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0
}

func friendlyCustomFaviconError(err error) string {
	switch {
	case errors.Is(err, errCustomFaviconMissing):
		return "Kein Favicon ausgewählt."
	case errors.Is(err, errCustomFaviconEmpty):
		return "Favicon ist leer."
	case errors.Is(err, errCustomFaviconTooLarge):
		return fmt.Sprintf("Favicon darf höchstens %s groß sein.", formatBytes(maxCustomFaviconBytes))
	case errors.Is(err, errCustomFaviconUnsupported):
		return "Dateityp nicht unterstützt. Erlaubt sind PNG, JPEG, GIF, WebP und ICO."
	default:
		return "Favicon konnte nicht verarbeitet werden."
	}
}

func (svc settingsService) CustomFavicon(ctx context.Context) (CustomFavicon, bool, error) {
	if svc.store == nil {
		return CustomFavicon{}, false, nil
	}
	if svc.app != nil {
		svc.app.mu.RLock()
		entry := svc.app.favicon
		if entry.loaded {
			svc.app.mu.RUnlock()
			return cloneCustomFavicon(entry.icon), entry.exists, nil
		}
		svc.app.mu.RUnlock()
	}
	icon, ok, err := customFavicon(ctx, svc.store)
	if err != nil {
		return CustomFavicon{}, false, err
	}
	svc.cacheCustomFavicon(icon, ok)
	return icon, ok, nil
}

func (svc settingsService) SaveCustomFavicon(ctx context.Context, icon CustomFavicon) error {
	if svc.store == nil {
		return nil
	}
	icon, err := normalizeStoredCustomFavicon(icon)
	if err != nil {
		return err
	}
	value, err := marshalCustomFavicon(icon)
	if err != nil {
		return err
	}
	if err := svc.store.SaveSetting(ctx, customFaviconSettingKey, value); err != nil {
		return err
	}
	svc.cacheCustomFavicon(icon, true)
	return nil
}

func (svc settingsService) ClearCustomFavicon(ctx context.Context) error {
	if svc.store == nil {
		return nil
	}
	if err := svc.store.SaveSetting(ctx, customFaviconSettingKey, ""); err != nil {
		return err
	}
	svc.cacheCustomFavicon(CustomFavicon{}, false)
	return nil
}

func (svc settingsService) cacheCustomFavicon(icon CustomFavicon, exists bool) {
	if svc.app == nil {
		return
	}
	svc.app.mu.Lock()
	svc.app.favicon = customFaviconCacheEntry{
		icon:   cloneCustomFavicon(icon),
		loaded: true,
		exists: exists,
	}
	svc.app.mu.Unlock()
}

func customFavicon(ctx context.Context, settings settingReader) (CustomFavicon, bool, error) {
	value, ok, err := settings.GetSetting(ctx, customFaviconSettingKey)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return CustomFavicon{}, false, err
	}
	icon, err := unmarshalCustomFavicon(value)
	if err != nil {
		return CustomFavicon{}, false, err
	}
	return icon, true, nil
}

func marshalCustomFavicon(icon CustomFavicon) (string, error) {
	icon, err := normalizeStoredCustomFavicon(icon)
	if err != nil {
		return "", err
	}
	payload := customFaviconPayload{
		Filename:  icon.Filename,
		MIMEType:  icon.MIMEType,
		Data:      base64.StdEncoding.EncodeToString(icon.Data),
		SHA256:    icon.SHA256,
		SizeBytes: icon.SizeBytes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalCustomFavicon(value string) (CustomFavicon, error) {
	var payload customFaviconPayload
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return CustomFavicon{}, err
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return CustomFavicon{}, err
	}
	icon := CustomFavicon{
		Filename:  payload.Filename,
		MIMEType:  payload.MIMEType,
		Data:      data,
		SHA256:    payload.SHA256,
		SizeBytes: payload.SizeBytes,
	}
	return normalizeStoredCustomFavicon(icon)
}

func normalizeStoredCustomFavicon(icon CustomFavicon) (CustomFavicon, error) {
	icon.Filename = cleanCustomFaviconFilename(icon.Filename)
	if len(icon.Data) == 0 {
		return CustomFavicon{}, errCustomFaviconEmpty
	}
	if int64(len(icon.Data)) > maxCustomFaviconBytes {
		return CustomFavicon{}, errCustomFaviconTooLarge
	}
	mimeType, ok := detectCustomFaviconMIME(icon.Data)
	if !ok {
		return CustomFavicon{}, errCustomFaviconUnsupported
	}
	icon.MIMEType = mimeType
	sum := sha256.Sum256(icon.Data)
	icon.SHA256 = hex.EncodeToString(sum[:])
	icon.SizeBytes = int64(len(icon.Data))
	icon.Data = bytes.Clone(icon.Data)
	return icon, nil
}

func cloneCustomFavicon(icon CustomFavicon) CustomFavicon {
	icon.Data = bytes.Clone(icon.Data)
	return icon
}

func customFaviconView(icon CustomFavicon) CustomFaviconView {
	if len(icon.Data) == 0 || icon.SHA256 == "" || icon.MIMEType == "" {
		return CustomFaviconView{}
	}
	return CustomFaviconView{
		Uploaded:  true,
		Href:      customFaviconRoute + "?v=" + customFaviconVersion(icon),
		Type:      icon.MIMEType,
		Filename:  icon.Filename,
		SizeBytes: icon.SizeBytes,
	}
}

func customFaviconVersion(icon CustomFavicon) string {
	if len(icon.SHA256) > 12 {
		return icon.SHA256[:12]
	}
	return icon.SHA256
}
