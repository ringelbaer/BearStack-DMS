// Datei kapselt Template-Rendering, Redirects und wiederkehrende HTML-Antwortmuster.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"bearstack"
	"bearstack/internal/document"
	"bearstack/internal/documentconvert"
	"bearstack/internal/photos"
	"bearstack/internal/tagutil"
)

func parseTemplates() (*template.Template, error) {
	assetVersions, err := staticAssetVersions()
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"formatBytes":            formatBytes,
		"formatDate":             formatDate,
		"formatDateInput":        formatDateInput,
		"formatDay":              formatDay,
		"formatDateTime":         formatDateTime,
		"formatDuration":         formatDuration,
		"formatNumber":           formatNumber,
		"formatCoord":            formatCoord,
		"formatRatingData":       formatRatingData,
		"formatTag":              formatTag,
		"formatFolderTag":        formatFolderTag,
		"formatFolderCrumb":      formatFolderCrumb,
		"folderBreadcrumbLabel":  folderBreadcrumbLabel,
		"joinTags":               func(tags []string) string { return strings.Join(tags, ", ") },
		"joinDisplayTags":        joinDisplayTags,
		"listVisibleTags":        listVisibleTags,
		"listHiddenTags":         listHiddenTags,
		"hasTag":                 hasTag,
		"customValue":            customValue,
		"customFieldFilterValue": customFieldFilterValue,
		"columnVisible":          columnVisible,
		"sortIndicator":          sortIndicator,
		"statBarStyle":           statBarStyle,
		"statShareStyle":         statShareStyle,
		"tagSegmentStyle":        tagSegmentStyle,
		"queryEscape": func(value string) template.URL {
			return template.URL(url.QueryEscape(value))
		},
		"assetURL": func(path string) template.URL {
			return staticAssetURL(path, assetVersions)
		},
		"pageCSSAssets": pageCSSAssets,
		"pageJSAssets":  pageJSAssets,
		"pageTagAssets": pageTagAssets,
		"pageCount":     pageCount,
		"tagStyle":      tagStyle,
		"isPDF":         func(mimeType string) bool { return mimeType == "application/pdf" },
		"isImage":       func(mimeType string) bool { return strings.HasPrefix(mimeType, "image/") },
		"isPreviewableDocument": func(name, mimeType string) bool {
			return mimeType == "application/pdf" || strings.HasPrefix(mimeType, "image/") || documentconvert.IsPreviewDocument(name, mimeType)
		},
		"shortHash": func(s string) string {
			if len(s) > 12 {
				return s[:12]
			}
			return s
		},
	}
	return template.New("web").Funcs(funcs).ParseFS(webFS, "templates/*.html")
}

func pageCSSAssets(data PageData) []string {
	pageAssets := resolvedPageAssets(data)
	assets := []string{
		"/static/app.css",
	}
	if pageAssets.Statistics {
		assets = append(assets, "/static/app-statistics.css")
	}
	if pageAssets.Documents {
		assets = append(assets, "/static/app-documents.css")
	}
	assets = append(assets,
		"/static/app-management.css",
		"/static/app-overlays.css",
		"/static/app-responsive.css",
	)
	return assets
}

func pageJSAssets(data PageData) []string {
	pageAssets := resolvedPageAssets(data)
	assets := []string{
		"/static/app.js",
	}
	if pageAssets.Tags {
		assets = append(assets, "/static/app-tags.js")
	}
	if pageAssets.Documents {
		assets = append(assets,
			"/static/app-upload.js",
			"/static/app-documents.js",
			"/static/app-preview.js",
		)
		if data.CustomPDFPreviewEnabled {
			assets = append(assets, "/static/app-pdf-preview.js")
		}
	}
	if pageAssets.OCR {
		assets = append(assets, "/static/app-ocr.js")
	}
	if pageAssets.Statistics {
		assets = append(assets, "/static/app-charts.js")
	}
	if pageAssets.Photos {
		assets = append(assets,
			"/static/app-photos-map.js",
			"/static/app-photos-thumbnails.js",
			"/static/app-photos.js",
			"/static/app-photos-frame.js",
		)
	}
	return assets
}

func pageTagAssets(data PageData) bool {
	return resolvedPageAssets(data).Tags
}

func resolvedPageAssets(data PageData) PageAssets {
	if data.Assets.Explicit {
		return data.Assets
	}
	return inferredPageAssets(data)
}

func inferredPageAssets(data PageData) PageAssets {
	return PageAssets{
		Explicit:   true,
		Documents:  needsDocumentAssets(data),
		OCR:        data.Document.ID > 0,
		Statistics: data.Active == "statistics",
		Photos:     data.PhotoPage || data.PhotoFrame || data.Active == "photos",
		Tags:       needsTagAssets(data),
	}
}

func documentPageAssets() PageAssets {
	return PageAssets{Explicit: true, Documents: true, Tags: true}
}

func documentDetailAssets() PageAssets {
	return PageAssets{Explicit: true, Documents: true, OCR: true, Tags: true}
}

func statisticsPageAssets() PageAssets {
	return PageAssets{Explicit: true, Statistics: true}
}

func photoPageAssets(includeTags ...bool) PageAssets {
	tags := true
	if len(includeTags) > 0 {
		tags = includeTags[0]
	}
	return PageAssets{Explicit: true, Photos: true, Tags: tags}
}

func photoFrameAssets() PageAssets {
	return PageAssets{Explicit: true, Photos: true}
}

func needsDocumentAssets(data PageData) bool {
	switch data.Active {
	case "documents", "folders", "duplicates", "trash":
		return true
	}
	return data.Document.ID > 0 || len(data.Documents) > 0 || data.Pagination.DocumentList || data.InlineDesktopPreview
}

func needsTagAssets(data PageData) bool {
	switch data.Active {
	case "documents", "folders", "duplicates", "trash", "search-favorites":
		return true
	}
	return data.Document.ID > 0 || data.PhotoPage
}

func formatCoord(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.5f", *v)
}

func formatRatingData(v *float64) string {
	if v == nil {
		return ""
	}
	rating := *v
	if rating < -1 {
		rating = -1
	}
	if rating > 5 {
		rating = 5
	}
	return strconv.FormatFloat(rating, 'f', -1, 64)
}

func staticAssetVersions() (map[string]string, error) {
	versions := make(map[string]string)
	err := fs.WalkDir(webFS, "static", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := webFS.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(contents)
		versions["/"+path] = hex.EncodeToString(sum[:])[:12]
		return nil
	})
	return versions, err
}

func staticAssetURL(path string, versions map[string]string) template.URL {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if version, ok := versions[path]; ok {
		return template.URL(path + "?v=" + version)
	}
	return template.URL(path)
}

func customFieldFilterValue(filters []document.CustomFieldFilter, fieldID int64) string {
	for _, filter := range filters {
		if filter.FieldID == fieldID {
			return filter.Value
		}
	}
	return ""
}

func sortIndicator(link SortLink) string {
	if !link.Active {
		return ""
	}
	if link.Direction == document.ListDirectionAscending {
		return "↑"
	}
	return "↓"
}

func statBarStyle(value, maximum int) template.CSS {
	if value < 0 {
		value = 0
	}
	if maximum < 1 {
		maximum = 1
	}
	percent := (value*100 + maximum/2) / maximum
	if percent > 100 {
		percent = 100
	}
	return template.CSS(fmt.Sprintf("--bar-value: %d%%;", percent))
}

func statShareStyle(value, total int) template.CSS {
	if value < 0 {
		value = 0
	}
	if total < 1 {
		total = 1
	}
	percent := (value*100 + total/2) / total
	if percent < 1 && value > 0 {
		percent = 1
	}
	if percent > 100 {
		percent = 100
	}
	return template.CSS(fmt.Sprintf("--bar-value: %d%%;", percent))
}

func tagSegmentStyle(value, total int, tagStyle template.CSS) template.CSS {
	return template.CSS(string(statShareStyle(value, total)) + " " + string(tagStyle))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	data = s.withRenderSettings(r, data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if trace := photos.ListTraceFromContext(r.Context()); trace != nil {
		var body bytes.Buffer
		finishRender := photos.StartListTraceStep(r.Context(), "photos.render.template", photos.ListTraceString("template", name))
		err := s.templates.ExecuteTemplate(&body, name, data)
		if err != nil {
			finishRender(photos.ListTraceString("error", err.Error()))
			s.log.Error("template render failed", "template", name, "error", err)
		} else {
			finishRender(photos.ListTraceInt("bytes", body.Len()))
		}
		if header := photoListServerTimingHeader(trace.Snapshot()); header != "" {
			w.Header().Set("Server-Timing", header)
		}
		if _, writeErr := w.Write(body.Bytes()); writeErr != nil {
			s.log.Warn("template response write failed", "template", name, "error", writeErr)
		}
		return
	}
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template render failed", "template", name, "error", err)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	data = s.withRenderSettings(r, data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template partial render failed", "template", name, "error", err)
	}
}

func (s *Server) withRenderSettings(r *http.Request, data PageData) PageData {
	ctx := r.Context()
	if !data.Assets.Explicit {
		data.Assets = inferredPageAssets(data)
	}
	if data.AppVersion == "" {
		data.AppVersion = bearstack.Version()
	}
	if data.WebDAVPath == "" {
		data.WebDAVPath = s.webDAVPath()
	}
	data.Auth = authPermissionsForRequest(s, r)
	if principal, ok := authPrincipalFromContext(r.Context()); ok {
		data.CustomPDFPreviewEnabled = s.customPDFPreviewForRequest(principal)
	}
	data.PhotoModuleEnabled = s.photos != nil
	if data.AppName == "" {
		data.AppName = defaultAppName
		if s.repo != nil {
			name, err := s.appName(ctx)
			if err != nil {
				if s.log != nil {
					s.log.Warn("app name setting failed", "error", err)
				}
			} else {
				data.AppName = name
			}
		}
	} else {
		data.AppName = normalizeAppName(data.AppName)
	}
	renderSettings := defaultRenderSettingsSnapshot()
	if s.repo != nil {
		settings, err := s.renderSettings(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Warn("render settings failed", "error", err)
			}
		} else {
			renderSettings = settings
		}
	}
	if data.TagDisplayMode == "" {
		data.TagDisplayMode = renderSettings.TagDisplayMode
	} else {
		data.TagDisplayMode = normalizeTagDisplayMode(data.TagDisplayMode)
	}
	if data.TagDisplayOptions == nil {
		data.TagDisplayOptions = tagDisplayOptions()
	}
	if s.repo != nil {
		data.DocumentCloudEnabled = renderSettings.DocumentCloudEnabled
	}
	if data.HomePageOptions == nil {
		data.HomePageOptions = homePageOptions(s.photos != nil, data.DocumentCloudEnabled)
	}
	if data.HomePage == "" {
		data.HomePage = s.resolveHomePage(renderSettings.HomePage, data.Auth, data.DocumentCloudEnabled)
	} else {
		data.HomePage = normalizeAvailableHomePage(data.HomePage, s.photos != nil, data.DocumentCloudEnabled)
	}
	if data.HomeURL == "" {
		data.HomeURL = homeURLForPermissions(data.HomePage, data.Auth, s.photos != nil)
	}
	if data.ThemeMode == "" {
		data.ThemeMode = renderSettings.ThemeMode
	} else {
		data.ThemeMode = normalizeThemeMode(data.ThemeMode)
	}
	if data.ThemeOptions == nil {
		data.ThemeOptions = themeOptions()
	}
	if data.TrashRetentionOptions == nil {
		data.TrashRetentionOptions = trashRetentionOptions()
	}
	if !data.CustomFavicon.Uploaded && s.repo != nil {
		icon, ok, err := s.customFavicon(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Warn("custom favicon setting failed", "error", err)
			}
		} else if ok {
			data.CustomFavicon = customFaviconView(icon)
		}
	}
	for i := range data.SearchFavorites {
		dateLabel := data.SearchFavorites[i].DateLabel
		data.SearchFavorites[i].Summary = searchFavoriteSummaryForDisplay(data.SearchFavorites[i].SearchFavorite, dateLabel, data.TagDisplayMode, data.CustomFields)
	}
	return data
}

func (s *Server) renderHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		s.renderError(w, r, http.StatusNotFound, errors.New("Dokument nicht gefunden"))
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	s.renderError(w, r, http.StatusInternalServerError, err)
}

func (s *Server) renderTagHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Tag nicht gefunden"), "/tags")
		return
	}
	s.renderHTTPError(w, r, err)
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, err error) {
	s.renderErrorWithReturn(w, r, status, err, "")
}

func (s *Server) renderErrorWithReturn(w http.ResponseWriter, r *http.Request, status int, err error, returnURL string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if status >= http.StatusInternalServerError && s.log != nil {
		s.log.Warn("request failed", "method", r.Method, "path", r.URL.Path, "status", status, "error", err)
	}
	if returnURL != "" {
		returnURL = safeReturnURL(returnURL)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.render(w, r, "error.html", PageData{
		Title:     "Fehler",
		Error:     publicErrorMessage(status, message),
		Active:    "",
		ReturnURL: returnURL,
	})
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) renderJSONError(w http.ResponseWriter, status int, message string) {
	if status >= http.StatusInternalServerError && s.log != nil {
		s.log.Warn("json request failed", "status", status, "error", message)
	}
	if err := writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: publicErrorMessage(status, message)}); err != nil {
		s.log.Warn("json error response failed", "error", err)
	}
}

func publicErrorMessage(status int, message string) string {
	if status >= http.StatusInternalServerError {
		return "Interner Serverfehler"
	}
	if strings.TrimSpace(message) == "" {
		return http.StatusText(status)
	}
	return message
}

func (s *Server) renderTagJSON(w http.ResponseWriter, tag document.Tag, displayMode ...string) {
	if err := writeJSON(w, http.StatusOK, tagAPIResponseFrom(tag, displayMode...)); err != nil {
		s.log.Warn("tag json response failed", "tag_id", tag.ID, "error", err)
	}
}

type tagAPIResponse struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	Description     string `json:"description"`
	Color           string `json:"color"`
	Kind            string `json:"kind,omitempty"`
	FavoriteID      int64  `json:"favorite_id,omitempty"`
	FieldID         int64  `json:"field_id,omitempty"`
	FieldLabel      string `json:"field_label,omitempty"`
	Value           string `json:"value,omitempty"`
	PrimaryTag      bool   `json:"primary_tag"`
	GroupMode       bool   `json:"group_mode"`
	ListHidden      bool   `json:"list_hidden"`
	DeleteProtected bool   `json:"delete_protected"`
	Style           string `json:"style"`
	Count           int    `json:"count"`
}

func tagAPIResponseFrom(tag document.Tag, displayMode ...string) tagAPIResponse {
	mode := tagDisplayModeLower
	if len(displayMode) > 0 {
		mode = normalizeTagDisplayMode(displayMode[0])
	}
	return tagAPIResponse{
		ID:              tag.ID,
		Name:            tag.Name,
		DisplayName:     formatTag(mode, tag.Name),
		Description:     tag.Description,
		Color:           tag.Color,
		PrimaryTag:      tag.PrimaryTag,
		GroupMode:       tag.GroupMode,
		ListHidden:      tag.ListHidden,
		DeleteProtected: tag.DeleteProtected,
		Style:           string(tagStyle(tag.Color)),
		Count:           tag.Count,
	}
}

func tagAPIResponsesFrom(tags []document.Tag, displayMode ...string) []tagAPIResponse {
	responses := make([]tagAPIResponse, len(tags))
	for i, tag := range tags {
		responses[i] = tagAPIResponseFrom(tag, displayMode...)
	}
	return responses
}

type folderPathAPIResponse struct {
	Kind       string `json:"kind"`
	Tag        string `json:"tag,omitempty"`
	FieldID    int64  `json:"field_id,omitempty"`
	FieldLabel string `json:"field_label,omitempty"`
	Value      string `json:"value,omitempty"`
	Label      string `json:"label"`
}

func folderPathAPIResponsesFrom(selection virtualFolderSelection) []folderPathAPIResponse {
	responses := make([]folderPathAPIResponse, 0, len(selection.Segments))
	for _, segment := range selection.Segments {
		response := folderPathAPIResponse{
			Kind:  segment.Kind,
			Label: virtualFolderSegmentLabel(segment),
		}
		switch segment.Kind {
		case virtualFolderSegmentTag:
			response.Tag = segment.Tag
		case virtualFolderSegmentFieldValue:
			response.FieldID = segment.FieldID
			response.FieldLabel = segment.FieldLabel
			response.Value = segment.Value
		}
		responses = append(responses, response)
	}
	return responses
}

func tagDescriptionMap(tags []document.Tag) map[string]string {
	descriptions := make(map[string]string, len(tags))
	for _, tag := range tags {
		descriptions[tag.Name] = tag.Description
	}
	return descriptions
}

func tagStyleMap(tags []document.Tag) map[string]template.CSS {
	styles := make(map[string]template.CSS, len(tags))
	for _, tag := range tags {
		styles[tag.Name] = tagStyle(tag.Color)
	}
	return styles
}

func tagListHiddenMap(tags []document.Tag) map[string]bool {
	hidden := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if tag.ListHidden {
			hidden[tag.Name] = true
		}
	}
	return hidden
}

func listVisibleTags(tags []string, hidden map[string]bool) []string {
	if len(tags) == 0 {
		return nil
	}
	visible := make([]string, 0, len(tags))
	for _, tag := range tags {
		if hidden != nil && hidden[tag] {
			continue
		}
		visible = append(visible, tag)
	}
	return visible
}

func listHiddenTags(tags []string, hidden map[string]bool) []string {
	if len(tags) == 0 || len(hidden) == 0 {
		return nil
	}
	hiddenTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		if hidden[tag] {
			hiddenTags = append(hiddenTags, tag)
		}
	}
	return hiddenTags
}

func formatTag(mode, tag string) string {
	return tagutil.DisplayName(mode, tag)
}

func formatFolderTag(mode, tag string) string {
	if tag == "Ordner" || tag == searchFavoritesFolderWebLabel {
		return tag
	}
	return formatTag(mode, tag)
}

func formatFolderCrumb(mode string, crumb FolderCrumb) string {
	if crumb.IsTag {
		return formatFolderTag(mode, crumb.Label)
	}
	return crumb.Label
}

func folderBreadcrumbLabel(mode string, crumbs []FolderCrumb) string {
	if len(crumbs) <= 1 {
		return ""
	}
	labels := make([]string, 0, len(crumbs)-1)
	for _, crumb := range crumbs[1:] {
		labels = append(labels, formatFolderCrumb(mode, crumb))
	}
	return strings.Join(labels, ", ")
}

func joinDisplayTags(mode string, tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	display := make([]string, 0, len(tags))
	for _, tag := range tags {
		display = append(display, formatTag(mode, tag))
	}
	return strings.Join(display, ", ")
}

func tagStyle(color string) template.CSS {
	color = tagutil.NormalizeColor(color)
	return template.CSS("--tag-color: " + color + "; --tag-text-color: " + tagutil.ReadableTextColor(color) + ";")
}

func hasTag(tags []string, name string) bool {
	for _, tag := range tags {
		if tag == name {
			return true
		}
	}
	return false
}

func customValue(values map[int64]string, id int64) string {
	if values == nil {
		return ""
	}
	return values[id]
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func writeJSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	return enc.Encode(value)
}

func contentDisposition(disposition, filename string) string {
	filename = strings.ReplaceAll(filename, "\r", "")
	filename = strings.ReplaceAll(filename, "\n", "")
	if filename == "" {
		filename = "document"
	}
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, escapeHeaderQuoted(filename), url.PathEscape(filename))
}

func escapeHeaderQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func formatNumber(value int) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	s := strconv.Itoa(value)
	if len(s) <= 3 {
		return sign + s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return sign + strings.Join(parts, ".")
}

func formatDate(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("02.01.2006")
}

func formatDateInput(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func formatDay(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("02.01.2006")
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("02.01.2006 15:04")
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	if d < time.Minute {
		return d.Round(100 * time.Millisecond).String()
	}
	if d < time.Hour {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}
