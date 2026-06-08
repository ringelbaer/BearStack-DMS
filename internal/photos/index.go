// Datei orchestriert den Fotoindex-Lauf, Ordner-Scans und Cache-Entscheidungen.
package photos

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type indexQueueItem struct {
	Path            string
	IndexedDirCount int
	ParentAdminOnly bool
}

type indexDirectoryStepResult struct {
	Children         []indexQueueItem
	Scanned          bool
	Skipped          bool
	RootEmptySkipped bool
	Files            int
	DBWrites         int
}

type indexRunCache struct {
	rootScanSignature  int64
	rootQuickSignature int64
	hasRootScan        bool
	folderStates       map[string]indexedFolderState
	folderChildren     map[string][]indexQueueItem
	stats              IndexStats
	hasIndexedContent  bool
}

type indexedFolderState struct {
	ScanSignature  int64
	QuickSignature int64
	DirCount       int
	AdminOnly      bool
}

const unknownIndexedDirCount = -1

var (
	emptyMediaCache = map[string]cachedMediaRow{}
	emptyBlogCache  = map[string]cachedBlogRow{}
)

func markAffectedFolders(paths map[string]struct{}, rel string) {
	for rel != "" {
		paths[rel] = struct{}{}
		rel = parentPath(rel)
	}
}

func slowIndexStep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *Library) IndexTelemetry() IndexTelemetry {
	if l == nil {
		return IndexTelemetry{}
	}
	l.telemetryMu.RLock()
	defer l.telemetryMu.RUnlock()
	telemetry := l.telemetry
	if len(l.telemetry.LastErrors) > 0 {
		telemetry.LastErrors = append([]string(nil), l.telemetry.LastErrors...)
	}
	return telemetry
}

func (l *Library) startIndexTelemetry() IndexTelemetry {
	telemetry := IndexTelemetry{
		Running:   true,
		StartedAt: time.Now().UTC(),
	}
	l.telemetryMu.Lock()
	l.telemetry = telemetry
	l.telemetryMu.Unlock()
	return telemetry
}

func (l *Library) finishIndexTelemetry(telemetry IndexTelemetry, stats IndexStats, err error) {
	now := time.Now().UTC()
	telemetry.Running = false
	telemetry.FinishedAt = now
	telemetry.Duration = now.Sub(telemetry.StartedAt)
	telemetry.Stats = stats
	if telemetry.Duration > 0 && telemetry.Files > 0 {
		telemetry.FilesPerSecond = float64(telemetry.Files) / telemetry.Duration.Seconds()
	}
	if err != nil {
		telemetry.addError("", err)
	}
	l.telemetryMu.Lock()
	l.telemetry = telemetry
	l.telemetryMu.Unlock()
}

func (telemetry *IndexTelemetry) addError(path string, err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if path != "" {
		message = path + ": " + message
	}
	const maxIndexTelemetryErrors = 5
	if len(telemetry.LastErrors) >= maxIndexTelemetryErrors {
		copy(telemetry.LastErrors, telemetry.LastErrors[1:])
		telemetry.LastErrors[len(telemetry.LastErrors)-1] = message
		return
	}
	telemetry.LastErrors = append(telemetry.LastErrors, message)
}

func (l *Library) loadIndexRunCache(ctx context.Context) (indexRunCache, error) {
	if l == nil {
		return indexRunCache{}, nil
	}
	return l.index.loadIndexRunCache(ctx)
}

func (l *Library) loadRootFolderScan(ctx context.Context) (int64, int64, bool, error) {
	if l == nil {
		return 0, 0, false, nil
	}
	return l.index.loadRootFolderScan(ctx)
}

func (cache indexRunCache) folderQuickScanIsFresh(rel string, quickSignature int64, adminOnly bool) bool {
	if rel == "" {
		return cache.hasRootScan && cache.rootQuickSignature == quickSignature
	}
	state, ok := cache.folderStates[rel]
	return ok && state.QuickSignature == quickSignature && state.AdminOnly == adminOnly
}

func (cache indexRunCache) folderScanIsFresh(rel string, scanSignature int64, adminOnly bool) bool {
	if rel == "" {
		return cache.hasRootScan && cache.rootScanSignature == scanSignature
	}
	state, ok := cache.folderStates[rel]
	return ok && state.ScanSignature == scanSignature && state.AdminOnly == adminOnly
}

func (cache indexRunCache) indexedChildFolders(rel string) []indexQueueItem {
	return cache.folderChildren[rel]
}

func (cache indexRunCache) indexedDirCount(rel string, fallback int) int {
	if fallback != unknownIndexedDirCount {
		return fallback
	}
	if rel == "" {
		return unknownIndexedDirCount
	}
	if state, ok := cache.folderStates[rel]; ok {
		return state.DirCount
	}
	return unknownIndexedDirCount
}

func (l *Library) indexDirectoryStep(ctx context.Context, item indexQueueItem, runCache indexRunCache) (indexDirectoryStepResult, error) {
	rel := item.Path
	abs, err := l.Resolve(rel)
	if err != nil {
		return indexDirectoryStepResult{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return indexDirectoryStepResult{}, err
	}
	if !info.IsDir() {
		return indexDirectoryStepResult{}, fmt.Errorf("photo path is not a directory: %s", rel)
	}
	adminOnly := item.ParentAdminOnly || adminOnlyMarkerExists(abs)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return indexDirectoryStepResult{}, err
	}
	orderMode := normalizeDirectoryOrderMode(directoryOrder(entries))
	quickSignature := folderQuickScanSignature(info, entries)
	var scanSignature int64
	var entryInfoCache map[string]os.FileInfo
	if runCache.folderQuickScanIsFresh(rel, quickSignature, adminOnly) {
		scanSignature, entryInfoCache = folderScanSignatureWithInfoCache(info, entries)
		if runCache.folderScanIsFresh(rel, scanSignature, adminOnly) {
			if runCache.indexedDirCount(rel, item.IndexedDirCount) == 0 {
				return indexDirectoryStepResult{Skipped: true}, nil
			}
			return indexDirectoryStepResult{Children: freshIndexedChildFolders(runCache.indexedChildFolders(rel), adminOnly), Skipped: true}, nil
		}
	}

	if rel == "" && len(entries) == 0 && runCache.hasIndexedContent {
		return indexDirectoryStepResult{RootEmptySkipped: true, Skipped: true}, nil
	}
	var children []indexQueueItem
	var seenFolders map[string]struct{}
	var seenMedia map[string]struct{}
	var seenBlogs map[string]struct{}
	var mediaToSave []Media
	var blogsToSave []BlogPost
	var mediaCache map[string]cachedMediaRow
	mediaCacheLoaded := false
	var blogCache map[string]cachedBlogRow
	blogCacheLoaded := false
	trackExisting := runCache.hasIndexedContent
	if trackExisting {
		seenFolders = make(map[string]struct{})
	}
	mediaCount := 0
	blogCount := 0
	dirCount := 0
	fileCount := 0
	signatureBuilder := newFolderScanSignatureBuilder(info)

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return indexDirectoryStepResult{}, err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if ignoredName(name) {
			continue
		}
		childRel := joinPath(rel, name)
		if entry.IsDir() {
			signatureBuilder.addDir(name)
			dirCount++
			children = append(children, indexQueueItem{
				Path:            childRel,
				IndexedDirCount: unknownIndexedDirCount,
				ParentAdminOnly: adminOnly,
			})
			if trackExisting {
				seenFolders[childRel] = struct{}{}
			}
			continue
		}
		kind, ok := supportedKind(name)
		if !ok {
			if strings.EqualFold(filepath.Ext(name), ".xmp") {
				childInfo, err := directoryEntryInfo(entry, entryInfoCache)
				if err == nil && !childInfo.IsDir() && childInfo.Mode()&os.ModeSymlink == 0 {
					signatureBuilder.addFile(name, childInfo)
				}
			}
			continue
		}
		if isMediaKind(kind) {
			childInfo, err := directoryEntryInfo(entry, entryInfoCache)
			if err != nil || childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			signatureBuilder.addFile(name, childInfo)
			fileCount++
			if !mediaCacheLoaded {
				if trackExisting {
					mediaCache, err = l.directoryMediaCache(ctx, rel)
					if err != nil {
						return indexDirectoryStepResult{}, err
					}
					seenMedia = make(map[string]struct{}, len(mediaCache)+1)
				} else {
					mediaCache = emptyMediaCache
				}
				mediaCacheLoaded = true
			}
			media, changed, err := l.mediaFromPathInfo(childRel, filepath.Join(abs, name), childInfo, kind, mediaCache, adminOnly, false)
			if err == nil {
				mediaCount++
				if trackExisting {
					seenMedia[media.Path] = struct{}{}
				}
				if changed {
					mediaToSave = append(mediaToSave, media)
				}
			}
			continue
		}
		switch kind {
		case MediaTypeBlog:
			childInfo, err := directoryEntryInfo(entry, entryInfoCache)
			if err != nil || childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			signatureBuilder.addFile(name, childInfo)
			fileCount++
			if !blogCacheLoaded {
				if trackExisting {
					blogCache, err = l.directoryBlogCache(ctx, rel)
					if err != nil {
						return indexDirectoryStepResult{}, err
					}
					seenBlogs = make(map[string]struct{}, len(blogCache)+1)
				} else {
					blogCache = emptyBlogCache
				}
				blogCacheLoaded = true
			}
			post, changed, err := l.blogFromPathInfo(childRel, filepath.Join(abs, name), childInfo, blogCache, adminOnly)
			if err == nil {
				blogCount++
				if trackExisting {
					seenBlogs[childRel] = struct{}{}
				}
				if changed {
					blogsToSave = append(blogsToSave, post)
				}
			}
		}
	}
	scanSignature = signatureBuilder.fullSignature()

	if rel == "" && runCache.hasIndexedContent && len(children) == 0 && mediaCount == 0 && blogCount == 0 {
		return indexDirectoryStepResult{RootEmptySkipped: true, Skipped: true, Files: fileCount}, nil
	}
	dbWrites := len(mediaToSave) + len(blogsToSave) + 1
	if err := l.saveMediaBatchWithExisting(ctx, mediaToSave, mediaCache); err != nil {
		return indexDirectoryStepResult{}, err
	}
	if err := l.saveBlogBatch(ctx, blogsToSave); err != nil {
		return indexDirectoryStepResult{}, err
	}
	if err := l.saveScannedFolder(ctx, rel, info.ModTime(), mediaCount, blogCount, dirCount, adminOnly, orderMode); err != nil {
		return indexDirectoryStepResult{}, err
	}
	if rel != "" {
		dbWrites++
	}
	if trackExisting {
		if mediaCacheLoaded {
			pruned, err := l.pruneDirectoryMediaCache(ctx, seenMedia, mediaCache)
			if err != nil {
				return indexDirectoryStepResult{}, err
			}
			dbWrites += pruned
		} else {
			pruned, err := l.pruneDirectoryMedia(ctx, rel, seenMedia)
			if err != nil {
				return indexDirectoryStepResult{}, err
			}
			dbWrites += pruned
		}
		if blogCacheLoaded {
			pruned, err := l.pruneDirectoryBlogCache(ctx, seenBlogs, blogCache)
			if err != nil {
				return indexDirectoryStepResult{}, err
			}
			dbWrites += pruned
		} else {
			pruned, err := l.pruneDirectoryBlogs(ctx, rel, seenBlogs)
			if err != nil {
				return indexDirectoryStepResult{}, err
			}
			dbWrites += pruned
		}
		pruned, err := l.pruneDirectoryFolders(ctx, rel, seenFolders)
		if err != nil {
			return indexDirectoryStepResult{}, err
		}
		dbWrites += pruned
	}
	if err := l.saveFolderScan(ctx, rel, scanSignature, quickSignature, orderMode); err != nil {
		return indexDirectoryStepResult{}, err
	}
	return indexDirectoryStepResult{Children: children, Scanned: true, Files: fileCount, DBWrites: dbWrites}, nil
}

func folderScanSignature(info os.FileInfo, entries []os.DirEntry) int64 {
	signature, _ := folderScanSignatureWithInfoCache(info, entries)
	return signature
}

func folderQuickScanSignature(info os.FileInfo, entries []os.DirEntry) int64 {
	builder := newFolderScanSignatureBuilder(info)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if ignoredName(name) {
			continue
		}
		if entry.IsDir() {
			builder.addDir(name)
			continue
		}
		if scanSignatureIncludesFile(name) {
			builder.addFileName(name)
		}
	}
	return builder.quickSignature()
}

func folderScanSignatureWithInfoCache(info os.FileInfo, entries []os.DirEntry) (int64, map[string]os.FileInfo) {
	builder := newFolderScanSignatureBuilder(info)
	infoCache := make(map[string]os.FileInfo)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if ignoredName(name) {
			continue
		}
		if entry.IsDir() {
			builder.addDir(name)
			continue
		}
		if !scanSignatureIncludesFile(name) {
			continue
		}
		childInfo, err := entry.Info()
		if err != nil || childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		builder.addFile(name, childInfo)
		infoCache[name] = childInfo
	}
	return builder.fullSignature(), infoCache
}

func scanSignatureIncludesFile(name string) bool {
	if isOrderFileName(name) {
		return true
	}
	kind, ok := supportedKind(name)
	if ok {
		return isMediaKind(kind) || kind == MediaTypeBlog
	}
	return strings.EqualFold(filepath.Ext(name), ".xmp")
}

func isOrderFileName(name string) bool {
	return strings.HasPrefix(name, ".order_") && strings.HasSuffix(name, ".pg2conf")
}

type folderScanSignatureBuilder struct {
	full  hash.Hash64
	quick hash.Hash64
}

func newFolderScanSignatureBuilder(info os.FileInfo) folderScanSignatureBuilder {
	builder := folderScanSignatureBuilder{
		full:  fnv.New64a(),
		quick: fnv.New64a(),
	}
	fmt.Fprintf(builder.full, "dir:%d\n", info.ModTime().UnixNano())
	fmt.Fprintf(builder.quick, "dir:%d\n", info.ModTime().UnixNano())
	return builder
}

func (b folderScanSignatureBuilder) addDir(name string) {
	fmt.Fprintf(b.full, "d:%s\n", name)
	fmt.Fprintf(b.quick, "d:%s\n", name)
}

func (b folderScanSignatureBuilder) addFileName(name string) {
	fmt.Fprintf(b.quick, "f:%s\n", name)
}

func (b folderScanSignatureBuilder) addFile(name string, info os.FileInfo) {
	b.addFileName(name)
	fmt.Fprintf(b.full, "f:%s:%d:%d\n", name, info.ModTime().UnixNano(), info.Size())
}

func (b folderScanSignatureBuilder) fullSignature() int64 {
	return int64(b.full.Sum64())
}

func (b folderScanSignatureBuilder) quickSignature() int64 {
	return int64(b.quick.Sum64())
}

func directoryEntryInfo(entry os.DirEntry, cache map[string]os.FileInfo) (os.FileInfo, error) {
	if cache != nil {
		if info, ok := cache[entry.Name()]; ok {
			return info, nil
		}
	}
	return entry.Info()
}

func freshIndexedChildFolders(children []indexQueueItem, parentAdminOnly bool) []indexQueueItem {
	if len(children) == 0 {
		return nil
	}
	fresh := make([]indexQueueItem, 0, len(children))
	for _, child := range children {
		child.ParentAdminOnly = parentAdminOnly
		fresh = append(fresh, child)
	}
	return fresh
}

func (l *Library) directoryMediaCache(ctx context.Context, rel string) (map[string]cachedMediaRow, error) {
	if l == nil {
		return nil, nil
	}
	return l.index.directoryMediaCache(ctx, rel)
}

func (l *Library) directoryBlogCache(ctx context.Context, rel string) (map[string]cachedBlogRow, error) {
	if l == nil {
		return nil, nil
	}
	return l.index.directoryBlogCache(ctx, rel)
}

func (l *Library) blogFromPathInfo(rel, abs string, info os.FileInfo, cache map[string]cachedBlogRow, adminOnly bool) (BlogPost, bool, error) {
	if info.IsDir() {
		return BlogPost{}, false, os.ErrNotExist
	}
	if row, ok := cache[rel]; ok && row.ModTimeUnixNano == info.ModTime().UnixNano() && (row.AdminOnly != 0) == adminOnly {
		return blogFromCachedRow(row, info.ModTime()), false, nil
	}
	file, err := os.Open(abs)
	if err != nil {
		return BlogPost{}, false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBlogBytes))
	if err != nil {
		return BlogPost{}, false, err
	}
	post := BlogPost{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		AdminOnly: adminOnly,
		Date:      markdownDate(raw),
		Text:      markdownText(raw),
		HTML:      renderMarkdown(raw),
		ModTime:   info.ModTime(),
	}
	if row, ok := cache[rel]; ok {
		post.Tags = tagsFromJSON(row.Tags)
	} else if tags, ok := l.blogTags(rel); ok {
		post.Tags = tags
	}
	return post, true, nil
}

func blogFromCachedRow(row cachedBlogRow, modTime time.Time) BlogPost {
	post := BlogPost{
		Name:      row.Name,
		Path:      row.Path,
		Tags:      tagsFromJSON(row.Tags),
		AdminOnly: row.AdminOnly != 0,
		Text:      row.Text,
		HTML:      renderMarkdown([]byte(row.Text)),
		ModTime:   modTime,
	}
	if row.Date != "" {
		if parsed, err := time.Parse("2006-01-02", row.Date); err == nil {
			post.Date = &parsed
		}
	}
	return post
}

func (l *Library) saveScannedFolder(ctx context.Context, rel string, modTime time.Time, mediaCount, blogCount, dirCount int, adminOnly bool, orderMode string) error {
	if l == nil {
		return nil
	}
	return l.index.saveScannedFolder(ctx, rel, modTime, mediaCount, blogCount, dirCount, adminOnly, orderMode)
}

func (l *Library) saveFolderScan(ctx context.Context, rel string, scanSignature, quickSignature int64, orderMode string) error {
	if l == nil {
		return nil
	}
	return l.index.saveFolderScan(ctx, rel, scanSignature, quickSignature, orderMode)
}

func (l *Library) pruneDirectoryMedia(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if l == nil {
		return 0, nil
	}
	return l.index.pruneDirectoryMedia(ctx, rel, seen)
}

func (l *Library) pruneDirectoryMediaCache(ctx context.Context, seen map[string]struct{}, cache map[string]cachedMediaRow) (int, error) {
	if l == nil {
		return 0, nil
	}
	return l.index.pruneDirectoryMediaCache(ctx, seen, cache)
}

func (l *Library) pruneDirectoryBlogs(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if l == nil {
		return 0, nil
	}
	return l.index.pruneDirectoryBlogs(ctx, rel, seen)
}

func (l *Library) pruneDirectoryBlogCache(ctx context.Context, seen map[string]struct{}, cache map[string]cachedBlogRow) (int, error) {
	if l == nil {
		return 0, nil
	}
	return l.index.pruneDirectoryBlogCache(ctx, seen, cache)
}

func (l *Library) pruneDirectoryFolders(ctx context.Context, rel string, seen map[string]struct{}) (int, error) {
	if l == nil {
		return 0, nil
	}
	return l.index.pruneDirectoryFolders(ctx, rel, seen)
}

func (l *Library) refreshFolderRecursiveCounts(ctx context.Context, affected map[string]struct{}) error {
	if len(affected) == 0 {
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
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		start, end := prefixRange(path + "/")
		var count int
		var publicCount int
		var blogCount int
		var publicBlogCount int
		if err := l.index.db.QueryRowContext(ctx, `
			SELECT
				(
					SELECT COUNT(*)
					FROM media_index
					WHERE directory = ?
				) + (
					SELECT COUNT(*)
					FROM media_index
					WHERE directory >= ?
						AND directory < ?
				),
				(
					SELECT COUNT(*)
					FROM media_index
					WHERE admin_only = 0
						AND directory = ?
				) + (
					SELECT COUNT(*)
					FROM media_index
					WHERE admin_only = 0
						AND directory >= ?
						AND directory < ?
				),
				(
					SELECT COUNT(*)
					FROM blog_index
					WHERE directory = ?
				) + (
					SELECT COUNT(*)
					FROM blog_index
					WHERE directory >= ?
						AND directory < ?
				),
				(
					SELECT COUNT(*)
					FROM blog_index
					WHERE admin_only = 0
						AND directory = ?
				) + (
					SELECT COUNT(*)
					FROM blog_index
					WHERE admin_only = 0
						AND directory >= ?
						AND directory < ?
				)`,
			path,
			start,
			end,
			path,
			start,
			end,
			path,
			start,
			end,
			path,
			start,
			end,
		).Scan(&count, &publicCount, &blogCount, &publicBlogCount); err != nil {
			return err
		}
		if _, err := l.index.db.ExecContext(ctx, `
			UPDATE folder_index
			SET recursive_media_count = ?,
				public_recursive_media_count = ?,
				recursive_blog_count = ?,
				public_recursive_blog_count = ?
			WHERE path = ?
				AND (
					recursive_media_count <> ?
					OR public_recursive_media_count <> ?
					OR recursive_blog_count <> ?
					OR public_recursive_blog_count <> ?
				)`,
			count,
			publicCount,
			blogCount,
			publicBlogCount,
			path,
			count,
			publicCount,
			blogCount,
			publicBlogCount,
		); err != nil {
			return err
		}
	}
	return nil
}

func (l *Library) refreshPhotoStats(ctx context.Context) error {
	if l == nil {
		return nil
	}
	return l.index.refreshPhotoStats(ctx)
}

func (l *Library) cachedIndexStats(ctx context.Context) (IndexStats, error) {
	if l == nil {
		return IndexStats{}, nil
	}
	return l.index.cachedIndexStats(ctx)
}
