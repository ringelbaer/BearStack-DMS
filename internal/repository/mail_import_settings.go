// Datei speichert Einstellungen fuer den Mail-Import und stellt sie der Anwendung bereit.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"bearstack/internal/document"
)

const MailImportSettingsKey = "mail_import_settings"

func (r *Repository) GetMailImportSettings(ctx context.Context) (document.MailImportSettings, bool, error) {
	value, ok, err := r.GetSetting(ctx, MailImportSettingsKey)
	if err != nil {
		return DefaultMailImportSettings(), false, err
	}
	if !ok {
		return DefaultMailImportSettings(), false, nil
	}
	var settings document.MailImportSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultMailImportSettings(), true, nil
	}
	settings = NormalizeMailImportSettings(settings)
	return settings, true, nil
}

func (r *Repository) SaveMailImportSettings(ctx context.Context, settings document.MailImportSettings) error {
	settings = NormalizeMailImportSettings(settings)
	if err := ValidateMailImportSettings(settings); err != nil {
		return err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return r.SaveSetting(ctx, MailImportSettingsKey, string(payload))
}

func DefaultMailImportSettings() document.MailImportSettings {
	return document.MailImportSettings{
		Port:                993,
		Security:            document.MailImportSecurityTLS,
		Mailbox:             "INBOX",
		PollIntervalMinutes: 15,
	}
}

func NormalizeMailImportSettings(settings document.MailImportSettings) document.MailImportSettings {
	settings.Host = strings.TrimSpace(settings.Host)
	settings.Security = strings.ToLower(strings.TrimSpace(settings.Security))
	settings.Username = strings.TrimSpace(settings.Username)
	settings.Password = strings.TrimSpace(settings.Password)
	settings.Mailbox = strings.TrimSpace(settings.Mailbox)
	settings.AllowedSenders = normalizeMailImportAllowedSenders(settings.AllowedSenders)
	if settings.Mailbox == "" {
		settings.Mailbox = "INBOX"
	}
	if settings.Security == "" {
		settings.Security = document.MailImportSecurityTLS
	}
	if settings.Port == 0 {
		if settings.Security == document.MailImportSecurityTLS {
			settings.Port = 993
		} else {
			settings.Port = 143
		}
	}
	if settings.PollIntervalMinutes == 0 {
		settings.PollIntervalMinutes = 15
	}
	return settings
}

func normalizeMailImportAllowedSenders(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

func ValidateMailImportSettings(settings document.MailImportSettings) error {
	if settings.Port < 1 || settings.Port > 65535 {
		return errors.New("IMAP-Port ist ungültig")
	}
	switch settings.Security {
	case document.MailImportSecurityTLS, document.MailImportSecuritySTARTTLS, document.MailImportSecurityNone:
	default:
		return errors.New("IMAP-Verschlüsselung ist ungültig")
	}
	if settings.PollIntervalMinutes < 1 || settings.PollIntervalMinutes > 1440 {
		return errors.New("Abrufhäufigkeit muss zwischen 1 und 1440 Minuten liegen")
	}
	if settings.Enabled {
		if settings.Host == "" {
			return errors.New("IMAP-Server fehlt")
		}
		if settings.Username == "" {
			return errors.New("IMAP-Benutzername fehlt")
		}
		if settings.Password == "" {
			return errors.New("IMAP-Passwort fehlt")
		}
	}
	return nil
}
