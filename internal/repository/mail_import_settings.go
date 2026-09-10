// Datei speichert Einstellungen fuer den Mail-Import und stellt sie der Anwendung bereit.
package repository

import (
	"context"
	"encoding/json"

	"bearstack/internal/document"
)

const MailImportSettingsKey = "mail_import_settings"

func (r *Repository) GetMailImportSettings(ctx context.Context) (document.MailImportSettings, bool, error) {
	value, ok, err := r.GetSetting(ctx, MailImportSettingsKey)
	if err != nil {
		return document.DefaultMailImportSettings(), false, err
	}
	if !ok {
		return document.DefaultMailImportSettings(), false, nil
	}
	var settings document.MailImportSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return document.DefaultMailImportSettings(), true, nil
	}
	settings = document.NormalizeMailImportSettings(settings)
	return settings, true, nil
}

func (r *Repository) SaveMailImportSettings(ctx context.Context, settings document.MailImportSettings) error {
	settings = document.NormalizeMailImportSettings(settings)
	if err := document.ValidateMailImportSettings(settings); err != nil {
		return err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return r.SaveSetting(ctx, MailImportSettingsKey, string(payload))
}
