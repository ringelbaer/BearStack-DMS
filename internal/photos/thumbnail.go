// Datei erzeugt, speichert und liefert Thumbnail-Dateien fuer Foto- und Videomedien.
package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bearstack/internal/fsutil"
)

const (
	// DefaultThumbnailSize is used when callers do not request a concrete size.
	DefaultThumbnailSize = 420

	// MinThumbnailSize and MaxThumbnailSize are the library-wide thumbnail bounds.
	MinThumbnailSize = 64
	MaxThumbnailSize = 3840

	thumbnailWebPQuality = 80
)

var errThumbnailBatchComplete = errors.New("thumbnail batch complete")

var thumbnailBackendCache sync.Map

func CanThumbnail(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".jpe", ".png", ".gif", ".webp":
		return true
	case ".mp4", ".webm", ".ogv", ".ogg":
		return true
	default:
		return false
	}
}

// NormalizeThumbnailSize clamps one requested thumbnail size to the supported range.
func NormalizeThumbnailSize(size int) int {
	if size <= 0 {
		return DefaultThumbnailSize
	}
	if size < MinThumbnailSize {
		return MinThumbnailSize
	}
	if size > MaxThumbnailSize {
		return MaxThumbnailSize
	}
	return size
}

func (l *Library) ThumbnailReady(rel string, size int) (bool, error) {
	return l.ThumbnailReadyContext(context.Background(), rel, size)
}

func (l *Library) ThumbnailReadyContext(ctx context.Context, rel string, size int) (bool, error) {
	if l == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	size = NormalizeThumbnailSize(size)
	clean, err := CleanPath(rel)
	if err != nil {
		return false, err
	}
	kind, ok := supportedKind(clean)
	if !ok || (kind != MediaTypeImage && kind != MediaTypeVideo) || !CanThumbnail(clean) {
		return false, errThumbnailUnavailable
	}
	source, err := l.Resolve(clean)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return false, err
	}
	media := Media{
		Name:      filepath.Base(filepath.FromSlash(clean)),
		Path:      clean,
		Directory: parentPath(clean),
		Type:      kind,
		SizeBytes: info.Size(),
		ModTime:   info.ModTime(),
	}
	return l.thumbnailReadyForMedia(ctx, media, size), nil
}

func (l *Library) CachedThumbnailReady(rel string, size int) bool {
	if l == nil {
		return false
	}
	size = NormalizeThumbnailSize(size)
	clean, err := CleanPath(rel)
	if err != nil || !CanThumbnail(clean) {
		return false
	}
	return l.thumbnailFileReady(clean, size)
}

func (l *Library) CachedThumbnailReadyForMedia(media Media, size int) bool {
	if l == nil || !mediaCanThumbnail(media) {
		return false
	}
	return l.thumbnailFileReadyForSource(media.Path, NormalizeThumbnailSize(size), media.ModTime)
}

func (l *Library) CachedThumbnailContext(ctx context.Context, rel string, size int, includeAdminOnly bool) (string, bool, error) {
	if l == nil || !l.index.available() {
		return "", false, nil
	}
	size = NormalizeThumbnailSize(size)
	clean, err := CleanPath(rel)
	if err != nil {
		return "", false, err
	}
	kind, ok := supportedKind(clean)
	if !ok || (kind != MediaTypeImage && kind != MediaTypeVideo) || !CanThumbnail(clean) {
		return "", false, errThumbnailUnavailable
	}
	var mediaType string
	var adminOnly int
	var sizeBytes int64
	var modUnix int64
	err = l.index.db.QueryRowContext(ctx, `SELECT type, admin_only, size_bytes, mod_time_unix_nano FROM media_index WHERE path = ?`, clean).Scan(&mediaType, &adminOnly, &sizeBytes, &modUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if mediaType != MediaTypeImage && mediaType != MediaTypeVideo {
		return "", false, errThumbnailUnavailable
	}
	if !includeAdminOnly && adminOnly != 0 {
		return "", false, errAdminOnly
	}
	media := Media{
		Name:      filepath.Base(filepath.FromSlash(clean)),
		Path:      clean,
		Directory: parentPath(clean),
		Type:      mediaType,
		SizeBytes: sizeBytes,
		ModTime:   time.Unix(0, modUnix),
	}
	cachePath, ready := l.thumbnailReadyCachePathForMedia(ctx, media, size)
	if !ready {
		return "", false, nil
	}
	_ = l.markThumbnailGenerated(ctx, media, size)
	return cachePath, true, nil
}

func (l *Library) Thumbnail(ctx context.Context, rel string, size int) (string, error) {
	size = NormalizeThumbnailSize(size)
	media, err := l.MediaContext(ctx, rel)
	if err != nil {
		return "", err
	}
	return l.thumbnailForMedia(ctx, media, size)
}

func (l *Library) thumbnailForMedia(ctx context.Context, media Media, size int) (string, error) {
	size = NormalizeThumbnailSize(size)
	if (media.Type != MediaTypeImage && media.Type != MediaTypeVideo) || !CanThumbnail(media.Path) {
		return "", errThumbnailUnavailable
	}
	source, err := l.Resolve(media.Path)
	if err != nil {
		return "", err
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	media.ModTime = sourceInfo.ModTime()
	media.SizeBytes = sourceInfo.Size()
	cachePath := l.thumbnailCachePath(media.Path, size)
	if readyPath, ready := l.thumbnailReadyCachePathForMedia(ctx, media, size); ready {
		_ = l.markThumbnailGenerated(ctx, media, size)
		return readyPath, nil
	}
	return l.thumbnail.flight(ctx, cachePath, func() (string, error) {
		if readyPath, ready := l.thumbnailReadyCachePathForMedia(ctx, media, size); ready {
			_ = l.markThumbnailGenerated(ctx, media, size)
			return readyPath, nil
		}
		release, err := l.thumbnail.acquireSlot(ctx)
		if err != nil {
			return "", err
		}
		defer release()
		if readyPath, ready := l.thumbnailReadyCachePathForMedia(ctx, media, size); ready {
			_ = l.markThumbnailGenerated(ctx, media, size)
			return readyPath, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			return "", err
		}
		tmp, err := thumbnailTempPath(cachePath)
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp)
		_ = os.Remove(tmp)
		thumbnailSource := source
		thumbnailType := media.Type
		if larger, ok := l.cachedLargerThumbnailSource(ctx, media, size); ok {
			thumbnailSource = larger
			thumbnailType = MediaTypeImage
		}
		if err := writeMediaThumbnail(ctx, thumbnailType, thumbnailSource, tmp, size); err != nil {
			_ = l.markThumbnailFailed(ctx, media, size, err)
			return "", err
		}
		if !fsutil.FileHasContent(tmp) {
			err := fmt.Errorf("empty thumbnail")
			_ = l.markThumbnailFailed(ctx, media, size, err)
			return "", err
		}
		if err := os.Rename(tmp, cachePath); err != nil {
			_ = l.markThumbnailFailed(ctx, media, size, err)
			return "", err
		}
		_ = l.markThumbnailGenerated(ctx, media, size)
		return cachePath, nil
	})
}

func (l *Library) EnsureThumbnails(ctx context.Context, sizes []int, batchSize int) (int, error) {
	if l == nil {
		return 0, nil
	}
	sizes = NormalizeThumbnailSizes(sizes)
	if len(sizes) == 0 {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 25
	}
	if l.index.available() {
		indexUsable, err := l.thumbnailIndexUsable(ctx)
		if err != nil {
			return 0, err
		}
		if indexUsable {
			generated, usedIndex, err := l.ensureThumbnailsFromIndex(ctx, sizes, batchSize)
			if err != nil || usedIndex {
				return generated, err
			}
		}
	}
	return l.ensureThumbnailsFromFilesystem(ctx, sizes, batchSize)
}

func (l *Library) thumbnailIndexUsable(ctx context.Context) (bool, error) {
	rootSignature, rootQuickSignature, ok, err := l.loadRootFolderScan(ctx)
	if err != nil || !ok {
		return false, err
	}
	info, err := os.Stat(l.root)
	if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return false, err
	}
	if rootQuickSignature != folderQuickScanSignature(info, entries) {
		return false, nil
	}
	return rootSignature == folderScanSignature(info, entries), nil
}

func (l *Library) ensureThumbnailsFromIndex(ctx context.Context, sizes []int, batchSize int) (int, bool, error) {
	generated := 0
	usedIndex := true
	var firstErr error
	candidateLimit := batchSize * 8
	if candidateLimit < 50 {
		candidateLimit = 50
	}
	if err := l.index.ensureThumbnailQueueForSizes(ctx, sizes, candidateLimit); err != nil {
		return 0, false, err
	}
	candidates, err := l.index.thumbnailCandidates(ctx, sizes, candidateLimit)
	if err != nil {
		return 0, usedIndex, err
	}
	now := time.Now().UTC()
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return generated, usedIndex, err
		}
		if generated >= batchSize {
			return generated, usedIndex, nil
		}
		if candidate.Status == thumbnailStatusFailed && thumbnailFailureBackoffActive(candidate.Attempts, candidate.LastAttemptAt, now) {
			continue
		}
		if l.thumbnailReadyForMedia(ctx, candidate.Media, candidate.Size) {
			_ = l.markThumbnailGenerated(ctx, candidate.Media, candidate.Size)
			continue
		}
		if _, err := l.thumbnailForMedia(ctx, candidate.Media, candidate.Size); err == nil {
			generated++
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if generated == 0 && firstErr != nil {
		return generated, usedIndex, firstErr
	}
	return generated, usedIndex, nil
}

func (l *Library) ensureThumbnailsFromFilesystem(ctx context.Context, sizes []int, batchSize int) (int, error) {
	generated := 0
	var firstErr error
	err := walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root:             l.root,
		IncludeAdminOnly: true,
	}, func(path string, entry os.DirEntry, _ string) error {
		if generated >= batchSize {
			return errThumbnailBatchComplete
		}
		if !CanThumbnail(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		media, err := l.mediaFromPath(ctx, filepath.ToSlash(rel))
		if err != nil || (media.Type != MediaTypeImage && media.Type != MediaTypeVideo) {
			return nil
		}
		for _, size := range sizes {
			if generated >= batchSize {
				return errThumbnailBatchComplete
			}
			if l.thumbnailFileReadyForSource(media.Path, size, media.ModTime) {
				continue
			}
			if _, err := l.thumbnailForMedia(ctx, media, size); err == nil {
				generated++
			} else if firstErr == nil {
				firstErr = err
			}
		}
		return nil
	})
	if errors.Is(err, errThumbnailBatchComplete) {
		err = nil
	}
	if err != nil {
		return generated, err
	}
	if generated == 0 && firstErr != nil {
		return generated, firstErr
	}
	return generated, nil
}

func (l *Library) thumbnailCachePath(rel string, size int) string {
	key := thumbnailCacheKey(rel)
	return filepath.Join(l.cacheDir, "thumbnails", "v2", key[:2], key[2:4], fmt.Sprintf("%s_%dq%d.webp", key, NormalizeThumbnailSize(size), thumbnailWebPQuality))
}

func (l *Library) legacyThumbnailCachePath(rel string, size int) string {
	clean := filepath.Clean(filepath.FromSlash(canonicalThumbnailCacheRel(rel)))
	if clean == "." || clean == string(filepath.Separator) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		clean = filepath.Base(clean)
		if clean == "." || clean == string(filepath.Separator) {
			clean = "media"
		}
	}
	dir := filepath.Dir(clean)
	if dir == "." {
		dir = ""
	}
	name := filepath.Base(clean)
	if name == "." || name == string(filepath.Separator) {
		name = "media"
	}
	return filepath.Join(l.cacheDir, "thumbnails", dir, fmt.Sprintf("%s_%dq%d.webp", name, size, thumbnailWebPQuality))
}

func (l *Library) thumbnailCachePaths(rel string, size int) []string {
	size = NormalizeThumbnailSize(size)
	primary := l.thumbnailCachePath(rel, size)
	legacy := l.legacyThumbnailCachePath(rel, size)
	if legacy == primary {
		return []string{primary}
	}
	return []string{primary, legacy}
}

func (l *Library) thumbnailReadyCachePathForMedia(ctx context.Context, media Media, size int) (string, bool) {
	if l == nil {
		return "", false
	}
	for _, cachePath := range l.thumbnailCachePaths(media.Path, size) {
		if !thumbnailFileReadyForSource(cachePath, media.ModTime) {
			continue
		}
		if !l.thumbnailIndexAllowsReady(ctx, media, size) {
			return "", false
		}
		return cachePath, true
	}
	return "", false
}

func (l *Library) thumbnailReadyForMedia(ctx context.Context, media Media, size int) bool {
	_, ok := l.thumbnailReadyCachePathForMedia(ctx, media, size)
	return ok
}

func (l *Library) thumbnailFileReady(rel string, size int) bool {
	if l == nil {
		return false
	}
	for _, cachePath := range l.thumbnailCachePaths(rel, size) {
		if fsutil.FileHasContent(cachePath) {
			return true
		}
	}
	return false
}

func (l *Library) thumbnailFileReadyForSource(rel string, size int, modTime time.Time) bool {
	if l == nil {
		return false
	}
	for _, cachePath := range l.thumbnailCachePaths(rel, size) {
		if thumbnailFileReadyForSource(cachePath, modTime) {
			return true
		}
	}
	return false
}

func thumbnailCacheKey(rel string) string {
	sum := sha256.Sum256([]byte(canonicalThumbnailCacheRel(rel)))
	return hex.EncodeToString(sum[:])
}

func canonicalThumbnailCacheRel(rel string) string {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == string(filepath.Separator) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		clean = filepath.Base(clean)
		if clean == "." || clean == string(filepath.Separator) {
			clean = "media"
		}
	}
	return filepath.ToSlash(clean)
}

// NormalizeThumbnailSizes clamps, deduplicates, and sorts explicit thumbnail sizes.
func NormalizeThumbnailSizes(values []int) []int {
	seen := map[int]struct{}{}
	sizes := make([]int, 0, len(values))
	for _, size := range values {
		if size <= 0 {
			continue
		}
		size = NormalizeThumbnailSize(size)
		if _, ok := seen[size]; ok {
			continue
		}
		seen[size] = struct{}{}
		sizes = append(sizes, size)
	}
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i] > sizes[j]
	})
	return sizes
}

func writeMediaThumbnail(ctx context.Context, mediaType, source, target string, size int) error {
	if mediaType == MediaTypeVideo {
		return writeVideoThumbnail(ctx, source, target, size)
	}
	return writeImageThumbnail(ctx, source, target, size)
}

func writeVideoThumbnail(ctx context.Context, source, target string, size int) error {
	err := extractVideoThumbnail(ctx, source, target, size, "1")
	if err == nil && fsutil.FileHasContent(target) {
		return nil
	}
	_ = os.Remove(target)
	if fallbackErr := extractVideoThumbnail(ctx, source, target, size, "0"); fallbackErr != nil {
		if err != nil {
			return err
		}
		return fallbackErr
	}
	if !fsutil.FileHasContent(target) {
		return fmt.Errorf("video thumbnail ffmpeg produced no frame")
	}
	return nil
}

func writeImageThumbnail(ctx context.Context, source, target string, size int) error {
	vipsErr := writeImageThumbnailWithVips(ctx, source, target, size)
	if vipsErr == nil && fsutil.FileHasContent(target) {
		return nil
	} else if vipsErr == nil {
		vipsErr = fmt.Errorf("vipsthumbnail produced no thumbnail")
	}
	_ = os.Remove(target)
	ffmpegErr := writeImageThumbnailWithFFmpeg(ctx, source, target, size)
	if ffmpegErr == nil && fsutil.FileHasContent(target) {
		return nil
	} else if ffmpegErr == nil {
		ffmpegErr = fmt.Errorf("ffmpeg produced no thumbnail")
	}
	_ = os.Remove(target)
	return fmt.Errorf("image thumbnail webp failed: %w", errors.Join(vipsErr, ffmpegErr))
}

func writeImageThumbnailWithVips(ctx context.Context, source, target string, size int) error {
	binary, err := thumbnailBackendPath("vipsthumbnail")
	if err != nil {
		return err
	}
	output := target + "[Q=" + strconv.Itoa(thumbnailWebPQuality) + ",strip]"
	return runThumbnailCommand(ctx, "vipsthumbnail", binary, source, "-s", strconv.Itoa(size)+"x"+strconv.Itoa(size)+">", "-o", output)
}

func writeImageThumbnailWithFFmpeg(ctx context.Context, source, target string, size int) error {
	binary, err := thumbnailBackendPath("ffmpeg")
	if err != nil {
		return err
	}
	return runThumbnailCommand(ctx, "ffmpeg", binary, "-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-i", source, "-frames:v", "1", "-vf", thumbnailScaleFilter(size), "-preset", "photo", "-quality", strconv.Itoa(thumbnailWebPQuality), target)
}

func extractVideoThumbnail(ctx context.Context, source, target string, size int, seek string) error {
	binary, err := thumbnailBackendPath("ffmpeg")
	if err != nil {
		return err
	}
	return runThumbnailCommand(ctx, "ffmpeg", binary, "-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-ss", seek, "-i", source, "-frames:v", "1", "-vf", thumbnailScaleFilter(size), "-preset", "photo", "-quality", strconv.Itoa(thumbnailWebPQuality), target)
}

func runThumbnailCommand(ctx context.Context, name, binary string, args ...string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "VIPS_CONCURRENCY=1")
	var output limitedThumbnailOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s thumbnail failed: %s", name, message)
	}
	return nil
}

func thumbnailScaleFilter(size int) string {
	return fmt.Sprintf("scale=w='min(iw\\,%d)':h='min(ih\\,%d)':force_original_aspect_ratio=decrease", size, size)
}

func thumbnailTempPath(target string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*.webp")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func (l *Library) cachedLargerThumbnailSource(ctx context.Context, media Media, size int) (string, bool) {
	if l == nil || !l.index.available() {
		return "", false
	}
	rows, err := l.index.db.QueryContext(ctx, `
		SELECT size
		FROM photo_thumbnail_index
		WHERE media_path = ?
			AND quality = ?
			AND status = ?
			AND source_mod_time_unix_nano = ?
			AND source_size_bytes = ?
			AND size > ?
		ORDER BY size ASC`, media.Path, thumbnailWebPQuality, thumbnailStatusGenerated, media.ModTime.UnixNano(), media.SizeBytes, NormalizeThumbnailSize(size))
	if err != nil {
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		var largerSize int
		if err := rows.Scan(&largerSize); err != nil {
			return "", false
		}
		cachePath, ready := l.thumbnailReadyCachePathForMedia(ctx, media, largerSize)
		if ready {
			return cachePath, true
		}
	}
	return "", false
}

func thumbnailBackendPath(name string) (string, error) {
	key := name + "\x00" + os.Getenv("PATH")
	if cached, ok := thumbnailBackendCache.Load(key); ok {
		result := cached.(thumbnailBackendLookup)
		return result.path, result.err
	}
	path, err := exec.LookPath(name)
	result := thumbnailBackendLookup{path: path, err: err}
	actual, _ := thumbnailBackendCache.LoadOrStore(key, result)
	result = actual.(thumbnailBackendLookup)
	return result.path, result.err
}

type thumbnailBackendLookup struct {
	path string
	err  error
}

type limitedThumbnailOutput struct {
	buf bytes.Buffer
}

func (o *limitedThumbnailOutput) Write(p []byte) (int, error) {
	const limit = 4096
	if o.buf.Len() < limit {
		remaining := limit - o.buf.Len()
		if len(p) > remaining {
			_, _ = o.buf.Write(p[:remaining])
		} else {
			_, _ = o.buf.Write(p)
		}
	}
	return len(p), nil
}

func (o *limitedThumbnailOutput) String() string {
	return o.buf.String()
}
