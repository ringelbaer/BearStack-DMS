package documentimport

import (
	"errors"
	"strings"

	"bearstack/internal/storage"
)

func UploadErrorMessage(err error) string {
	if errors.Is(err, storage.ErrFileTooLarge) {
		return "Datei überschreitet die konfigurierte Maximalgröße"
	}
	if errors.Is(err, storage.ErrInvalidFilename) {
		return "Dateiname ist ungültig"
	}
	if errors.Is(err, storage.ErrUnsupportedFileType) {
		return "Dateityp wird nicht unterstützt"
	}
	if IsIncompleteRequestBodyError(err) {
		return "Upload unvollständig übertragen"
	}
	return "Datei konnte nicht verarbeitet werden"
}

func IsIncompleteRequestBodyError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "content-length") && strings.Contains(text, "only wrote")
}

func ImportErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return "Dokument konnte nicht importiert werden"
}
