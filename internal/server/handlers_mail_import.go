// Datei behandelt Konfiguration, Ausfuehrung und Statusanzeigen des Mail-Imports.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

func (s *Server) handleMailImportSettings(w http.ResponseWriter, r *http.Request) {
	settings, _, err := s.repo.GetMailImportSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "settings.html", PageData{
		Title:           "Einstellungen",
		Active:          "settings",
		SettingsTab:     "mail_import",
		MailImport:      settings,
		MailPasswordSet: settings.Password != "",
		Notice:          r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleSaveMailImportSettings(w http.ResponseWriter, r *http.Request) {
	current, _, err := s.repo.GetMailImportSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}

	settings, err := mailImportSettingsFromRequest(r, current)
	if err != nil {
		s.renderMailImportFormError(w, r, current, err)
		return
	}
	if err := repository.ValidateMailImportSettings(settings); err != nil {
		s.renderMailImportFormError(w, r, settings, err)
		return
	}
	if err := s.repo.SaveMailImportSettings(r.Context(), settings); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	redirectWithNotice(w, r, "/settings/mail-import", "E-Mail-Import gespeichert.")
}

func (s *Server) handleTestMailImportSettings(w http.ResponseWriter, r *http.Request) {
	current, _, err := s.repo.GetMailImportSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}

	settings, err := mailImportSettingsFromRequest(r, current)
	if err != nil {
		s.renderMailImportFormError(w, r, current, err)
		return
	}
	mailImport := s.mailImportService()
	if err := mailImport.CheckSettings(r.Context(), settings); err != nil {
		s.log.Warn("mail import settings test failed", "host", settings.Host, "mailbox", settings.Mailbox, "error", err)
		mailImport.RecordAudit(r.Context(), "E-Mail-Verbindungstest fehlgeschlagen", err.Error(), httpStatusError)
		s.renderMailImportFormError(w, r, settings, err)
		return
	}
	mailImport.RecordAudit(r.Context(), "E-Mail-Verbindungstest erfolgreich", settings.Host+" / "+settings.Mailbox, httpStatusOK)
	s.renderMailImportForm(w, r, http.StatusOK, settings, "E-Mail-Einstellungen erfolgreich geprüft.", "")
}

func (s *Server) handleRunMailImportNow(w http.ResponseWriter, r *http.Request) {
	settings, _, err := s.repo.GetMailImportSettings(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if err := repository.ValidateMailImportSettings(settings); err != nil {
		s.renderMailImportFormError(w, r, settings, err)
		return
	}

	mailImport := s.mailImportService()
	result, err := mailImport.ImportPDFs(r.Context(), settings)
	if err != nil {
		s.log.Warn("manual mail import failed", "host", settings.Host, "mailbox", settings.Mailbox, "error", err)
		mailImport.RecordAudit(r.Context(), "E-Mail-Import manuell fehlgeschlagen", err.Error(), httpStatusError)
		s.renderMailImportFormError(w, r, settings, err)
		return
	}

	notice := manualMailImportNotice(result)
	status := httpStatusOK
	action := "E-Mail-Import manuell abgeschlossen"
	if result.Errors > 0 {
		status = httpStatusError
		action = "E-Mail-Import manuell mit Fehlern"
	}
	mailImport.RecordAudit(r.Context(), action, notice, status)
	s.renderMailImportForm(w, r, http.StatusOK, settings, notice, "")
}

func manualMailImportNotice(result mailImportRunResult) string {
	if result.Messages == 0 {
		return "Keine E-Mails zum Abrufen gefunden."
	}
	notice := fmt.Sprintf("%d Mail(s) geprüft, %d PDF(s) importiert, %d E-Mail-Archiv(e), %d Duplikat(e), %d abgelehnt, %d gelöscht", result.Messages, result.Uploaded, result.Archived, result.Duplicates, result.Rejected, result.Deleted)
	if result.Errors > 0 {
		notice = fmt.Sprintf("%s, %d Fehler", notice, result.Errors)
	}
	return notice + "."
}

func mailImportSettingsFromRequest(r *http.Request, current document.MailImportSettings) (document.MailImportSettings, error) {
	port, err := strconv.Atoi(r.FormValue("port"))
	if err != nil {
		return document.MailImportSettings{}, errors.New("IMAP-Port ist ungültig")
	}
	pollInterval, err := strconv.Atoi(r.FormValue("poll_interval_minutes"))
	if err != nil {
		return document.MailImportSettings{}, errors.New("Abrufhäufigkeit ist ungültig")
	}

	settings := document.MailImportSettings{
		Enabled:             r.FormValue("enabled") == "1",
		Host:                r.FormValue("host"),
		Port:                port,
		Security:            r.FormValue("security"),
		Username:            r.FormValue("username"),
		Password:            r.FormValue("password"),
		Mailbox:             r.FormValue("mailbox"),
		PollIntervalMinutes: pollInterval,
		AllowedSenders:      r.FormValue("allowed_senders"),
	}
	if settings.Password == "" {
		settings.Password = current.Password
	}
	return repository.NormalizeMailImportSettings(settings), nil
}

func (s *Server) renderMailImportFormError(w http.ResponseWriter, r *http.Request, settings document.MailImportSettings, err error) {
	s.renderMailImportForm(w, r, http.StatusBadRequest, settings, "", err.Error())
}

func (s *Server) renderMailImportForm(w http.ResponseWriter, r *http.Request, status int, settings document.MailImportSettings, notice string, errorText string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.render(w, r, "settings.html", PageData{
		Title:           "Einstellungen",
		Active:          "settings",
		SettingsTab:     "mail_import",
		MailImport:      settings,
		MailPasswordSet: settings.Password != "",
		Notice:          notice,
		Error:           errorText,
	})
}
