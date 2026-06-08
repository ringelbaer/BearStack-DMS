// Datei enthaelt Ordnerabfragen aus dem Fotoindex.
package photos

import (
	"context"
	"strings"
	"time"
)

func (l *Library) indexFolders(ctx context.Context, rel string, includeAdminOnly bool) ([]Folder, error) {
	finishFolderIndex := StartListTraceStep(ctx, "photos.index.folder_index", ListTraceString("path", rel))
	folders, ok, err := l.indexFoldersFromFolderIndex(ctx, rel, includeAdminOnly)
	finishFolderIndex(
		ListTraceBool("ok", ok),
		ListTraceInt("count", len(folders)),
	)
	if ok || err != nil {
		return folders, err
	}
	finishDerived := StartListTraceStep(ctx, "photos.index.folder_derived_query", ListTraceString("path", rel))
	rows, err := l.index.db.QueryContext(ctx, indexFolderSQL(rel, includeAdminOnly), indexFolderArgs(rel)...)
	if err != nil {
		finishDerived(ListTraceString("error", err.Error()))
		return nil, err
	}
	defer rows.Close()
	folders = make([]Folder, 0)
	for rows.Next() {
		var name string
		var count int
		var modUnix int64
		if err := rows.Scan(&name, &count, &modUnix); err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		folders = append(folders, Folder{
			Name:             name,
			Path:             joinPath(rel, name),
			MediaCount:       count,
			DirectMediaCount: count,
			ModTime:          time.Unix(0, modUnix),
		})
	}
	finishDerived(ListTraceInt("count", len(folders)))
	return folders, rows.Err()
}

func (l *Library) indexFoldersFromFolderIndex(ctx context.Context, rel string, includeAdminOnly bool) ([]Folder, bool, error) {
	adminFilter := ""
	if !includeAdminOnly {
		adminFilter = " AND admin_only = 0"
	}
	finishQuery := StartListTraceStep(ctx, "photos.index.folder_index_query", ListTraceString("path", rel), ListTraceBool("include_admin_only", includeAdminOnly))
	rows, err := l.index.db.QueryContext(ctx, `
		SELECT name, path, media_count, public_media_count, recursive_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count, dir_count, mod_time_unix_nano, tags, admin_only
		FROM folder_index
		WHERE parent = ?`+adminFilter+`
		ORDER BY name COLLATE NOCASE`, rel)
	if err != nil {
		finishQuery(ListTraceString("error", err.Error()))
		return nil, false, err
	}
	defer rows.Close()
	folders := make([]Folder, 0)
	needsVisibleCountFallback := false
	needsVisibilityFallback := false
	for rows.Next() {
		scanned, err := scanIndexedFolder(rows, includeAdminOnly)
		if err != nil {
			return nil, false, err
		}
		needsVisibleCountFallback = needsVisibleCountFallback || scanned.needsVisibleCountFallback
		needsVisibilityFallback = needsVisibilityFallback || scanned.needsVisibilityFallback
		if scanned.include {
			folders = append(folders, scanned.folder)
		}
	}
	if err := rows.Err(); err != nil {
		finishQuery(ListTraceString("error", err.Error()))
		return nil, false, err
	}
	finishQuery(
		ListTraceInt("count", len(folders)),
		ListTraceBool("visible_count_fallback", needsVisibleCountFallback),
		ListTraceBool("visibility_fallback", needsVisibilityFallback),
	)
	if needsVisibleCountFallback {
		finishCounts := StartListTraceStep(ctx, "photos.index.folder_visible_counts", ListTraceInt("folders", len(folders)))
		if err := l.applyVisibleRecursiveMediaCounts(ctx, folders); err != nil {
			finishCounts(ListTraceString("error", err.Error()))
			return nil, false, err
		}
		finishCounts()
	}
	if !includeAdminOnly && needsVisibilityFallback {
		var err error
		finishVisibility := StartListTraceStep(ctx, "photos.index.folder_visibility_filter", ListTraceInt("folders", len(folders)))
		folders, err = l.filterEmptyAdminOnlyIndexedContainerFolders(ctx, folders)
		if err != nil {
			finishVisibility(ListTraceString("error", err.Error()))
			return nil, false, err
		}
		finishVisibility(ListTraceInt("visible", len(folders)))
	}
	return folders, true, nil
}

type indexedFolderScanResult struct {
	folder                    Folder
	include                   bool
	needsVisibleCountFallback bool
	needsVisibilityFallback   bool
}

func scanIndexedFolder(scanner mediaScanner, includeAdminOnly bool) (indexedFolderScanResult, error) {
	var folder Folder
	var directCount int
	var publicDirectCount int
	var recursiveCount int
	var publicRecursiveCount int
	var recursiveBlogCount int
	var publicRecursiveBlogCount int
	var modUnix int64
	var rawTags string
	var adminOnly int
	if err := scanner.Scan(&folder.Name, &folder.Path, &directCount, &publicDirectCount, &recursiveCount, &publicRecursiveCount, &recursiveBlogCount, &publicRecursiveBlogCount, &folder.DirCount, &modUnix, &rawTags, &adminOnly); err != nil {
		return indexedFolderScanResult{}, err
	}
	folder.DirectMediaCount = directCount
	folder.MediaCount = recursiveCount
	includeFolder := true
	needsVisibleCountFallback := false
	needsVisibilityFallback := false
	if !includeAdminOnly {
		if publicDirectCount >= 0 {
			folder.DirectMediaCount = publicDirectCount
		}
		if publicRecursiveCount >= 0 {
			folder.MediaCount = publicRecursiveCount
		} else {
			needsVisibleCountFallback = true
		}
		if publicRecursiveCount >= 0 && publicRecursiveBlogCount >= 0 {
			includeFolder = publicRecursiveCount > 0 || publicRecursiveBlogCount > 0 || (recursiveCount == 0 && recursiveBlogCount == 0)
		} else {
			needsVisibilityFallback = true
		}
	}
	folder.ModTime = time.Unix(0, modUnix)
	folder.Tags = tagsFromJSON(rawTags)
	folder.AdminOnly = adminOnly != 0
	return indexedFolderScanResult{
		folder:                    folder,
		include:                   includeFolder,
		needsVisibleCountFallback: needsVisibleCountFallback,
		needsVisibilityFallback:   needsVisibilityFallback,
	}, nil
}

func (l *Library) indexSearchFolders(ctx context.Context, rel, query string, includeAdminOnly bool) ([]Folder, error) {
	if l == nil || !l.index.available() {
		return nil, nil
	}
	plan := indexQueryPlanFor(query)
	where := []string{}
	args := []any{}
	joinSearch := !plan.Disjunctive && folderSearchNeedsFTS(plan)
	if joinSearch {
		fts := folderSearchFTSQuery(plan, query)
		if fts != "" {
			where = append(where, "folder_search MATCH ?")
			args = append(args, fts)
		} else {
			joinSearch = false
		}
	}
	for _, term := range plan.SQLTerms {
		if term.Field != "tag" {
			continue
		}
		for _, tag := range cleanPhotoTags([]string{term.Value}) {
			if term.Negated {
				where = append(where, "NOT EXISTS (SELECT 1 FROM folder_tag_index fti WHERE fti.folder_path = fi.path AND fti.tag = ?)")
			} else {
				where = append(where, "fi.path IN (SELECT folder_path FROM folder_tag_index WHERE tag = ?)")
			}
			args = append(args, tag)
		}
	}
	if rel != "" {
		start, end := prefixRange(rel + "/")
		where = append(where, "(fi.path = ? OR (fi.path >= ? AND fi.path < ?))")
		args = append(args, rel, start, end)
	}
	if len(where) == 0 && !plan.PostFilter {
		return nil, nil
	}
	if !includeAdminOnly {
		where = append(where, "fi.admin_only = 0")
	}
	from := "folder_index fi"
	if joinSearch {
		from += " JOIN folder_search ON folder_search.rowid = fi.rowid"
	}
	sql := `
		SELECT fi.name, fi.path, fi.media_count, fi.public_media_count, fi.recursive_media_count, fi.public_recursive_media_count, fi.recursive_blog_count, fi.public_recursive_blog_count, fi.dir_count, fi.mod_time_unix_nano, fi.tags, fi.admin_only
		FROM ` + from
	if len(where) > 0 {
		sql += ` WHERE ` + strings.Join(where, " AND ")
	}
	sql += ` ORDER BY fi.name COLLATE NOCASE`
	if !plan.PostFilter {
		sql += ` LIMIT 50`
	}
	rows, err := l.index.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	folders := make([]Folder, 0)
	needsVisibleCountFallback := false
	needsVisibilityFallback := false
	for rows.Next() {
		scanned, err := scanIndexedFolder(rows, includeAdminOnly)
		if err != nil {
			return nil, err
		}
		needsVisibleCountFallback = needsVisibleCountFallback || scanned.needsVisibleCountFallback
		needsVisibilityFallback = needsVisibilityFallback || scanned.needsVisibilityFallback
		if scanned.include && matchesFolderQuery(scanned.folder, query) {
			folders = append(folders, scanned.folder)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if needsVisibleCountFallback {
		if err := l.applyVisibleRecursiveMediaCounts(ctx, folders); err != nil {
			return nil, err
		}
	}
	if !includeAdminOnly && needsVisibilityFallback {
		var err error
		folders, err = l.filterEmptyAdminOnlyIndexedContainerFolders(ctx, folders)
		if err != nil {
			return nil, err
		}
	}
	if plan.PostFilter {
		filtered := folders[:0]
		for _, folder := range folders {
			if matchesFolderQuery(folder, query) {
				filtered = append(filtered, folder)
			}
		}
		folders = filtered
		if len(folders) > 50 {
			folders = folders[:50]
		}
	}
	return folders, nil
}

func indexFolderSQL(rel string, includeAdminOnly bool) string {
	adminFilter := ""
	if !includeAdminOnly {
		adminFilter = " AND admin_only = 0"
	}
	if rel == "" {
		return `
			SELECT child, COUNT(*) AS media_count, MAX(mod_time_unix_nano) AS mod_time
			FROM (
				SELECT CASE
					WHEN instr(directory, '/') = 0 THEN directory
					ELSE substr(directory, 1, instr(directory, '/') - 1)
				END AS child, mod_time_unix_nano
				FROM media_index
				WHERE directory <> ''` + adminFilter + `
			)
			WHERE child <> ''
			GROUP BY child
			ORDER BY child COLLATE NOCASE`
	}
	return `
		SELECT child, COUNT(*) AS media_count, MAX(mod_time_unix_nano) AS mod_time
		FROM (
			SELECT CASE
				WHEN instr(substr(directory, ?), '/') = 0 THEN substr(directory, ?)
				ELSE substr(substr(directory, ?), 1, instr(substr(directory, ?), '/') - 1)
			END AS child, mod_time_unix_nano
			FROM media_index
			WHERE directory >= ? AND directory < ?` + adminFilter + `
		)
		WHERE child <> ''
		GROUP BY child
		ORDER BY child COLLATE NOCASE`
}

func indexFolderArgs(rel string) []any {
	if rel == "" {
		return nil
	}
	start := len(rel) + 2
	rangeStart, rangeEnd := prefixRange(rel + "/")
	return []any{start, start, start, start, rangeStart, rangeEnd}
}
