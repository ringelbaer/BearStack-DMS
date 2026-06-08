// Datei berechnet und liefert Statistiken ueber Medien, Ordner und Fotoindex-Zustand.
package photos

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Statistics struct {
	Index                    IndexStats
	IndexAvailable           bool
	MediaBytes               int64
	AverageMediaBytes        int64
	ImageCount               int
	VideoCount               int
	AudioCount               int
	GPSMediaCount            int
	GPSCoveragePercent       int
	MediaWithDimensions      int
	DimensionCoveragePercent int
	MediaWithCamera          int
	CameraCoveragePercent    int
	MediaWithLens            int
	TaggedMediaCount         int
	MediaTagAssignments      int
	FolderTagAssignments     int
	BlogTagAssignments       int
	PhotoTagCount            int
	DistinctCameraCount      int
	PortraitCount            int
	LandscapeCount           int
	SquareCount              int
	LastIndexedAt            time.Time
	ThumbnailCacheFiles      int
	ThumbnailCacheBytes      int64
	IndexDatabaseBytes       int64
	RootPath                 string
	CachePath                string
	IndexDatabasePath        string
	ThumbnailBackends        []ThumbnailBackendStatus
}

type ThumbnailBackendStatus struct {
	Name      string
	Purpose   string
	Path      string
	Version   string
	Available bool
}

const (
	thumbnailBackendStatusCacheTTL = 5 * time.Minute
	thumbnailCacheStatsCacheTTL    = 30 * time.Second
)

type thumbnailCacheStatsEntry struct {
	root      string
	files     int
	bytes     int64
	expiresAt time.Time
}

var thumbnailBackendStatusCache struct {
	mu        sync.Mutex
	expiresAt time.Time
	statuses  []ThumbnailBackendStatus
}

func (l *Library) Statistics(ctx context.Context) (Statistics, error) {
	if l == nil {
		return Statistics{}, nil
	}
	stats := Statistics{
		RootPath:          l.Root(),
		CachePath:         l.CacheDir(),
		IndexDatabasePath: l.DBPath(),
		ThumbnailBackends: thumbnailBackendStatuses(ctx),
	}
	cacheFiles, cacheBytes, err := l.cachedThumbnailCacheStats(filepath.Join(l.CacheDir(), "thumbnails"))
	if err != nil {
		return Statistics{}, err
	}
	stats.ThumbnailCacheFiles = cacheFiles
	stats.ThumbnailCacheBytes = cacheBytes
	stats.IndexDatabaseBytes = sqliteDatabaseBytes(l.DBPath())

	if !l.index.available() {
		return stats, nil
	}
	stats.IndexAvailable = true

	indexStats, err := l.cachedIndexStats(ctx)
	if err != nil {
		return Statistics{}, err
	}
	stats.Index = indexStats
	if err := l.loadMediaStatistics(ctx, &stats); err != nil {
		return Statistics{}, err
	}
	if err := l.loadTagStatistics(ctx, &stats); err != nil {
		return Statistics{}, err
	}
	if err := l.loadLastIndexedAt(ctx, &stats); err != nil {
		return Statistics{}, err
	}
	return stats, nil
}

func (l *Library) InvalidateStatisticsCache() {
	if l == nil {
		return
	}
	l.statsMu.Lock()
	l.statsCache = thumbnailCacheStatsEntry{}
	l.statsMu.Unlock()
}

func (l *Library) loadMediaStatistics(ctx context.Context, stats *Statistics) error {
	if err := l.index.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(size_bytes), 0),
			COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN latitude IS NOT NULL AND longitude IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN width > 0 AND height > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN trim(camera) != '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN trim(lens) != '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN tags != '' AND tags != '[]' THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT NULLIF(trim(camera), '')),
			COALESCE(SUM(CASE WHEN orientation = 'portrait' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN orientation = 'landscape' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN orientation = 'square' THEN 1 ELSE 0 END), 0)
		FROM media_index`,
		MediaTypeImage,
		MediaTypeVideo,
		MediaTypeAudio,
	).Scan(
		&stats.MediaBytes,
		&stats.ImageCount,
		&stats.VideoCount,
		&stats.AudioCount,
		&stats.GPSMediaCount,
		&stats.MediaWithDimensions,
		&stats.MediaWithCamera,
		&stats.MediaWithLens,
		&stats.TaggedMediaCount,
		&stats.DistinctCameraCount,
		&stats.PortraitCount,
		&stats.LandscapeCount,
		&stats.SquareCount,
	); err != nil {
		return err
	}
	if stats.Index.Media > 0 {
		stats.AverageMediaBytes = stats.MediaBytes / int64(stats.Index.Media)
	}
	stats.GPSCoveragePercent = photoPercent(stats.GPSMediaCount, stats.Index.Media)
	stats.DimensionCoveragePercent = photoPercent(stats.MediaWithDimensions, stats.Index.Media)
	stats.CameraCoveragePercent = photoPercent(stats.MediaWithCamera, stats.Index.Media)
	return nil
}

func (l *Library) loadTagStatistics(ctx context.Context, stats *Statistics) error {
	return l.index.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM media_tag_index),
			(SELECT COUNT(*) FROM folder_tag_index),
			(SELECT COUNT(*) FROM blog_tag_index),
			(SELECT COUNT(*) FROM photo_tags)`,
	).Scan(
		&stats.MediaTagAssignments,
		&stats.FolderTagAssignments,
		&stats.BlogTagAssignments,
		&stats.PhotoTagCount,
	)
}

func (l *Library) loadLastIndexedAt(ctx context.Context, stats *Statistics) error {
	var value sql.NullString
	if err := l.index.db.QueryRowContext(ctx, `
		SELECT MAX(indexed_at)
		FROM (
			SELECT indexed_at FROM media_index
			UNION ALL SELECT indexed_at FROM folder_index
			UNION ALL SELECT indexed_at FROM blog_index
		)`).Scan(&value); err != nil {
		return err
	}
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	stats.LastIndexedAt = parsed
	return nil
}

func thumbnailCacheStats(root string) (int, int64, error) {
	if root == "" {
		return 0, 0, nil
	}
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, nil
	}
	var files int
	var bytes int64
	err = walkFilesystemFiles(context.Background(), root, func(entry os.DirEntry) error {
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}

func (l *Library) cachedThumbnailCacheStats(root string) (int, int64, error) {
	if l == nil {
		return thumbnailCacheStats(root)
	}
	now := time.Now()
	l.statsMu.Lock()
	entry := l.statsCache
	if entry.root == root && now.Before(entry.expiresAt) {
		l.statsMu.Unlock()
		return entry.files, entry.bytes, nil
	}
	l.statsMu.Unlock()

	files, bytes, err := thumbnailCacheStats(root)
	if err != nil {
		return 0, 0, err
	}

	l.statsMu.Lock()
	l.statsCache = thumbnailCacheStatsEntry{
		root:      root,
		files:     files,
		bytes:     bytes,
		expiresAt: time.Now().Add(thumbnailCacheStatsCacheTTL),
	}
	l.statsMu.Unlock()
	return files, bytes, nil
}

func sqliteDatabaseBytes(path string) int64 {
	if path == "" {
		return 0
	}
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(path + suffix); err == nil {
			total += info.Size()
		}
	}
	return total
}

func thumbnailBackendStatuses(ctx context.Context) []ThumbnailBackendStatus {
	now := time.Now()
	thumbnailBackendStatusCache.mu.Lock()
	if now.Before(thumbnailBackendStatusCache.expiresAt) {
		statuses := cloneThumbnailBackendStatuses(thumbnailBackendStatusCache.statuses)
		thumbnailBackendStatusCache.mu.Unlock()
		return statuses
	}
	thumbnailBackendStatusCache.mu.Unlock()

	backends := probeThumbnailBackendStatuses(ctx)

	thumbnailBackendStatusCache.mu.Lock()
	thumbnailBackendStatusCache.statuses = cloneThumbnailBackendStatuses(backends)
	thumbnailBackendStatusCache.expiresAt = now.Add(thumbnailBackendStatusCacheTTL)
	thumbnailBackendStatusCache.mu.Unlock()
	return backends
}

func probeThumbnailBackendStatuses(ctx context.Context) []ThumbnailBackendStatus {
	backends := []ThumbnailBackendStatus{
		{Name: "vipsthumbnail", Purpose: "Bilder, primär"},
		{Name: "ffmpeg", Purpose: "Video-Frames und Bild-Fallback"},
	}
	for i := range backends {
		path, err := exec.LookPath(backends[i].Name)
		if err != nil {
			continue
		}
		backends[i].Available = true
		backends[i].Path = path
		backends[i].Version = thumbnailBackendVersion(ctx, backends[i].Name)
	}
	return backends
}

func cloneThumbnailBackendStatuses(statuses []ThumbnailBackendStatus) []ThumbnailBackendStatus {
	if len(statuses) == 0 {
		return nil
	}
	return append([]ThumbnailBackendStatus(nil), statuses...)
}

func thumbnailBackendVersion(parent context.Context, name string) string {
	ctx, cancel := context.WithTimeout(parent, 700*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}

func photoPercent(part, total int) int {
	if total <= 0 || part <= 0 {
		return 0
	}
	return (part*100 + total/2) / total
}
