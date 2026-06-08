// Datei befuellt und waehlt Ordner-Vorschaumedien fuer Foto-Listings.
package photos

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func limitFolderPreviewMedia(previews []Media, limit int) []Media {
	limit = NormalizeFolderPreviewCount(limit)
	if len(previews) <= limit {
		return previews
	}
	return previews[:limit]
}

func (l *Library) populateFolderPreviews(ctx context.Context, folders []Folder, previewLimit int, includeAdminOnly, recursiveFallback, allowFilesystemFallback bool) error {
	previewLimit = NormalizeFolderPreviewCount(previewLimit)
	indexPreviews := map[string][]Media{}
	persistedPreviews := map[string][]Media{}
	if l != nil && l.index.available() && len(folders) > 0 {
		finishLoad := StartListTraceStep(ctx, "photos.previews.cache_load", ListTraceInt("folders", len(folders)), ListTraceInt("limit", previewLimit))
		previews, err := l.loadFolderPreviewIndexBatch(ctx, folders, previewLimit, includeAdminOnly)
		if err != nil {
			finishLoad(ListTraceString("error", err.Error()))
			return err
		}
		finishLoad(ListTraceInt("folders_with_preview", len(previews)), ListTraceInt("preview_items", previewMapItemCount(previews)))
		persistedPreviews = previews
		missing := make([]Folder, 0)
		missingPaths := make([]string, 0)
		for _, folder := range folders {
			if len(previews[folder.Path]) > 0 || folder.MediaCount == 0 {
				continue
			}
			missing = append(missing, folder)
			missingPaths = append(missingPaths, folder.Path)
		}
		if len(missing) > 0 {
			cacheLimit := previewLimit
			if cacheLimit < folderPreviewSize {
				cacheLimit = folderPreviewSize
			}
			finishMissing := StartListTraceStep(ctx, "photos.previews.index_missing", ListTraceInt("folders", len(missing)), ListTraceInt("limit", cacheLimit))
			previews, err := l.indexFolderPreviewMediaBatch(ctx, missing, cacheLimit, includeAdminOnly)
			if err != nil {
				finishMissing(ListTraceString("error", err.Error()))
				return err
			}
			finishMissing(ListTraceInt("folders_with_preview", len(previews)), ListTraceInt("preview_items", previewMapItemCount(previews)))
			indexPreviews = previews
			finishSave := StartListTraceStep(ctx, "photos.previews.cache_save", ListTraceInt("folders", len(missingPaths)))
			_ = l.saveFolderPreviewIndexBatch(ctx, missingPaths, previews)
			finishSave()
		}
	}
	filesystemFallbacks := 0
	assigned := 0
	finishAssign := StartListTraceStep(ctx, "photos.previews.assign", ListTraceInt("folders", len(folders)), ListTraceBool("filesystem_fallback_allowed", allowFilesystemFallback), ListTraceBool("recursive_fallback", recursiveFallback))
	for i := range folders {
		if err := ctx.Err(); err != nil {
			finishAssign(ListTraceString("error", err.Error()))
			return err
		}
		if folders[i].previewScanned {
			folders[i].Previews = limitFolderPreviewMedia(folders[i].Previews, previewLimit)
			if len(folders[i].Previews) > 0 {
				assigned++
			}
			continue
		}
		if previews := persistedPreviews[folders[i].Path]; len(previews) > 0 {
			folders[i].Previews = limitFolderPreviewMedia(previews, previewLimit)
			assigned++
			continue
		}
		if previews := indexPreviews[folders[i].Path]; len(previews) > 0 {
			folders[i].Previews = limitFolderPreviewMedia(previews, previewLimit)
			assigned++
			continue
		}
		if !allowFilesystemFallback {
			continue
		}
		if recursiveFallback && filesystemFallbacks >= folderPreviewFallbackLimit {
			continue
		}
		var previews []Media
		var err error
		if recursiveFallback {
			filesystemFallbacks++
			previews, err = l.filesystemFolderPreviewMedia(ctx, folders[i].Path, previewLimit, includeAdminOnly)
		} else {
			previews, err = l.filesystemDirectFolderPreviewMedia(ctx, folders[i].Path, previewLimit, includeAdminOnly)
		}
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			finishAssign(ListTraceString("error", err.Error()))
			return err
		}
		folders[i].Previews = limitFolderPreviewMedia(previews, previewLimit)
		if len(folders[i].Previews) > 0 {
			assigned++
		}
	}
	finishAssign(ListTraceInt("filesystem_fallbacks", filesystemFallbacks), ListTraceInt("assigned", assigned))
	return nil
}

func (l *Library) indexFolderPreviewMediaBatch(ctx context.Context, folders []Folder, limit int, includeAdminOnly bool) (map[string][]Media, error) {
	previews := make(map[string][]Media, len(folders))
	if limit <= 0 || len(folders) == 0 {
		return previews, nil
	}
	directFolders := make([]Folder, 0, len(folders))
	recursiveFolders := make([]Folder, 0)
	for _, folder := range folders {
		if folder.DirCount == 0 {
			directFolders = append(directFolders, folder)
		} else {
			recursiveFolders = append(recursiveFolders, folder)
		}
	}
	if len(directFolders) > 0 {
		finishDirect := StartListTraceStep(ctx, "photos.previews.index_direct", ListTraceInt("folders", len(directFolders)), ListTraceInt("limit", limit))
		directPreviews, err := l.indexDirectFolderPreviewMediaBatch(ctx, directFolders, limit, includeAdminOnly)
		if err != nil {
			finishDirect(ListTraceString("error", err.Error()))
			return nil, err
		}
		finishDirect(ListTraceInt("folders_with_preview", len(directPreviews)), ListTraceInt("preview_items", previewMapItemCount(directPreviews)))
		for path, items := range directPreviews {
			previews[path] = items
		}
	}
	if len(recursiveFolders) == 0 {
		return previews, nil
	}
	finishRecursive := StartListTraceStep(ctx, "photos.previews.index_recursive", ListTraceInt("folders", len(recursiveFolders)), ListTraceInt("limit", limit))
	recursivePreviews, err := l.indexRecursiveFolderPreviewMediaBatch(ctx, recursiveFolders, limit, includeAdminOnly)
	if err != nil {
		finishRecursive(ListTraceString("error", err.Error()))
		return nil, err
	}
	finishRecursive(ListTraceInt("folders_with_preview", len(recursivePreviews)), ListTraceInt("preview_items", previewMapItemCount(recursivePreviews)))
	for path, items := range recursivePreviews {
		previews[path] = items
	}
	return previews, nil
}

func (l *Library) indexDirectFolderPreviewMediaBatch(ctx context.Context, folders []Folder, limit int, includeAdminOnly bool) (map[string][]Media, error) {
	previews := make(map[string][]Media, len(folders))
	if limit <= 0 || len(folders) == 0 {
		return previews, nil
	}
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, end-start+1)
		for _, folder := range folders[start:end] {
			values = append(values, "(?)")
			args = append(args, folder.Path)
		}
		args = append(args, limit)
		adminFilter := ""
		if !includeAdminOnly {
			adminFilter = " AND mi.admin_only = 0"
		}
		rows, err := l.index.db.QueryContext(ctx, `
			WITH requested(path) AS (
				VALUES `+strings.Join(values, ",")+`
			),
			ranked AS (
				SELECT requested.path AS folder_path, `+mediaIndexColumnsWithMetadata(`mi`, false)+`,
					ROW_NUMBER() OVER (
						PARTITION BY requested.path
						ORDER BY CASE WHEN mi.type = 'image' THEN 0 ELSE 1 END,
							COALESCE(mi.rating, 0) DESC, mi.captured_at DESC, mi.mod_time_unix_nano DESC, mi.path DESC
					) AS preview_rank
				FROM requested
				JOIN media_index mi ON mi.directory = requested.path`+adminFilter+`
			)
			SELECT folder_path, `+mediaIndexColumnsWithMetadata(``, false)+`
			FROM ranked
			WHERE preview_rank <= ?
			ORDER BY folder_path ASC, preview_rank ASC`, args...)
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

func (l *Library) indexRecursiveFolderPreviewMediaBatch(ctx context.Context, folders []Folder, limit int, includeAdminOnly bool) (map[string][]Media, error) {
	previews := make(map[string][]Media, len(folders))
	if limit <= 0 || len(folders) == 0 {
		return previews, nil
	}
	for start := 0; start < len(folders); start += folderPreviewIndexBatchSize {
		end := start + folderPreviewIndexBatchSize
		if end > len(folders) {
			end = len(folders)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3+2)
		for _, folder := range folders[start:end] {
			rel := folder.Path
			prefixStart, prefixEnd := "", string(rune(0x10ffff))
			if rel != "" {
				prefixStart, prefixEnd = prefixRange(rel + "/")
			}
			values = append(values, "(?, ?, ?)")
			args = append(args, rel, prefixStart, prefixEnd)
		}
		args = append(args, limit, limit)
		adminFilter := ""
		if !includeAdminOnly {
			adminFilter = " AND mi.admin_only = 0"
		}
		rows, err := l.index.db.QueryContext(ctx, `
			WITH requested(path, prefix_start, prefix_end) AS (
				VALUES `+strings.Join(values, ",")+`
			),
			candidates AS (
				SELECT requested.path AS folder_path, `+mediaIndexColumnsWithMetadata(`mi`, false)+`,
					CASE
						WHEN requested.path = '' THEN
							CASE
								WHEN mi.directory = '' THEN ''
								WHEN instr(mi.directory, '/') = 0 THEN mi.directory
								ELSE substr(mi.directory, 1, instr(mi.directory, '/') - 1)
							END
						WHEN mi.directory = requested.path THEN ''
						ELSE
							CASE
								WHEN instr(substr(mi.directory, length(requested.path) + 2), '/') = 0 THEN substr(mi.directory, length(requested.path) + 2)
								ELSE substr(substr(mi.directory, length(requested.path) + 2), 1, instr(substr(mi.directory, length(requested.path) + 2), '/') - 1)
							END
					END AS preview_group
				FROM requested
				JOIN media_index mi ON (
					requested.path = ''
					OR mi.directory = requested.path
					OR (mi.directory >= requested.prefix_start AND mi.directory < requested.prefix_end)
				)`+adminFilter+`
			),
			source_ranked AS (
				SELECT *,
					ROW_NUMBER() OVER (
						PARTITION BY folder_path, preview_group
						ORDER BY CASE WHEN type = 'image' THEN 0 ELSE 1 END,
							COALESCE(rating, 0) DESC, captured_at DESC, mod_time_unix_nano DESC, path DESC
					) AS source_rank
				FROM candidates
			),
			preview_ranked AS (
				SELECT *,
					ROW_NUMBER() OVER (
						PARTITION BY folder_path
						ORDER BY source_rank ASC,
							CASE WHEN type = 'image' THEN 0 ELSE 1 END,
							COALESCE(rating, 0) DESC, captured_at DESC, mod_time_unix_nano DESC, path DESC
					) AS preview_rank
				FROM source_ranked
				WHERE source_rank <= ?
			)
			SELECT folder_path, `+mediaIndexColumnsWithMetadata(``, false)+`
			FROM preview_ranked
			WHERE preview_rank <= ?
			ORDER BY folder_path ASC, preview_rank ASC`, args...)
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

func (l *Library) filesystemDirectFolderPreviewMedia(ctx context.Context, rel string, limit int, includeAdminOnly bool) ([]Media, error) {
	if limit <= 0 {
		return nil, nil
	}
	abs, err := l.Resolve(rel)
	if err != nil {
		return nil, err
	}
	if !includeAdminOnly && l.directoryAdminOnlyFromAbs(rel, abs) {
		return nil, errAdminOnly
	}
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	candidates := make([]Media, 0, limit)
	seen := 0
	adminOnly := l.directoryAdminOnlyFromAbs(rel, abs)
	for {
		entries, err := file.ReadDir(256)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			seen++
			if seen > fastFolderSummaryEntryLimit {
				return selectFolderPreviewMedia(rel, candidates, limit), nil
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || ignoredName(entry.Name()) {
				continue
			}
			kind, ok := supportedKind(entry.Name())
			if !ok || !isMediaKind(kind) {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			candidates = append(candidates, l.quickMediaFromPathInfo(joinPath(rel, entry.Name()), info, kind, adminOnly))
		}
		if errors.Is(err, io.EOF) {
			return selectFolderPreviewMedia(rel, candidates, limit), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (l *Library) filesystemFolderPreviewMedia(ctx context.Context, rel string, limit int, includeAdminOnly bool) ([]Media, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return nil, err
	}
	if !includeAdminOnly && l.directoryAdminOnlyFromAbs(rel, abs) {
		return nil, errAdminOnly
	}
	candidates := make([]Media, 0, limit)
	err = walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root:             abs,
		IncludeAdminOnly: includeAdminOnly,
	}, func(path string, _ os.DirEntry, kind string) error {
		if !isMediaKind(kind) {
			return nil
		}
		childRel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		media, err := l.mediaFromPath(ctx, filepath.ToSlash(childRel))
		if err != nil {
			return nil
		}
		candidates = append(candidates, media)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return selectFolderPreviewMedia(rel, candidates, limit), nil
}

func selectFolderPreviewMedia(rel string, candidates []Media, limit int) []Media {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	candidates = append([]Media(nil), candidates...)
	sort.SliceStable(candidates, func(i, j int) bool {
		return folderPreviewMediaLess(candidates[i], candidates[j])
	})
	selected := make([]Media, 0, min(limit, len(candidates)))
	selectedPaths := make(map[string]struct{}, limit)
	usedGroups := map[string]struct{}{}
	for _, media := range candidates {
		group := folderPreviewGroup(rel, media.Path)
		if _, ok := usedGroups[group]; ok {
			continue
		}
		selected = append(selected, media)
		selectedPaths[media.Path] = struct{}{}
		usedGroups[group] = struct{}{}
		if len(selected) >= limit {
			return selected
		}
	}
	for _, media := range candidates {
		if _, ok := selectedPaths[media.Path]; ok {
			continue
		}
		selected = append(selected, media)
		if len(selected) >= limit {
			return selected
		}
	}
	return selected
}

func folderPreviewMediaLess(left, right Media) bool {
	leftType, rightType := 1, 1
	if left.Type == MediaTypeImage {
		leftType = 0
	}
	if right.Type == MediaTypeImage {
		rightType = 0
	}
	if leftType != rightType {
		return leftType < rightType
	}
	leftRating, rightRating := 0.0, 0.0
	if left.Rating != nil {
		leftRating = *left.Rating
	}
	if right.Rating != nil {
		rightRating = *right.Rating
	}
	if leftRating != rightRating {
		return leftRating > rightRating
	}
	leftDate, rightDate := mediaDate(left), mediaDate(right)
	if !leftDate.Equal(rightDate) {
		return leftDate.After(rightDate)
	}
	if !left.ModTime.Equal(right.ModTime) {
		return left.ModTime.After(right.ModTime)
	}
	return left.Path > right.Path
}

func folderPreviewGroup(rel, mediaPath string) string {
	directory := parentPath(mediaPath)
	if rel != "" {
		if directory == rel {
			return ""
		}
		prefix := rel + "/"
		if strings.HasPrefix(directory, prefix) {
			return firstPathSegment(strings.TrimPrefix(directory, prefix))
		}
		return directory
	}
	return firstPathSegment(directory)
}

func firstPathSegment(value string) string {
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	if index := strings.Index(value, "/"); index >= 0 {
		return value[:index]
	}
	return value
}
