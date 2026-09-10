package document

import (
	"errors"
	"strings"
)

func DefaultMailImportSettings() MailImportSettings {
	return MailImportSettings{
		Port:                993,
		Security:            MailImportSecurityTLS,
		Mailbox:             "INBOX",
		PollIntervalMinutes: 15,
	}
}

func NormalizeMailImportSettings(settings MailImportSettings) MailImportSettings {
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
		settings.Security = MailImportSecurityTLS
	}
	if settings.Port == 0 {
		if settings.Security == MailImportSecurityTLS {
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

func ValidateMailImportSettings(settings MailImportSettings) error {
	if settings.Port < 1 || settings.Port > 65535 {
		return errors.New("IMAP-Port ist ungültig")
	}
	switch settings.Security {
	case MailImportSecurityTLS, MailImportSecuritySTARTTLS, MailImportSecurityNone:
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
