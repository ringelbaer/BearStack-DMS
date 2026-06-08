// Datei stellt einheitliche Logging-Helfer fuer Services und Hintergrundjobs bereit.
package server

import "log/slog"

func logWarn(log *slog.Logger, message string, args ...any) {
	if log != nil {
		log.Warn(message, args...)
	}
}

func logInfo(log *slog.Logger, message string, args ...any) {
	if log != nil {
		log.Info(message, args...)
	}
}
