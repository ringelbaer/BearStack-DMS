// Datei rendert Einstellungsseiten und verarbeitet Anwendungskonfiguration aus Formularen.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) handleGeneralSettings(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "settings.html", PageData{
		Title:       "Einstellungen",
		Active:      "settings",
		SettingsTab: "general",
		Notice:      r.URL.Query().Get("notice"),
	})
}

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
	section := r.PostForm.Get("settings_section")
	if section != "" && section != "general" && section != "documents" {
		s.renderError(w, r, http.StatusBadRequest, errors.New("Ungültiger Einstellungsbereich."))
		return
	}
	// Forms without a section retain the original combined save behavior.
	saveGeneral := section != "documents"
	saveDocuments := section != "general"
	cloudEnabled, err := s.documentCloudEnabled(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if saveDocuments {
		cloudEnabled = r.PostForm.Get("document_cloud_enabled") == "1"
	}
	values := make(map[string]string)
	if saveGeneral {
		values[appNameSettingKey] = normalizeAppName(r.PostForm.Get("app_name"))
		values[tagDisplayModeSettingKey] = normalizeTagDisplayMode(r.PostForm.Get("tag_display_mode"))
		values[themeModeSettingKey] = normalizeThemeMode(r.PostForm.Get("theme_mode"))
		values[homePageSettingKey] = normalizeAvailableHomePage(r.PostForm.Get("home_page"), s.photos != nil, cloudEnabled)
	}
	if saveDocuments {
		values[desktopPreviewModeSettingKey] = normalizeDesktopPreviewMode(r.PostForm.Get("desktop_preview_mode"))
		values[documentCloudEnabledSettingKey] = boolSettingValue(cloudEnabled)
		values[trashRetentionDaysSettingKey] = strconv.Itoa(normalizeTrashRetentionDays(r.PostForm.Get("trash_retention_days")))
		values[folderTagMinDocumentsSettingKey] = strconv.Itoa(normalizeFolderTagMinDocuments(r.PostForm.Get("folder_tag_min_documents")))
	}
	if err := s.settingsService().SaveSettings(r.Context(), values); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if !saveDocuments {
		redirectWithNotice(w, r, "/settings/general", "Allgemeine Einstellungen gespeichert.")
		return
	}
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
