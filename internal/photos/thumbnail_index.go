// Datei verwaltet Thumbnail-Queue, Thumbnail-Metadaten und Indexabfragen fuer Vorschaubilder.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"bearstack/internal/sqlutil"
)

const (
	thumbnailStatusQueued    = "queued"
	thumbnailStatusGenerated = "generated"
	thumbnailStatusFailed    = "failed"

	thumbnailDefaultPriority = 100
	thumbnailVisiblePriority = 10
	thumbnailLookupBatchSize = 500
)

const thumbnailableMediaSQL = `(type = 'video'
	OR (type = 'image' AND (
		LOWER(path) LIKE '%.jpg'
		OR LOWER(path) LIKE '%.jpeg'
		OR LOWER(path) LIKE '%.jpe'
		OR LOWER(path) LIKE '%.png'
		OR LOWER(path) LIKE '%.gif'
		OR LOWER(path) LIKE '%.webp'
	)))`

func ThumbnailVisiblePriority() int {
	return thumbnailVisiblePriority
}

type thumbnailCandidate struct {
	Media         Media
	Size          int
	Status        string
	Attempts      int
	LastAttemptAt string
}

type ThumbnailReadyRequest struct {
	Path string
	Size int
}

func (l *Library) QueueThumbnailContext(ctx context.Context, rel string, size int, priority int) error {
	if l == nil || !l.index.available() {
		return nil
	}
	size = NormalizeThumbnailSize(size)
	media, err := l.MediaContext(ctx, rel)
	if err != nil {
		return err
	}
	if !mediaCanThumbnail(media) {
		return errThumbnailUnavailable
	}
	return l.index.queueThumbnailForMedia(ctx, media, size, priority)
}

func (l *Library) QueueThumbnailsContext(ctx context.Context, requests []ThumbnailReadyRequest, priority int, includeAdminOnly bool) error {
	if l == nil || !l.index.available() || len(requests) == 0 {
		return nil
	}
	normalized := make([]ThumbnailReadyRequest, 0, len(requests))
	seen := map[ThumbnailReadyRequest]struct{}{}
	for _, request := range requests {
		clean, err := CleanPath(request.Path)
		if err != nil {
			return err
		}
		kind, ok := supportedKind(clean)
		if !ok || (kind != MediaTypeImage && kind != MediaTypeVideo) || !CanThumbnail(clean) {
			return errThumbnailUnavailable
		}
		request = ThumbnailReadyRequest{Path: clean, Size: NormalizeThumbnailSize(request.Size)}
		if _, ok := seen[request]; ok {
			continue
		}
		seen[request] = struct{}{}
		normalized = append(normalized, request)
	}
	if len(normalized) == 0 {
		return nil
	}
	mediaByPath, err := l.thumbnailMediaForRequests(ctx, normalized, includeAdminOnly)
	if err != nil {
		return err
	}
	for _, request := range normalized {
		if _, ok := mediaByPath[request.Path]; ok {
			continue
		}
		media, err := l.MediaContext(ctx, request.Path)
		if err != nil {
			return err
		}
		if !includeAdminOnly && media.AdminOnly {
			return errAdminOnly
		}
		if !mediaCanThumbnail(media) {
			return errThumbnailUnavailable
		}
		mediaByPath[media.Path] = media
	}
	return l.index.queueThumbnailsForMedia(ctx, normalized, mediaByPath, priority)
}

func (s *photoIndexStore) queueThumbnailForMedia(ctx context.Context, media Media, size int, priority int) error {
	if !s.available() || !mediaCanThumbnail(media) {
		return nil
	}
	if priority <= 0 {
		priority = thumbnailDefaultPriority
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, thumbnailQueueUpsertSQL(),
		media.Path,
		NormalizeThumbnailSize(size),
		thumbnailWebPQuality,
		media.ModTime.UnixNano(),
		media.SizeBytes,
		thumbnailStatusQueued,
		priority,
		now,
		thumbnailStatusGenerated,
		thumbnailStatusQueued,
	)
	return err
}

func (s *photoIndexStore) queueThumbnailsForMedia(ctx context.Context, requests []ThumbnailReadyRequest, mediaByPath map[string]Media, priority int) error {
	if !s.available() || len(requests) == 0 {
		return nil
	}
	if priority <= 0 {
		priority = thumbnailDefaultPriority
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, thumbnailQueueUpsertSQL())
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return err
		}
		media, ok := mediaByPath[request.Path]
		if !ok || !mediaCanThumbnail(media) {
			continue
		}
		if _, err := stmt.ExecContext(ctx,
			media.Path,
			NormalizeThumbnailSize(request.Size),
			thumbnailWebPQuality,
			media.ModTime.UnixNano(),
			media.SizeBytes,
			thumbnailStatusQueued,
			priority,
			now,
			thumbnailStatusGenerated,
			thumbnailStatusQueued,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func thumbnailQueueUpsertSQL() string {
	return `
		INSERT INTO photo_thumbnail_index(media_path, size, quality, source_mod_time_unix_nano, source_size_bytes, status, attempts, last_attempt_at, generated_at, error, priority, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, '', '', '', ?, ?)
		ON CONFLICT(media_path, size) DO UPDATE SET
			quality = excluded.quality,
			source_mod_time_unix_nano = excluded.source_mod_time_unix_nano,
			source_size_bytes = excluded.source_size_bytes,
			status = CASE
				WHEN photo_thumbnail_index.status = ?
					AND photo_thumbnail_index.quality = excluded.quality
					AND photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.status
				ELSE ?
			END,
			attempts = CASE
				WHEN photo_thumbnail_index.quality = excluded.quality
					AND photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.attempts
				ELSE 0
			END,
			last_attempt_at = CASE
				WHEN photo_thumbnail_index.quality = excluded.quality
					AND photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.last_attempt_at
				ELSE ''
			END,
			generated_at = CASE
				WHEN photo_thumbnail_index.status = 'generated'
					AND photo_thumbnail_index.quality = excluded.quality
					AND photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.generated_at
				ELSE ''
			END,
			error = CASE
				WHEN photo_thumbnail_index.quality = excluded.quality
					AND photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.error
				ELSE ''
			END,
			priority = CASE
				WHEN photo_thumbnail_index.priority < excluded.priority THEN photo_thumbnail_index.priority
				ELSE excluded.priority
			END,
			requested_at = excluded.requested_at
		WHERE photo_thumbnail_index.quality <> excluded.quality
			OR photo_thumbnail_index.source_mod_time_unix_nano <> excluded.source_mod_time_unix_nano
			OR photo_thumbnail_index.source_size_bytes <> excluded.source_size_bytes
			OR photo_thumbnail_index.status NOT IN ('queued', 'generated')
			OR excluded.priority < photo_thumbnail_index.priority`
}

func (s *photoIndexStore) ensureThumbnailQueueForSizes(ctx context.Context, sizes []int, limit int) error {
	if !s.available() {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	for _, size := range sizes {
		if err := ctx.Err(); err != nil {
			return err
		}
		size = NormalizeThumbnailSize(size)
		var mediaCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index WHERE `+thumbnailableMediaSQL).Scan(&mediaCount); err != nil {
			return err
		}
		if mediaCount == 0 {
			continue
		}
		var queueCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM photo_thumbnail_index WHERE size = ? AND quality = ?`, size, thumbnailWebPQuality).Scan(&queueCount); err != nil {
			return err
		}
		if queueCount >= mediaCount {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, `
				INSERT INTO photo_thumbnail_index(media_path, size, quality, source_mod_time_unix_nano, source_size_bytes, status, attempts, last_attempt_at, generated_at, error, priority, requested_at)
				SELECT mi.path, ?, ?, mi.mod_time_unix_nano, mi.size_bytes, ?, 0, '', '', '', ?, ?
				FROM media_index mi
				LEFT JOIN photo_thumbnail_index ti ON ti.media_path = mi.path AND ti.size = ?
				WHERE ti.media_path IS NULL AND `+thumbnailableMediaSQL+`
				ORDER BY CASE mi.type WHEN 'image' THEN 0 ELSE 1 END,
					mi.captured_at DESC,
					mi.mod_time_unix_nano DESC,
					mi.path DESC
				LIMIT ?`,
			size,
			thumbnailWebPQuality,
			thumbnailStatusQueued,
			thumbnailDefaultPriority,
			now,
			size,
			limit,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *photoIndexStore) queueChangedMediaThumbnailsTx(ctx context.Context, tx *sql.Tx, items []Media, requestedAt string) error {
	if !s.available() || tx == nil || len(items) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT size FROM photo_thumbnail_index WHERE quality = ?`, thumbnailWebPQuality)
	if err != nil {
		return err
	}
	var sizes []int
	for rows.Next() {
		var size int
		if err := rows.Scan(&size); err != nil {
			_ = rows.Close()
			return err
		}
		sizes = append(sizes, size)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(sizes) == 0 {
		return nil
	}
	for _, media := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !mediaCanThumbnail(media) {
			continue
		}
		for _, size := range sizes {
			if _, err := tx.ExecContext(ctx, thumbnailQueueUpsertSQL(),
				media.Path,
				NormalizeThumbnailSize(size),
				thumbnailWebPQuality,
				media.ModTime.UnixNano(),
				media.SizeBytes,
				thumbnailStatusQueued,
				thumbnailDefaultPriority,
				requestedAt,
				thumbnailStatusGenerated,
				thumbnailStatusQueued,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *photoIndexStore) thumbnailCandidates(ctx context.Context, sizes []int, limit int) ([]thumbnailCandidate, error) {
	if !s.available() || len(sizes) == 0 || limit <= 0 {
		return nil, nil
	}
	args := make([]any, 0, len(sizes)+3)
	args = append(args, thumbnailWebPQuality)
	for _, size := range sizes {
		args = append(args, NormalizeThumbnailSize(size))
	}
	args = append(args, thumbnailStatusGenerated, thumbnailWebPQuality, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT mi.path, mi.type, mi.size_bytes, mi.mod_time_unix_nano, ti.size, ti.status, ti.attempts, ti.last_attempt_at
		FROM photo_thumbnail_index ti
		JOIN media_index mi ON mi.path = ti.media_path
		WHERE ti.quality = ?
			AND ti.size IN (`+sqlutil.Placeholders(len(sizes))+`)
			AND (ti.status <> ? OR ti.source_mod_time_unix_nano <> mi.mod_time_unix_nano OR ti.source_size_bytes <> mi.size_bytes OR ti.quality <> ?)
			AND `+thumbnailableMediaSQL+`
		ORDER BY
			CASE ti.status WHEN 'queued' THEN 0 WHEN 'failed' THEN 1 ELSE 2 END,
			ti.priority ASC,
			ti.requested_at DESC,
			CASE mi.type WHEN 'image' THEN 0 ELSE 1 END,
			ti.size DESC,
			mi.captured_at DESC,
			mi.mod_time_unix_nano DESC,
			mi.path DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]thumbnailCandidate, 0, limit)
	for rows.Next() {
		var candidate thumbnailCandidate
		var modUnix int64
		if err := rows.Scan(
			&candidate.Media.Path,
			&candidate.Media.Type,
			&candidate.Media.SizeBytes,
			&modUnix,
			&candidate.Size,
			&candidate.Status,
			&candidate.Attempts,
			&candidate.LastAttemptAt,
		); err != nil {
			return nil, err
		}
		candidate.Media.Name = pathBase(candidate.Media.Path)
		candidate.Media.Directory = parentPath(candidate.Media.Path)
		candidate.Media.ModTime = time.Unix(0, modUnix)
		if mediaCanThumbnail(candidate.Media) {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, rows.Err()
}

func (l *Library) thumbnailIndexAllowsReady(ctx context.Context, media Media, size int) bool {
	if l == nil || !l.index.available() {
		return true
	}
	return l.index.thumbnailReadyForMedia(ctx, media, size)
}

func (s *photoIndexStore) thumbnailReadyForMedia(ctx context.Context, media Media, size int) bool {
	if !s.available() {
		return true
	}
	var quality int
	var sourceMod int64
	var sourceSize int64
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT quality, source_mod_time_unix_nano, source_size_bytes, status
		FROM photo_thumbnail_index
		WHERE media_path = ? AND size = ?`, media.Path, NormalizeThumbnailSize(size)).Scan(&quality, &sourceMod, &sourceSize, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return true
	}
	if quality != thumbnailWebPQuality || sourceMod != media.ModTime.UnixNano() || sourceSize != media.SizeBytes {
		return false
	}
	return status == thumbnailStatusGenerated || status == thumbnailStatusQueued || status == thumbnailStatusFailed
}

func (l *Library) CachedThumbnailsReadyForMediaContext(ctx context.Context, items []Media, size int) map[string]bool {
	result := make(map[string]bool, len(items))
	if len(items) == 0 {
		return result
	}
	finishTrace := StartListTraceStep(ctx, "photos.thumbnails.cached_ready", ListTraceInt("items", len(items)), ListTraceInt("size", size))
	size = NormalizeThumbnailSize(size)
	mediaByPath := make(map[string]Media, len(items))
	requests := make([]ThumbnailReadyRequest, 0, len(items))
	for _, media := range items {
		if !mediaCanThumbnail(media) {
			continue
		}
		if _, seen := mediaByPath[media.Path]; seen {
			continue
		}
		mediaByPath[media.Path] = media
		requests = append(requests, ThumbnailReadyRequest{Path: media.Path, Size: size})
	}
	for request, ready := range l.thumbnailReadyBatchForMedia(ctx, requests, mediaByPath) {
		result[request.Path] = ready
	}
	readyCount := 0
	for _, ready := range result {
		if ready {
			readyCount++
		}
	}
	finishTrace(ListTraceInt("requests", len(requests)), ListTraceInt("ready", readyCount))
	return result
}

func (l *Library) ThumbnailReadyBatchContext(ctx context.Context, requests []ThumbnailReadyRequest, includeAdminOnly bool) (map[ThumbnailReadyRequest]bool, error) {
	normalized := make([]ThumbnailReadyRequest, 0, len(requests))
	seen := map[ThumbnailReadyRequest]struct{}{}
	for _, request := range requests {
		clean, err := CleanPath(request.Path)
		if err != nil {
			return nil, err
		}
		if !CanThumbnail(clean) {
			continue
		}
		kind, ok := supportedKind(clean)
		if !ok || (kind != MediaTypeImage && kind != MediaTypeVideo) {
			continue
		}
		request = ThumbnailReadyRequest{Path: clean, Size: NormalizeThumbnailSize(request.Size)}
		if _, ok := seen[request]; ok {
			continue
		}
		seen[request] = struct{}{}
		normalized = append(normalized, request)
	}
	result := make(map[ThumbnailReadyRequest]bool, len(normalized))
	if len(normalized) == 0 {
		return result, nil
	}
	mediaByPath, err := l.thumbnailMediaForRequests(ctx, normalized, includeAdminOnly)
	if err != nil {
		return nil, err
	}
	for request, ready := range l.thumbnailReadyBatchForMedia(ctx, normalized, mediaByPath) {
		result[request] = ready
	}
	return result, nil
}

func (l *Library) thumbnailMediaForRequests(ctx context.Context, requests []ThumbnailReadyRequest, includeAdminOnly bool) (map[string]Media, error) {
	mediaByPath := make(map[string]Media, len(requests))
	if l == nil {
		return mediaByPath, nil
	}
	paths := make([]string, 0, len(requests))
	seen := map[string]struct{}{}
	for _, request := range requests {
		if _, ok := seen[request.Path]; ok {
			continue
		}
		seen[request.Path] = struct{}{}
		paths = append(paths, request.Path)
	}
	if !l.index.available() {
		for _, path := range paths {
			media, err := l.MediaContext(ctx, path)
			if err != nil {
				return nil, err
			}
			if !includeAdminOnly && media.AdminOnly {
				return nil, errAdminOnly
			}
			if mediaCanThumbnail(media) {
				mediaByPath[path] = media
			}
		}
		return mediaByPath, nil
	}
	return l.index.thumbnailMediaForPaths(ctx, paths, includeAdminOnly)
}

func (s *photoIndexStore) thumbnailMediaForPaths(ctx context.Context, paths []string, includeAdminOnly bool) (map[string]Media, error) {
	mediaByPath := make(map[string]Media, len(paths))
	if !s.available() || len(paths) == 0 {
		return mediaByPath, nil
	}
	for start := 0; start < len(paths); start += thumbnailLookupBatchSize {
		end := start + thumbnailLookupBatchSize
		if end > len(paths) {
			end = len(paths)
		}
		args := make([]any, end-start)
		for i, path := range paths[start:end] {
			args[i] = path
		}
		rows, err := s.db.QueryContext(ctx, `
				SELECT path, name, directory, type, size_bytes, mod_time_unix_nano, admin_only
				FROM media_index
				WHERE path IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var media Media
			var modUnix int64
			var adminOnly int
			if err := rows.Scan(&media.Path, &media.Name, &media.Directory, &media.Type, &media.SizeBytes, &modUnix, &adminOnly); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if !includeAdminOnly && adminOnly != 0 {
				_ = rows.Close()
				return nil, errAdminOnly
			}
			media.ModTime = time.Unix(0, modUnix)
			media.AdminOnly = adminOnly != 0
			if mediaCanThumbnail(media) {
				mediaByPath[media.Path] = media
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return mediaByPath, nil
}

func (l *Library) thumbnailReadyBatchForMedia(ctx context.Context, requests []ThumbnailReadyRequest, mediaByPath map[string]Media) map[ThumbnailReadyRequest]bool {
	result := make(map[ThumbnailReadyRequest]bool, len(requests))
	if l == nil || len(requests) == 0 {
		return result
	}
	requestsBySize := map[int][]ThumbnailReadyRequest{}
	for _, request := range requests {
		if _, ok := mediaByPath[request.Path]; !ok {
			continue
		}
		request.Size = NormalizeThumbnailSize(request.Size)
		requestsBySize[request.Size] = append(requestsBySize[request.Size], request)
	}
	for size, group := range requestsBySize {
		paths := make([]string, 0, len(group))
		seenPath := map[string]struct{}{}
		for _, request := range group {
			if _, ok := seenPath[request.Path]; ok {
				continue
			}
			seenPath[request.Path] = struct{}{}
			paths = append(paths, request.Path)
		}
		finishMetadata := StartListTraceStep(ctx, "photos.thumbnails.metadata", ListTraceInt("size", size), ListTraceInt("paths", len(paths)))
		metadata := l.index.thumbnailMetadataForPaths(ctx, paths, size)
		finishMetadata(ListTraceInt("indexed", len(metadata)))
		fileChecks := 0
		finishFileChecks := StartListTraceStep(ctx, "photos.thumbnails.file_checks", ListTraceInt("size", size))
		for _, request := range group {
			media := mediaByPath[request.Path]
			state, ok := metadata[request.Path]
			if ok && (state.quality != thumbnailWebPQuality || state.sourceMod != media.ModTime.UnixNano() || state.sourceSize != media.SizeBytes) {
				continue
			}
			if ok && state.status == thumbnailStatusGenerated {
				// For consistent generated entries we can trust index metadata and skip
				// per-item filesystem stats on listing/status hot paths.
				result[request] = true
				continue
			}
			if ok && state.status != thumbnailStatusGenerated && state.status != thumbnailStatusQueued && state.status != thumbnailStatusFailed {
				continue
			}
			fileChecks++
			result[request] = l.thumbnailFileReadyForSource(media.Path, size, media.ModTime)
		}
		finishFileChecks(ListTraceInt("paths", fileChecks))
	}
	return result
}

type thumbnailMetadata struct {
	quality    int
	sourceMod  int64
	sourceSize int64
	status     string
}

func (s *photoIndexStore) thumbnailMetadataForPaths(ctx context.Context, paths []string, size int) map[string]thumbnailMetadata {
	result := make(map[string]thumbnailMetadata, len(paths))
	if !s.available() || len(paths) == 0 {
		return result
	}
	size = NormalizeThumbnailSize(size)
	for start := 0; start < len(paths); start += thumbnailLookupBatchSize {
		end := start + thumbnailLookupBatchSize
		if end > len(paths) {
			end = len(paths)
		}
		args := make([]any, 0, end-start+1)
		args = append(args, size)
		for _, path := range paths[start:end] {
			args = append(args, path)
		}
		rows, err := s.db.QueryContext(ctx, `
				SELECT media_path, quality, source_mod_time_unix_nano, source_size_bytes, status
				FROM photo_thumbnail_index
				WHERE size = ? AND media_path IN (`+sqlutil.Placeholders(end-start)+`)`, args...)
		if err != nil {
			return result
		}
		for rows.Next() {
			var path string
			var metadata thumbnailMetadata
			if err := rows.Scan(&path, &metadata.quality, &metadata.sourceMod, &metadata.sourceSize, &metadata.status); err != nil {
				_ = rows.Close()
				return result
			}
			result[path] = metadata
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return result
		}
		if err := rows.Close(); err != nil {
			return result
		}
	}
	return result
}

func (l *Library) markThumbnailGenerated(ctx context.Context, media Media, size int) error {
	if l == nil {
		return nil
	}
	return l.index.markThumbnailGenerated(ctx, media, size)
}

func (s *photoIndexStore) markThumbnailGenerated(ctx context.Context, media Media, size int) error {
	if !s.available() {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO photo_thumbnail_index(media_path, size, quality, source_mod_time_unix_nano, source_size_bytes, status, attempts, last_attempt_at, generated_at, error, priority, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, '', ?, '', ?, ?)
		ON CONFLICT(media_path, size) DO UPDATE SET
			quality = excluded.quality,
			source_mod_time_unix_nano = excluded.source_mod_time_unix_nano,
			source_size_bytes = excluded.source_size_bytes,
			status = excluded.status,
			attempts = 0,
			last_attempt_at = '',
			generated_at = excluded.generated_at,
			error = '',
			priority = ?,
			requested_at = excluded.requested_at
		WHERE photo_thumbnail_index.quality <> excluded.quality
			OR photo_thumbnail_index.source_mod_time_unix_nano <> excluded.source_mod_time_unix_nano
			OR photo_thumbnail_index.source_size_bytes <> excluded.source_size_bytes
			OR photo_thumbnail_index.status <> excluded.status
			OR photo_thumbnail_index.attempts <> 0
			OR photo_thumbnail_index.error <> ''`,
		media.Path,
		NormalizeThumbnailSize(size),
		thumbnailWebPQuality,
		media.ModTime.UnixNano(),
		media.SizeBytes,
		thumbnailStatusGenerated,
		now,
		thumbnailDefaultPriority,
		now,
		thumbnailDefaultPriority,
	)
	return err
}

func (l *Library) markThumbnailFailed(ctx context.Context, media Media, size int, cause error) error {
	if l == nil {
		return nil
	}
	return l.index.markThumbnailFailed(ctx, media, size, cause)
}

func (s *photoIndexStore) markThumbnailFailed(ctx context.Context, media Media, size int, cause error) error {
	if !s.available() {
		return nil
	}
	message := strings.TrimSpace(fmt.Sprint(cause))
	if len(message) > 500 {
		message = message[:500]
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO photo_thumbnail_index(media_path, size, quality, source_mod_time_unix_nano, source_size_bytes, status, attempts, last_attempt_at, generated_at, error, priority, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, '', ?, ?, ?)
		ON CONFLICT(media_path, size) DO UPDATE SET
			quality = excluded.quality,
			source_mod_time_unix_nano = excluded.source_mod_time_unix_nano,
			source_size_bytes = excluded.source_size_bytes,
			status = excluded.status,
			attempts = CASE
				WHEN photo_thumbnail_index.source_mod_time_unix_nano = excluded.source_mod_time_unix_nano
					AND photo_thumbnail_index.source_size_bytes = excluded.source_size_bytes
				THEN photo_thumbnail_index.attempts + 1
				ELSE 1
			END,
			last_attempt_at = excluded.last_attempt_at,
			generated_at = '',
			error = excluded.error,
			priority = photo_thumbnail_index.priority,
			requested_at = excluded.requested_at`,
		media.Path,
		NormalizeThumbnailSize(size),
		thumbnailWebPQuality,
		media.ModTime.UnixNano(),
		media.SizeBytes,
		thumbnailStatusFailed,
		now,
		message,
		thumbnailDefaultPriority,
		now,
	)
	return err
}

func thumbnailFailureBackoffActive(attempts int, lastAttemptAt string, now time.Time) bool {
	if attempts <= 0 || strings.TrimSpace(lastAttemptAt) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, lastAttemptAt)
	if err != nil {
		return false
	}
	delay := 15 * time.Minute
	switch {
	case attempts >= 4:
		delay = 24 * time.Hour
	case attempts == 3:
		delay = 6 * time.Hour
	case attempts == 2:
		delay = time.Hour
	}
	return now.Before(parsed.Add(delay))
}

func thumbnailFileReadyForSource(path string, sourceMod time.Time) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 {
		return false
	}
	if !sourceMod.IsZero() && info.ModTime().Before(sourceMod) {
		return false
	}
	return true
}

func mediaCanThumbnail(media Media) bool {
	return (media.Type == MediaTypeImage || media.Type == MediaTypeVideo) && CanThumbnail(media.Path)
}

func pathBase(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}
