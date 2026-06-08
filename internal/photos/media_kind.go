// Datei klassifiziert Mediendateien nach Typ, MIME-Information und unterstuetzten Erweiterungen.
package photos

import (
	"path/filepath"
	"strings"
)

const mediaKindGPX = "gpx"

func supportedKind(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".jpe", ".png", ".gif", ".webp", ".svg":
		return MediaTypeImage, true
	case ".mp4", ".webm", ".ogv", ".ogg":
		return MediaTypeVideo, true
	case ".mp3":
		return MediaTypeAudio, true
	case ".md":
		return MediaTypeBlog, true
	case ".gpx":
		return mediaKindGPX, true
	default:
		return "", false
	}
}

func isMediaKind(kind string) bool {
	return kind == MediaTypeImage || kind == MediaTypeVideo || kind == MediaTypeAudio
}
