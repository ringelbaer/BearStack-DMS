// Datei liest Mediendateien aus dem Dateisystem und erstellt daraus Foto-Medienmodelle.
package photos

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/textmeta"
)

func (l *Library) Media(rel string) (Media, error) {
	return l.MediaContext(context.Background(), rel)
}

func (l *Library) MediaContext(ctx context.Context, rel string) (Media, error) {
	clean, err := CleanPath(rel)
	if err != nil {
		return Media{}, err
	}
	kind, ok := supportedKind(clean)
	if !ok || !isMediaKind(kind) {
		return Media{}, os.ErrNotExist
	}
	return l.mediaFromPath(ctx, clean)
}

func (l *Library) mediaFromPath(ctx context.Context, rel string) (Media, error) {
	media, changed, err := l.mediaFromPathData(rel)
	if err != nil {
		return Media{}, err
	}
	if changed {
		if err := l.saveMediaContext(ctx, media); err != nil {
			l.logWriteError("photo media cache update failed", media.Path, err)
		}
	}
	return media, nil
}

func (l *Library) logWriteError(message, path string, err error) {
	if err == nil {
		return
	}
	slog.Default().Warn(message, "path", path, "error", err)
}

func (l *Library) mediaFromPathData(rel string) (Media, bool, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return Media{}, false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Media{}, false, err
	}
	if info.IsDir() {
		return Media{}, false, os.ErrNotExist
	}
	kind, ok := supportedKind(rel)
	if !ok || !isMediaKind(kind) {
		return Media{}, false, os.ErrNotExist
	}
	adminOnly := l.directoryAdminOnly(parentPath(rel))
	return l.mediaFromPathInfo(rel, abs, info, kind, nil, adminOnly, true)
}

func (l *Library) mediaFromPathInfo(rel, abs string, info os.FileInfo, kind string, cache map[string]cachedMediaRow, adminOnly, fallbackCapturedAt bool) (Media, bool, error) {
	if info.IsDir() {
		return Media{}, false, os.ErrNotExist
	}
	media := Media{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		Directory: parentPath(rel),
		Type:      kind,
		MIMEType:  mimeTypeForPath(rel, kind),
		SizeBytes: info.Size(),
		ModTime:   info.ModTime(),
		AdminOnly: adminOnly,
	}
	if kind == MediaTypeImage {
		media.XMPFingerprint = xmpSidecarFingerprint(abs)
	}
	if row, ok := cache[media.Path]; ok {
		if cached, ok := cachedMediaFromRow(media, row); ok {
			return cached, false, nil
		}
	} else if cache == nil {
		if cached, ok := l.cachedMedia(media); ok {
			return cached, false, nil
		}
	}
	if kind == MediaTypeImage && info.Size() > 0 {
		if meta, err := readMetadata(abs); err == nil {
			media.CapturedAt = meta.CapturedAt
			media.Width = meta.Width
			media.Height = meta.Height
			media.Orientation = orientationName(meta.Orientation, meta.Width, meta.Height)
			media.Camera = meta.Camera
			media.Lens = meta.Lens
			media.Rating = meta.Rating
			media.Latitude = meta.Latitude
			media.Longitude = meta.Longitude
			media.Keywords = meta.Keywords
			media.Faces = meta.Faces
		}
	}
	if media.CapturedAt == nil {
		media.CapturedAt = mediaDateFromFilename(media.Name)
		if media.CapturedAt == nil && fallbackCapturedAt {
			captured := info.ModTime()
			media.CapturedAt = &captured
		}
	}
	return media, true, nil
}

func (l *Library) quickMediaFromPathInfo(rel string, info os.FileInfo, kind string, adminOnly bool) Media {
	media := Media{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		Directory: parentPath(rel),
		Type:      kind,
		MIMEType:  mimeTypeForPath(rel, kind),
		SizeBytes: info.Size(),
		ModTime:   info.ModTime(),
		AdminOnly: adminOnly,
	}
	if captured := mediaDateFromFilename(media.Name); captured != nil {
		media.CapturedAt = captured
	} else {
		captured := info.ModTime()
		media.CapturedAt = &captured
	}
	return media
}

func mediaDateFromFilename(name string) *time.Time {
	if !strings.Contains(name, "19") && !strings.Contains(name, "20") {
		return nil
	}
	_, date := textmeta.FromFilename(name)
	if date != nil {
		return date
	}
	return nil
}

func imageSize(path string) (int, int) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func mimeTypeForPath(path, kind string) string {
	if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); detected != "" {
		return strings.Split(detected, ";")[0]
	}
	switch kind {
	case MediaTypeImage:
		return "image/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	case MediaTypeVideo:
		return "video/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	case MediaTypeAudio:
		return "audio/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	default:
		return "application/octet-stream"
	}
}

func orientationName(exifOrientation, width, height int) string {
	if exifOrientation >= 5 && exifOrientation <= 8 {
		width, height = height, width
	}
	if width > 0 && height > 0 {
		if height > width {
			return "portrait"
		}
		if width > height {
			return "landscape"
		}
		return "square"
	}
	return ""
}
