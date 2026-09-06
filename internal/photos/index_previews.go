// Datei pflegt Vorschauzuordnungen fuer Ordner und Medien im Fotoindex.
package photos

import (
	"context"
	"sort"
	"strings"

	"bearstack/internal/sqlutil"
)

// Reserve rank ranges for the selection algorithm and visibility scope. Legacy
// previews used ranks 0..3; they are ignored and replaced lazily without
// rebuilding the photo index or changing its schema. Each scope stores at most
// four paths so hidden images cannot skew the public percentage positions.
const folderPreviewCacheRankBase = 2 * MaxFolderPreviewCount

func folderPreviewCacheRankStart(includeAdminOnly bool) int {
	if includeAdminOnly {
		return folderPreviewCacheRankBase + MaxFolderPreviewCount
	}
	return folderPreviewCacheRankBase
}

func (l *Library) loadFolderPreviewIndexBatch(ctx context.Context, folders []Folder, limit int, includeAdminOnly bool) (map[string][]Media, error) {
	previews := make(map[string][]Media, len(folders))
	if l == nil || !l.index.available() || len(folders) == 0 {
		return previews, nil
	}
	limit = NormalizeFolderPreviewCount(limit)
	rankStart := folderPreviewCacheRankStart(includeAdminOnly)
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		pathArgs := make([]any, 0, end-start)
		for _, folder := range folders[start:end] {
			pathArgs = append(pathArgs, folder.Path)
		}
		args := make([]any, 0, len(pathArgs)+2)
		args = append(args, pathArgs...)
		args = append(args, rankStart, rankStart+limit)
		rows, err := l.index.db.QueryContext(ctx, `
				SELECT fpi.folder_path, `+mediaIndexColumnsWithMetadata(`mi`, false)+`
				FROM folder_preview_index fpi
				CROSS JOIN media_index mi
				WHERE fpi.folder_path IN (`+sqlutil.Placeholders(len(pathArgs))+`)
					AND fpi.rank >= ? AND fpi.rank < ?
					AND mi.path = fpi.media_path
				ORDER BY fpi.folder_path ASC, fpi.rank ASC`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var folderPath string
			media, err := scanIndexedMediaWithPrefixAndMetadata(rows, &folderPath, false)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			if !includeAdminOnly && media.AdminOnly {
				continue
			}
			if len(previews[folderPath]) >= limit {
				continue
			}
			previews[folderPath] = append(previews[folderPath], media)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return previews, nil
}

func (l *Library) refreshFolderPreviewIndex(ctx context.Context, affected map[string]struct{}) error {
	if l == nil || !l.index.available() || len(affected) == 0 {
		return nil
	}
	paths := make([]string, 0, len(affected))
	for path := range affected {
		if path != "" {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if strings.Count(paths[i], "/") == strings.Count(paths[j], "/") {
			return paths[i] < paths[j]
		}
		return strings.Count(paths[i], "/") > strings.Count(paths[j], "/")
	})
	for start := 0; start < len(paths); start += folderPreviewIndexBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + folderPreviewIndexBatchSize
		if end > len(paths) {
			end = len(paths)
		}
		dirCounts, err := l.folderDirCounts(ctx, paths[start:end])
		if err != nil {
			return err
		}
		folders := make([]Folder, 0, end-start)
		for _, path := range paths[start:end] {
			folders = append(folders, Folder{Path: path, DirCount: dirCounts[path]})
		}
		for _, includeAdminOnly := range []bool{false, true} {
			previews, err := l.indexFolderPreviewMediaBatch(ctx, folders, folderPreviewSize, includeAdminOnly)
			if err != nil {
				return err
			}
			if err := l.saveFolderPreviewIndexBatch(ctx, paths[start:end], previews, includeAdminOnly); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Library) folderDirCounts(ctx context.Context, paths []string) (map[string]int, error) {
	counts := make(map[string]int, len(paths))
	if len(paths) == 0 {
		return counts, nil
	}
	args := make([]any, 0, len(paths))
	for _, path := range paths {
		args = append(args, path)
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT path, dir_count FROM folder_index WHERE path IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var dirCount int
		if err := rows.Scan(&path, &dirCount); err != nil {
			return nil, err
		}
		counts[path] = dirCount
	}
	return counts, rows.Err()
}

func (l *Library) saveFolderPreviewIndexBatch(ctx context.Context, paths []string, previews map[string][]Media, includeAdminOnly bool) error {
	if len(paths) == 0 {
		return nil
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := make([]any, 0, len(paths))
	for _, path := range paths {
		args = append(args, path)
	}
	rankStart := folderPreviewCacheRankStart(includeAdminOnly)
	args = append(args, folderPreviewCacheRankBase, rankStart, rankStart+MaxFolderPreviewCount)
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_preview_index
		WHERE folder_path IN (`+sqlutil.Placeholders(len(paths))+`)
		AND (rank < ? OR (rank >= ? AND rank < ?))`, args...); err != nil {
		return err
	}
	type previewRow struct {
		folderPath string
		rank       int
		mediaPath  string
	}
	rows := make([]previewRow, 0, len(paths)*folderPreviewSize)
	for _, path := range paths {
		for rank, media := range limitFolderPreviewMedia(previews[path], folderPreviewSize) {
			if media.Path == "" {
				continue
			}
			rows = append(rows, previewRow{folderPath: path, rank: rankStart + rank, mediaPath: media.Path})
		}
	}
	for start := 0; start < len(rows); start += searchWriteChunkSize {
		end := start + searchWriteChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		var b strings.Builder
		b.WriteString(`INSERT INTO folder_preview_index(folder_path, rank, media_path) VALUES `)
		args := make([]any, 0, (end-start)*3)
		for i, row := range rows[start:end] {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(`(?, ?, ?)`)
			args = append(args, row.folderPath, row.rank, row.mediaPath)
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
