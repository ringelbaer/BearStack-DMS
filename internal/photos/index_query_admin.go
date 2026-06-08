// Datei enthaelt AdminOnly- und Sichtbarkeitsabfragen aus dem Fotoindex.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (l *Library) indexFolderAdminOnly(ctx context.Context, rel string) (bool, bool, error) {
	if l == nil || !l.index.available() || rel == "" {
		return false, false, nil
	}
	var adminOnly int
	err := l.index.db.QueryRowContext(ctx, `SELECT admin_only FROM folder_index WHERE path = ?`, rel).Scan(&adminOnly)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return adminOnly != 0, true, nil
}

func (l *Library) filterEmptyAdminOnlyIndexedContainerFolders(ctx context.Context, folders []Folder) ([]Folder, error) {
	if l == nil || !l.index.available() || len(folders) == 0 {
		return folders, nil
	}
	candidates := make([]Folder, 0)
	for _, folder := range folders {
		if folder.MediaCount == 0 {
			candidates = append(candidates, folder)
		}
	}
	if len(candidates) == 0 {
		return folders, nil
	}
	hidden, err := l.emptyAdminOnlyIndexedContainerPaths(ctx, candidates)
	if err != nil {
		return nil, err
	}
	if len(hidden) == 0 {
		return folders, nil
	}
	filtered := folders[:0]
	for _, folder := range folders {
		if hidden[folder.Path] {
			continue
		}
		filtered = append(filtered, folder)
	}
	return filtered, nil
}

func (l *Library) emptyAdminOnlyIndexedContainerPaths(ctx context.Context, folders []Folder) (map[string]bool, error) {
	hidden := make(map[string]bool)
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3)
		for _, folder := range folders[start:end] {
			prefixStart, prefixEnd := prefixRange(folder.Path + "/")
			values = append(values, "(?, ?, ?)")
			args = append(args, folder.Path, prefixStart, prefixEnd)
		}
		rows, err := l.index.db.QueryContext(ctx, `
			WITH requested(path, prefix_start, prefix_end) AS (
				VALUES `+strings.Join(values, ",")+`
			)
			SELECT requested.path
			FROM requested
			WHERE (
				EXISTS (
					SELECT 1
					FROM folder_index fi
					WHERE fi.admin_only = 1
						AND fi.path >= requested.prefix_start
						AND fi.path < requested.prefix_end
					LIMIT 1
				)
				OR EXISTS (
					SELECT 1
					FROM media_index mi
					WHERE mi.admin_only = 1
						AND (mi.directory = requested.path OR (mi.directory >= requested.prefix_start AND mi.directory < requested.prefix_end))
					LIMIT 1
				)
				OR EXISTS (
					SELECT 1
					FROM blog_index bi
					WHERE bi.admin_only = 1
						AND (bi.directory = requested.path OR (bi.directory >= requested.prefix_start AND bi.directory < requested.prefix_end))
					LIMIT 1
				)
			)
			AND NOT EXISTS (
				SELECT 1
				FROM blog_index bi
				WHERE bi.admin_only = 0
					AND (bi.directory = requested.path OR (bi.directory >= requested.prefix_start AND bi.directory < requested.prefix_end))
				LIMIT 1
			)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				_ = rows.Close()
				return nil, err
			}
			hidden[path] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return hidden, nil
}

func (l *Library) applyVisibleRecursiveMediaCounts(ctx context.Context, folders []Folder) error {
	if len(folders) == 0 {
		return nil
	}
	finishHasAdminOnly := StartListTraceStep(ctx, "photos.index.admin_only_media_check")
	hasAdminOnlyMedia, err := l.indexHasAdminOnlyMedia(ctx)
	if err != nil {
		finishHasAdminOnly(ListTraceString("error", err.Error()))
		return err
	}
	finishHasAdminOnly(ListTraceBool("has_admin_only_media", hasAdminOnlyMedia))
	if !hasAdminOnlyMedia {
		finishCache := StartListTraceStep(ctx, "photos.index.cache_public_counts", ListTraceInt("folders", len(folders)))
		_ = l.cachePublicRecursiveMediaCounts(ctx, folders)
		finishCache()
		return nil
	}
	finishAffected := StartListTraceStep(ctx, "photos.index.admin_only_descendant_folders", ListTraceInt("folders", len(folders)))
	affected, err := l.foldersWithAdminOnlyMediaDescendants(ctx, folders)
	if err != nil {
		finishAffected(ListTraceString("error", err.Error()))
		return err
	}
	finishAffected(ListTraceInt("affected", len(affected)))
	if len(affected) == 0 {
		finishCache := StartListTraceStep(ctx, "photos.index.cache_public_counts", ListTraceInt("folders", len(folders)))
		_ = l.cachePublicRecursiveMediaCounts(ctx, folders)
		finishCache()
		return nil
	}
	recount := make([]Folder, 0, len(affected))
	for _, folder := range folders {
		if affected[folder.Path] {
			recount = append(recount, folder)
		}
	}
	finishCounts := StartListTraceStep(ctx, "photos.index.visible_recursive_counts", ListTraceInt("folders", len(recount)))
	counts, err := l.visibleRecursiveMediaCounts(ctx, recount)
	if err != nil {
		finishCounts(ListTraceString("error", err.Error()))
		return err
	}
	finishCounts(ListTraceInt("counts", len(counts)))
	for i := range folders {
		if affected[folders[i].Path] {
			folders[i].MediaCount = counts[folders[i].Path]
		}
	}
	finishCache := StartListTraceStep(ctx, "photos.index.cache_public_counts", ListTraceInt("folders", len(folders)))
	_ = l.cachePublicRecursiveMediaCounts(ctx, folders)
	finishCache()
	return nil
}

func (l *Library) cachePublicRecursiveMediaCounts(ctx context.Context, folders []Folder) error {
	if l == nil || !l.index.available() || len(folders) == 0 {
		return nil
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		UPDATE folder_index
		SET public_recursive_media_count = ?
		WHERE path = ?
			AND public_recursive_media_count <> ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, folder := range folders {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, folder.MediaCount, folder.Path, folder.MediaCount); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *Library) indexHasAdminOnlyMedia(ctx context.Context) (bool, error) {
	if l == nil || !l.index.available() {
		return false, nil
	}
	var one int
	err := l.index.db.QueryRowContext(ctx, `SELECT 1 FROM media_index WHERE admin_only = 1 LIMIT 1`).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (l *Library) foldersWithAdminOnlyMediaDescendants(ctx context.Context, folders []Folder) (map[string]bool, error) {
	affected := make(map[string]bool)
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3)
		for _, folder := range folders[start:end] {
			prefixStart, prefixEnd := prefixRange(folder.Path + "/")
			values = append(values, "(?, ?, ?)")
			args = append(args, folder.Path, prefixStart, prefixEnd)
		}
		rows, err := l.index.db.QueryContext(ctx, `
			WITH requested(path, prefix_start, prefix_end) AS (
				VALUES `+strings.Join(values, ",")+`
			)
			SELECT requested.path
			FROM requested
			WHERE EXISTS (
				SELECT 1
				FROM media_index mi
				WHERE mi.admin_only = 1
					AND mi.directory = requested.path
				LIMIT 1
			)
			OR EXISTS (
				SELECT 1
				FROM media_index mi
				WHERE mi.admin_only = 1
					AND mi.directory >= requested.prefix_start
					AND mi.directory < requested.prefix_end
				LIMIT 1
			)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				_ = rows.Close()
				return nil, err
			}
			affected[path] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return affected, nil
}

func (l *Library) visibleRecursiveMediaCounts(ctx context.Context, folders []Folder) (map[string]int, error) {
	counts := make(map[string]int, len(folders))
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3)
		for _, folder := range folders[start:end] {
			prefixStart, prefixEnd := prefixRange(folder.Path + "/")
			values = append(values, "(?, ?, ?)")
			args = append(args, folder.Path, prefixStart, prefixEnd)
		}
		rows, err := l.index.db.QueryContext(ctx, `
			WITH requested(path, prefix_start, prefix_end) AS (
				VALUES `+strings.Join(values, ",")+`
			)
			SELECT requested.path,
				(
					SELECT COUNT(*)
					FROM media_index mi
					WHERE mi.admin_only = 0
						AND mi.directory = requested.path
				) + (
					SELECT COUNT(*)
					FROM media_index mi
					WHERE mi.admin_only = 0
						AND mi.directory >= requested.prefix_start
						AND mi.directory < requested.prefix_end
				)
			FROM requested
			`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var path string
			var count int
			if err := rows.Scan(&path, &count); err != nil {
				_ = rows.Close()
				return nil, err
			}
			counts[path] = count
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return counts, nil
}
