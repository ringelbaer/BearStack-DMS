// Datei enthaelt Store-Operationen fuer Rebuild-Caches, Scan-Staende, Pruning und Indexstatistiken.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"bearstack/internal/sqlutil"
)

func (s *photoIndexStore) loadIndexRunCache(ctx context.Context) (indexRunCache, error) {
	stats, err := s.cachedIndexStats(ctx)
	if err != nil {
		return indexRunCache{}, err
	}
	rootScanSignature, rootQuickSignature, hasRootScan, err := s.loadRootFolderScan(ctx)
	if err != nil {
		return indexRunCache{}, err
	}
	states, children, err := s.loadIndexedFolderRunCache(ctx, stats.Folders)
	if err != nil {
		return indexRunCache{}, err
	}
	return indexRunCache{
		rootScanSignature:  rootScanSignature,
		rootQuickSignature: rootQuickSignature,
		hasRootScan:        hasRootScan,
		folderStates:       states,
		folderChildren:     children,
		stats:              stats,
		hasIndexedContent:  stats.Media > 0 || stats.Folders > 0 || stats.Blogs > 0,
	}, nil
}

func (s *photoIndexStore) loadRootFolderScan(ctx context.Context) (int64, int64, bool, error) {
	if !s.available() {
		return 0, 0, false, nil
	}
	var scanSignature, quickSignature int64
	err := s.db.QueryRowContext(ctx, `SELECT mod_time_unix_nano, quick_signature_unix_nano FROM photo_folder_scan WHERE path = ''`).Scan(&scanSignature, &quickSignature)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return scanSignature, quickSignature, true, nil
}

func (s *photoIndexStore) loadIndexedFolderRunCache(ctx context.Context, folderCount int) (map[string]indexedFolderState, map[string][]indexQueueItem, error) {
	if folderCount == 0 {
		return nil, nil, nil
	}
	if !s.available() {
		return nil, nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.parent, fi.path, fi.dir_count, fi.admin_only, COALESCE(pfs.mod_time_unix_nano, -1), COALESCE(pfs.quick_signature_unix_nano, 0)
		FROM folder_index fi
		LEFT JOIN photo_folder_scan pfs ON pfs.path = fi.path
		ORDER BY fi.parent, fi.name COLLATE NOCASE`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	states := make(map[string]indexedFolderState, folderCount)
	children := make(map[string][]indexQueueItem)
	for rows.Next() {
		var parent, path string
		var dirCount int
		var adminOnly int
		var scanSignature, quickSignature int64
		if err := rows.Scan(&parent, &path, &dirCount, &adminOnly, &scanSignature, &quickSignature); err != nil {
			return nil, nil, err
		}
		if path != "" {
			states[path] = indexedFolderState{ScanSignature: scanSignature, QuickSignature: quickSignature, DirCount: dirCount, AdminOnly: adminOnly != 0}
			children[parent] = append(children[parent], indexQueueItem{
				Path:            path,
				IndexedDirCount: dirCount,
			})
		}
	}
	return states, children, rows.Err()
}

func (s *photoIndexStore) directoryMediaCache(ctx context.Context, rel string) (map[string]cachedMediaRow, error) {
	if !s.available() {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+mediaIndexColumns(``)+` FROM media_index WHERE directory = ?`, rel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := make(map[string]cachedMediaRow)
	for rows.Next() {
		var row cachedMediaRow
		if err := rows.Scan(
			&row.Path,
			&row.Name,
			&row.Directory,
			&row.Type,
			&row.MIMEType,
			&row.SizeBytes,
			&row.ModTimeUnixNano,
			&row.CapturedAt,
			&row.Width,
			&row.Height,
			&row.Orientation,
			&row.Camera,
			&row.Lens,
			&row.Rating,
			&row.Latitude,
			&row.Longitude,
			&row.Keywords,
			&row.Tags,
			&row.Faces,
			&row.XMPFingerprint,
			&row.AdminOnly,
		); err != nil {
			return nil, err
		}
		cache[row.Path] = row
	}
	return cache, rows.Err()
}

func (s *photoIndexStore) directoryBlogCache(ctx context.Context, rel string) (map[string]cachedBlogRow, error) {
	if !s.available() {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path, name, directory, date, mod_time_unix_nano, text, tags, admin_only FROM blog_index WHERE directory = ?`, rel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := make(map[string]cachedBlogRow)
	for rows.Next() {
		var row cachedBlogRow
		if err := rows.Scan(
			&row.Path,
			&row.Name,
			&row.Directory,
			&row.Date,
			&row.ModTimeUnixNano,
			&row.Text,
			&row.Tags,
			&row.AdminOnly,
		); err != nil {
			return nil, err
		}
		cache[row.Path] = row
	}
	return cache, rows.Err()
}

func (s *photoIndexStore) saveScannedFolder(ctx context.Context, rel string, modTime time.Time, mediaCount, blogCount, dirCount int, adminOnly bool, orderMode string) error {
	if !s.available() || rel == "" {
		return nil
	}
	tags, _ := s.folderTags(rel)
	publicMediaCount := mediaCount
	publicBlogCount := blogCount
	if adminOnly {
		publicMediaCount = 0
		publicBlogCount = 0
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO folder_index(path, parent, name, media_count, public_media_count, recursive_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count, dir_count, mod_time_unix_nano, order_mode, tags, admin_only, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			parent = excluded.parent,
			name = excluded.name,
			media_count = excluded.media_count,
			public_media_count = excluded.public_media_count,
			recursive_media_count = excluded.recursive_media_count,
			public_recursive_media_count = excluded.public_recursive_media_count,
			recursive_blog_count = excluded.recursive_blog_count,
			public_recursive_blog_count = excluded.public_recursive_blog_count,
			dir_count = excluded.dir_count,
			mod_time_unix_nano = excluded.mod_time_unix_nano,
			order_mode = excluded.order_mode,
			admin_only = excluded.admin_only,
			indexed_at = excluded.indexed_at
		WHERE folder_index.parent <> excluded.parent
			OR folder_index.name <> excluded.name
			OR folder_index.media_count <> excluded.media_count
			OR folder_index.public_media_count <> excluded.public_media_count
			OR folder_index.recursive_media_count <> excluded.recursive_media_count
			OR folder_index.dir_count <> excluded.dir_count
			OR folder_index.mod_time_unix_nano <> excluded.mod_time_unix_nano
			OR folder_index.order_mode <> excluded.order_mode
			OR folder_index.admin_only <> excluded.admin_only
			OR folder_index.public_recursive_media_count <> excluded.public_recursive_media_count
			OR folder_index.recursive_blog_count <> excluded.recursive_blog_count
			OR folder_index.public_recursive_blog_count <> excluded.public_recursive_blog_count`,
		rel,
		parentPath(rel),
		filepath.Base(filepath.FromSlash(rel)),
		mediaCount,
		publicMediaCount,
		mediaCount,
		publicMediaCount,
		blogCount,
		publicBlogCount,
		dirCount,
		modTime.UnixNano(),
		orderMode,
		tagsJSONString(tags),
		boolInt(adminOnly),
		now,
	)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return nil
	}
	if len(tags) > 0 {
		s.syncFolderTags(rel, tags)
	}
	return s.refreshFolderSearch(ctx, rel)
}

func (s *photoIndexStore) saveFolderScan(ctx context.Context, rel string, scanSignature, quickSignature int64, orderMode string) error {
	if !s.available() {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO photo_folder_scan(path, mod_time_unix_nano, quick_signature_unix_nano, order_mode, scanned_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			mod_time_unix_nano = excluded.mod_time_unix_nano,
			quick_signature_unix_nano = excluded.quick_signature_unix_nano,
			order_mode = excluded.order_mode,
			scanned_at = excluded.scanned_at
		WHERE photo_folder_scan.mod_time_unix_nano <> excluded.mod_time_unix_nano
			OR photo_folder_scan.quick_signature_unix_nano <> excluded.quick_signature_unix_nano
			OR photo_folder_scan.order_mode <> excluded.order_mode`,
		rel,
		scanSignature,
		quickSignature,
		orderMode,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *photoIndexStore) pruneDirectoryMedia(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if !s.available() {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM media_index WHERE directory = ?`, rel)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	stale := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if _, ok := seen[path]; !ok {
			stale = append(stale, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return len(stale), s.deleteMediaIndexPaths(ctx, stale)
}

func (s *photoIndexStore) pruneDirectoryMediaCache(ctx context.Context, seen map[string]struct{}, cache map[string]cachedMediaRow) (int, error) {
	if !s.available() {
		return 0, nil
	}
	stale := make([]string, 0)
	for path := range cache {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if _, ok := seen[path]; ok {
			continue
		}
		stale = append(stale, path)
	}
	return len(stale), s.deleteMediaIndexPaths(ctx, stale)
}

func (s *photoIndexStore) pruneDirectoryBlogs(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if !s.available() {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM blog_index WHERE directory = ?`, rel)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	stale := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if _, ok := seen[path]; !ok {
			stale = append(stale, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return len(stale), s.deleteBlogIndexPaths(ctx, stale)
}

func (s *photoIndexStore) pruneDirectoryBlogCache(ctx context.Context, seen map[string]struct{}, cache map[string]cachedBlogRow) (int, error) {
	if !s.available() {
		return 0, nil
	}
	stale := make([]string, 0)
	for path := range cache {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if _, ok := seen[path]; ok {
			continue
		}
		stale = append(stale, path)
	}
	return len(stale), s.deleteBlogIndexPaths(ctx, stale)
}

func (s *photoIndexStore) pruneDirectoryFolders(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if !s.available() {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM folder_index WHERE parent = ?`, rel)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	stale := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if _, ok := seen[path]; !ok {
			stale = append(stale, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, path := range stale {
		if err := s.deleteFolderIndexSubtree(ctx, path); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

func (s *photoIndexStore) deleteMediaIndexPaths(ctx context.Context, paths []string) error {
	return s.deleteIndexPaths(ctx, paths, []string{
		`DELETE FROM media_search WHERE path IN (%s)`,
		`DELETE FROM media_tag_index WHERE media_path IN (%s)`,
		`DELETE FROM folder_preview_index WHERE media_path IN (%s)`,
		`DELETE FROM photo_thumbnail_index WHERE media_path IN (%s)`,
		`DELETE FROM media_index WHERE path IN (%s)`,
	})
}

func (s *photoIndexStore) deleteBlogIndexPaths(ctx context.Context, paths []string) error {
	return s.deleteIndexPaths(ctx, paths, []string{
		`DELETE FROM blog_search WHERE path IN (%s)`,
		`DELETE FROM blog_tag_index WHERE blog_path IN (%s)`,
		`DELETE FROM blog_index WHERE path IN (%s)`,
	})
}

func (s *photoIndexStore) deleteIndexPaths(ctx context.Context, paths []string, statements []string) error {
	if !s.available() || len(paths) == 0 {
		return nil
	}
	for start := 0; start < len(paths); start += searchWriteChunkSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + searchWriteChunkSize
		if end > len(paths) {
			end = len(paths)
		}
		args := make([]any, 0, end-start)
		for _, path := range paths[start:end] {
			args = append(args, path)
		}
		in := sqlutil.Placeholders(len(args))
		for _, stmt := range statements {
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf(stmt, in), args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *photoIndexStore) deleteFolderIndexSubtree(ctx context.Context, rel string) error {
	if !s.available() {
		return nil
	}
	start, end := prefixRange(rel + "/")
	mediaArgs := []any{rel, start, end}
	blogArgs := []any{rel, start, end}
	folderArgs := []any{rel, start, end}
	statements := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM media_search WHERE path IN (SELECT path FROM media_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, mediaArgs},
		{`DELETE FROM media_tag_index WHERE media_path IN (SELECT path FROM media_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, mediaArgs},
		{`DELETE FROM folder_preview_index WHERE media_path IN (SELECT path FROM media_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, mediaArgs},
		{`DELETE FROM photo_thumbnail_index WHERE media_path IN (SELECT path FROM media_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, mediaArgs},
		{`DELETE FROM media_index WHERE directory = ? OR (directory >= ? AND directory < ?)`, mediaArgs},
		{`DELETE FROM blog_search WHERE path IN (SELECT path FROM blog_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, blogArgs},
		{`DELETE FROM blog_tag_index WHERE blog_path IN (SELECT path FROM blog_index WHERE directory = ? OR (directory >= ? AND directory < ?))`, blogArgs},
		{`DELETE FROM blog_index WHERE directory = ? OR (directory >= ? AND directory < ?)`, blogArgs},
		{`DELETE FROM folder_search WHERE path = ? OR (path >= ? AND path < ?)`, folderArgs},
		{`DELETE FROM folder_tag_index WHERE folder_path = ? OR (folder_path >= ? AND folder_path < ?)`, folderArgs},
		{`DELETE FROM folder_preview_index WHERE folder_path = ? OR (folder_path >= ? AND folder_path < ?)`, folderArgs},
		{`DELETE FROM folder_index WHERE path = ? OR (path >= ? AND path < ?)`, folderArgs},
		{`DELETE FROM photo_folder_scan WHERE path = ? OR (path >= ? AND path < ?)`, folderArgs},
	}
	for _, stmt := range statements {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *photoIndexStore) refreshPhotoStats(ctx context.Context) error {
	if !s.available() {
		return nil
	}
	var mediaCount, gpsCount, folderCount, blogCount, rootMediaCount, rootPublicMediaCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index`).Scan(&mediaCount); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index WHERE latitude IS NOT NULL AND longitude IS NOT NULL`).Scan(&gpsCount); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM folder_index`).Scan(&folderCount); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blog_index`).Scan(&blogCount); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index WHERE directory = ''`).Scan(&rootMediaCount); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index WHERE directory = '' AND admin_only = 0`).Scan(&rootPublicMediaCount); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO photo_stats(key, value) VALUES
			('media_count', ?),
			('gps_media_count', ?),
			('folder_count', ?),
			('blog_count', ?),
			('root_media_count', ?),
			('root_public_media_count', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value WHERE photo_stats.value <> excluded.value`,
		mediaCount,
		gpsCount,
		folderCount,
		blogCount,
		rootMediaCount,
		rootPublicMediaCount,
	)
	return err
}

func (s *photoIndexStore) cachedIndexStats(ctx context.Context) (IndexStats, error) {
	if !s.available() {
		return IndexStats{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM photo_stats WHERE key IN ('media_count', 'folder_count', 'blog_count')`)
	if err != nil {
		return s.indexStats(ctx)
	}
	defer rows.Close()
	values := make(map[string]int, 3)
	for rows.Next() {
		var key string
		var value int
		if err := rows.Scan(&key, &value); err != nil {
			return IndexStats{}, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return IndexStats{}, err
	}
	media, mediaOK := values["media_count"]
	folders, foldersOK := values["folder_count"]
	blogs, blogsOK := values["blog_count"]
	if !mediaOK || !foldersOK || !blogsOK {
		return s.indexStats(ctx)
	}
	return IndexStats{Media: media, Folders: folders, Blogs: blogs}, nil
}

func (s *photoIndexStore) indexStats(ctx context.Context) (IndexStats, error) {
	if !s.available() {
		return IndexStats{}, nil
	}
	var stats IndexStats
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM media_index),
			(SELECT COUNT(*) FROM folder_index),
			(SELECT COUNT(*) FROM blog_index)`,
	).Scan(&stats.Media, &stats.Folders, &stats.Blogs); err != nil {
		return IndexStats{}, err
	}
	return stats, nil
}
