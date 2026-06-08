// Datei rendert Einstellungsseiten und verarbeitet Anwendungskonfiguration aus Formularen.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	previewMode, err := s.desktopPreviewMode(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	retentionDays, err := s.trashRetentionDays(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	folderTagMinDocuments, err := s.folderTagMinDocuments(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	cloudEnabled, err := s.documentCloudEnabled(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "settings.html", PageData{
		Title:                 "Einstellungen",
		Active:                "settings",
		SettingsTab:           "main",
		DesktopPreviewMode:    previewMode,
		DocumentCloudEnabled:  cloudEnabled,
		FolderTagMinDocuments: folderTagMinDocuments,
		TrashRetentionDays:    retentionDays,
		TrashRetentionOptions: trashRetentionOptions(),
		Notice:                r.URL.Query().Get("notice"),
	})
}

func (s *Server) handlePhotoSettings(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderError(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."))
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "settings.html", PageData{
		Title:         "Einstellungen",
		Active:        "settings",
		SettingsTab:   "photos",
		PhotoSettings: settings,
		Notice:        r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	appName := normalizeAppName(r.FormValue("app_name"))
	if err := s.repo.SaveSetting(r.Context(), appNameSettingKey, appName); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.cacheAppName(appName)
	previewMode := normalizeDesktopPreviewMode(r.FormValue("desktop_preview_mode"))
	if err := s.repo.SaveSetting(r.Context(), desktopPreviewModeSettingKey, previewMode); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	tagDisplayMode := normalizeTagDisplayMode(r.FormValue("tag_display_mode"))
	if err := s.repo.SaveSetting(r.Context(), tagDisplayModeSettingKey, tagDisplayMode); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	themeMode := normalizeThemeMode(r.FormValue("theme_mode"))
	if err := s.repo.SaveSetting(r.Context(), themeModeSettingKey, themeMode); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	cloudEnabled := r.FormValue("document_cloud_enabled") == "1"
	if err := s.repo.SaveSetting(r.Context(), documentCloudEnabledSettingKey, boolSettingValue(cloudEnabled)); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	homePage := normalizeAvailableHomePage(r.FormValue("home_page"), s.photos != nil, cloudEnabled)
	if err := s.repo.SaveSetting(r.Context(), homePageSettingKey, homePage); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	retentionDays := normalizeTrashRetentionDays(r.FormValue("trash_retention_days"))
	if err := s.repo.SaveSetting(r.Context(), trashRetentionDaysSettingKey, strconv.Itoa(retentionDays)); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	folderTagMinDocuments := normalizeFolderTagMinDocuments(r.FormValue("folder_tag_min_documents"))
	if err := s.repo.SaveSetting(r.Context(), folderTagMinDocumentsSettingKey, strconv.Itoa(folderTagMinDocuments)); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.cacheRenderSettings(renderSettingsSnapshot{
		TagDisplayMode:       tagDisplayMode,
		ThemeMode:            themeMode,
		HomePage:             homePage,
		DocumentCloudEnabled: cloudEnabled,
	})
	purged, err := s.purgeTrashByRetention(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	notice := "Einstellungen gespeichert."
	if purged > 0 {
		notice = fmt.Sprintf("Einstellungen gespeichert. %d Dokument(e) aus dem Papierkorb endgültig gelöscht.", purged)
	}
	redirectWithNotice(w, r, "/settings", notice)
}

func (s *Server) handleSavePhotoSettings(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderError(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."))
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	settings := photoSettingsFromRequest(r)
	if err := s.savePhotoSettings(r.Context(), settings); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.configurePhotoThumbnailer(settings)
	redirectWithNotice(w, r, "/settings/photos", "Foto-Einstellungen gespeichert.")
}

func (s *Server) handleRunPhotoThumbnailWorkerNow(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderError(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."))
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !s.startPhotoThumbnailJob(settings) {
		redirectWithNotice(w, r, "/settings/photos", "Ein Foto-Hintergrundjob läuft bereits.")
		return
	}
	redirectWithNotice(w, r, "/settings/photos", "Thumbnail-Erzeugung im Hintergrund gestartet.")
}

func (s *Server) handleRunPhotoIndexWorkerNow(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderError(w, r, http.StatusNotFound, errors.New("Foto-Modul ist nicht aktiv."))
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !s.startPhotoIndexJob(settings) {
		redirectWithNotice(w, r, "/settings/photos", "Ein Foto-Hintergrundjob läuft bereits.")
		return
	}
	redirectWithNotice(w, r, "/settings/photos", "Foto-Indexierung im Hintergrund gestartet.")
}
