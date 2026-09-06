// Datei enthaelt Dateisystem-Listings, Such-Fallbacks und Sichtbarkeitsfilter der Foto-Library.
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

func (l *Library) listFromFilesystem(ctx context.Context, rel, abs string, opts ListOptions, listing *Listing, fast bool) error {
	if opts.Query != "" {
		listing.Order = l.directoryOrderForPath(rel)
		return l.search(ctx, rel, opts.IncludeAdminOnly, listing)
	}
	if opts.Recursive {
		return l.listRecursiveMedia(ctx, rel, abs, opts.IncludeAdminOnly, opts.IncludeMapData, listing)
	}
	if fast {
		return l.listDirectoryFast(ctx, rel, abs, opts, listing)
	}
	return l.listDirectory(ctx, rel, abs, opts.IncludeAdminOnly, opts.IncludeMapData, listing)
}

func (l *Library) listDirectory(ctx context.Context, rel, abs string, includeAdminOnly, includeMapData bool, listing *Listing) error {
	finishReadDir := StartListTraceStep(ctx, "photos.fs.read_dir", ListTraceString("path", rel))
	entries, err := os.ReadDir(abs)
	if err != nil {
		finishReadDir(ListTraceString("error", err.Error()))
		return err
	}
	finishReadDir(ListTraceInt("entries", len(entries)))
	order := directoryOrder(entries)
	if order == "" {
		order = "ascending_date"
	}
	listing.Order = order

	finishScan := StartListTraceStep(ctx, "photos.fs.scan_directory", ListTraceString("path", rel), ListTraceInt("entries", len(entries)))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			finishScan(ListTraceString("error", err.Error()))
			return err
		}
		name := entry.Name()
		if ignoredName(name) {
			continue
		}
		childRel := joinPath(rel, name)
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			if !includeAdminOnly && adminOnlyMarkerExists(filepath.Join(abs, name)) {
				continue
			}
			folder, err := l.folderFromPath(childRel, includeAdminOnly)
			if err == nil {
				if !includeAdminOnly && folder.MediaCount == 0 {
					hide, err := l.hideEmptyAdminOnlyContainerFolder(ctx, filepath.Join(abs, name))
					if err != nil {
						return err
					}
					if hide {
						continue
					}
				}
				listing.Folders = append(listing.Folders, folder)
			}
			continue
		}
		kind, ok := supportedKind(name)
		if !ok {
			continue
		}
		if isMediaKind(kind) {
			media, err := l.mediaFromPath(ctx, childRel)
			if err == nil {
				listing.Media = append(listing.Media, media)
			}
			continue
		}
		switch kind {
		case MediaTypeBlog:
			post, err := l.blogFromPath(childRel)
			if err == nil {
				listing.Blogs = append(listing.Blogs, post)
			}
		case mediaKindGPX:
			if !includeMapData {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			track, err := l.gpxFromPathInfo(childRel, info)
			if err == nil && len(track.Points) > 0 {
				listing.GPXTracks = append(listing.GPXTracks, track)
			}
		}
	}
	finishSort := StartListTraceStep(ctx, "photos.fs.sort_directory", ListTraceInt("folders", len(listing.Folders)), ListTraceInt("blogs", len(listing.Blogs)))
	sort.Slice(listing.Folders, func(i, j int) bool {
		return strings.ToLower(listing.Folders[i].Name) < strings.ToLower(listing.Folders[j].Name)
	})
	sort.Slice(listing.Blogs, func(i, j int) bool {
		left, right := listing.Blogs[i].ModTime, listing.Blogs[j].ModTime
		if listing.Blogs[i].Date != nil {
			left = *listing.Blogs[i].Date
		}
		if listing.Blogs[j].Date != nil {
			right = *listing.Blogs[j].Date
		}
		return left.After(right)
	})
	finishSort()
	finishScan(
		ListTraceInt("folders", len(listing.Folders)),
		ListTraceInt("blogs", len(listing.Blogs)),
		ListTraceInt("media", len(listing.Media)),
		ListTraceInt("gpx_tracks", len(listing.GPXTracks)),
	)
	return nil
}

func (l *Library) listDirectoryFast(ctx context.Context, rel, abs string, opts ListOptions, listing *Listing) error {
	finishReadDir := StartListTraceStep(ctx, "photos.fs.read_dir_fast", ListTraceString("path", rel))
	entries, err := os.ReadDir(abs)
	if err != nil {
		finishReadDir(ListTraceString("error", err.Error()))
		return err
	}
	finishReadDir(ListTraceInt("entries", len(entries)))
	order := directoryOrder(entries)
	if order == "" {
		order = "ascending_date"
	}
	listing.Order = order
	currentAdminOnly := l.directoryAdminOnlyFromAbs(rel, abs)
	mediaCandidates := make([]Media, 0)

	finishScan := StartListTraceStep(ctx, "photos.fs.scan_directory_fast", ListTraceString("path", rel), ListTraceInt("entries", len(entries)))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			finishScan(ListTraceString("error", err.Error()))
			return err
		}
		name := entry.Name()
		if ignoredName(name) {
			continue
		}
		childRel := joinPath(rel, name)
		childAbs := filepath.Join(abs, name)
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			if !opts.IncludeAdminOnly && adminOnlyMarkerExists(childAbs) {
				continue
			}
			folder, err := l.shallowFolderFromPath(ctx, childRel, childAbs, opts.IncludeAdminOnly)
			if err == nil {
				if !opts.IncludeAdminOnly && folder.MediaCount == 0 {
					hide, err := l.hideEmptyAdminOnlyContainerFolder(ctx, childAbs)
					if err != nil {
						return err
					}
					if hide {
						continue
					}
				}
				listing.Folders = append(listing.Folders, folder)
			}
			continue
		}
		kind, ok := supportedKind(name)
		if !ok {
			continue
		}
		if isMediaKind(kind) {
			if opts.MediaType != "" && kind != opts.MediaType {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			mediaCandidates = append(mediaCandidates, l.quickMediaFromPathInfo(childRel, info, kind, currentAdminOnly))
			continue
		}
		switch kind {
		case MediaTypeBlog:
			post, err := l.blogFromPath(childRel)
			if err == nil {
				listing.Blogs = append(listing.Blogs, post)
			}
		case mediaKindGPX:
			if !opts.IncludeMapData {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			track, err := l.gpxFromPathInfo(childRel, info)
			if err == nil && len(track.Points) > 0 {
				listing.GPXTracks = append(listing.GPXTracks, track)
			}
		}
	}
	finishSort := StartListTraceStep(ctx, "photos.fs.sort_directory_fast", ListTraceInt("folders", len(listing.Folders)), ListTraceInt("blogs", len(listing.Blogs)), ListTraceInt("media_candidates", len(mediaCandidates)))
	sort.Slice(listing.Folders, func(i, j int) bool {
		return strings.ToLower(listing.Folders[i].Name) < strings.ToLower(listing.Folders[j].Name)
	})
	sort.Slice(listing.Blogs, func(i, j int) bool {
		left, right := listing.Blogs[i].ModTime, listing.Blogs[j].ModTime
		if listing.Blogs[i].Date != nil {
			left = *listing.Blogs[i].Date
		}
		if listing.Blogs[j].Date != nil {
			right = *listing.Blogs[j].Date
		}
		return left.After(right)
	})
	sortMedia(mediaCandidates, listing.Order, opts.Sort)
	finishSort()
	listing.Total = len(mediaCandidates)
	start := (opts.Page - 1) * opts.PageSize
	if start > len(mediaCandidates) {
		start = len(mediaCandidates)
	}
	end := start + opts.PageSize
	if end > len(mediaCandidates) {
		end = len(mediaCandidates)
	}
	listing.Media = append([]Media(nil), mediaCandidates[start:end]...)
	finishScan(
		ListTraceInt("folders", len(listing.Folders)),
		ListTraceInt("blogs", len(listing.Blogs)),
		ListTraceInt("media_candidates", len(mediaCandidates)),
		ListTraceInt("page_media", len(listing.Media)),
		ListTraceInt("gpx_tracks", len(listing.GPXTracks)),
	)
	return nil
}

func (l *Library) shallowFolderFromPath(ctx context.Context, rel, abs string, includeAdminOnly bool) (Folder, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return Folder{}, err
	}
	if !info.IsDir() {
		return Folder{}, os.ErrNotExist
	}
	folder := Folder{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		AdminOnly: l.directoryAdminOnlyFromAbs(rel, abs),
		ModTime:   info.ModTime(),
	}
	if tags, ok := l.folderTags(rel); ok {
		folder.Tags = tags
	}
	summary, err := l.directFolderSummary(ctx, rel, abs, includeAdminOnly, fastFolderSummaryEntryLimit)
	if err != nil {
		return Folder{}, err
	}
	folder.MediaCount = summary.MediaCount
	folder.DirectMediaCount = summary.MediaCount
	folder.DirCount = summary.DirCount
	folder.MediaCountApproximate = summary.Approximate || folder.DirCount > 0
	folder.Previews = summary.Previews
	folder.previewScanned = true
	return folder, nil
}

type directFolderSummaryResult struct {
	MediaCount  int
	DirCount    int
	Approximate bool
	Previews    []Media
}

func (l *Library) directFolderSummary(ctx context.Context, rel, abs string, includeAdminOnly bool, limit int) (directFolderSummaryResult, error) {
	file, err := os.Open(abs)
	if err != nil {
		return directFolderSummaryResult{}, err
	}
	defer file.Close()
	result := directFolderSummaryResult{}
	adminOnly := l.directoryAdminOnlyFromAbs(rel, abs)
	previewCandidates := make([]Media, 0, folderPreviewSize)
	seen := 0
	for {
		entries, err := file.ReadDir(256)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return directFolderSummaryResult{}, err
			}
			seen++
			if limit > 0 && seen > limit {
				result.Approximate = true
				result.Previews = selectFolderPreviewMedia(previewCandidates, folderPreviewSize)
				return result, nil
			}
			name := entry.Name()
			if ignoredName(name) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if entry.IsDir() {
				if !includeAdminOnly && adminOnlyMarkerExists(filepath.Join(abs, name)) {
					continue
				}
				result.DirCount++
				continue
			}
			kind, ok := supportedKind(name)
			if ok && isMediaKind(kind) {
				result.MediaCount++
				info, err := entry.Info()
				if err == nil && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
					previewCandidates = append(previewCandidates, l.quickMediaFromPathInfo(joinPath(rel, name), info, kind, adminOnly))
				}
			}
		}
		if errors.Is(err, io.EOF) {
			result.Previews = selectFolderPreviewMedia(previewCandidates, folderPreviewSize)
			return result, nil
		}
		if err != nil {
			return directFolderSummaryResult{}, err
		}
	}
}

var errVisibleFolderContentFound = errors.New("visible folder content found")

func (l *Library) hideEmptyAdminOnlyContainerFolder(ctx context.Context, abs string) (bool, error) {
	visible, hidden, err := filesystemFolderVisibleContentState(ctx, abs)
	if err != nil {
		return false, err
	}
	return hidden && !visible, nil
}

func filesystemFolderVisibleContentState(ctx context.Context, abs string) (bool, bool, error) {
	hiddenAdminOnly := false
	err := walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root: abs,
		OnAdminOnlyDir: func() {
			hiddenAdminOnly = true
		},
	}, func(_ string, _ os.DirEntry, kind string) error {
		if isMediaKind(kind) || kind == MediaTypeBlog || kind == mediaKindGPX {
			return errVisibleFolderContentFound
		}
		return nil
	})
	if errors.Is(err, errVisibleFolderContentFound) {
		return true, hiddenAdminOnly, nil
	}
	if err != nil {
		return false, hiddenAdminOnly, err
	}
	return false, hiddenAdminOnly, nil
}

func (l *Library) listRecursiveMedia(ctx context.Context, rel, abs string, includeAdminOnly, includeMapData bool, listing *Listing) error {
	entries, err := os.ReadDir(abs)
	if err == nil {
		listing.Order = directoryOrder(entries)
	}
	if listing.Order == "" {
		listing.Order = "ascending_date"
	}
	return walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root:             abs,
		IncludeAdminOnly: includeAdminOnly,
	}, func(path string, entry os.DirEntry, kind string) error {
		childRel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		childRel = filepath.ToSlash(childRel)
		if kind == mediaKindGPX {
			if !includeMapData {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			track, err := l.gpxFromPathInfo(childRel, info)
			if err == nil && len(track.Points) > 0 {
				listing.GPXTracks = append(listing.GPXTracks, track)
			}
			return nil
		}
		if !isMediaKind(kind) {
			return nil
		}
		media, err := l.mediaFromPath(ctx, childRel)
		if err == nil {
			listing.Media = append(listing.Media, media)
		}
		return nil
	})
}

func (l *Library) search(ctx context.Context, rel string, includeAdminOnly bool, listing *Listing) error {
	root, err := l.Resolve(rel)
	if err != nil {
		return err
	}
	return walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root:             root,
		IncludeAdminOnly: includeAdminOnly,
	}, func(path string, entry os.DirEntry, kind string) error {
		if kind == MediaTypeBlog {
			childRel, err := filepath.Rel(l.root, path)
			if err != nil {
				return nil
			}
			post, err := l.blogFromPath(filepath.ToSlash(childRel))
			if err == nil && matchesBlogQuery(post, listing.Query) {
				listing.Blogs = append(listing.Blogs, post)
			}
			return nil
		}
		if !isMediaKind(kind) {
			return nil
		}
		childRel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		childRel = filepath.ToSlash(childRel)
		media, err := l.mediaFromPath(ctx, childRel)
		if err != nil {
			return nil
		}
		if matchesQuery(media, listing.Query) {
			listing.Media = append(listing.Media, media)
		}
		return nil
	})
}

func (l *Library) folderFromPath(rel string, includeAdminOnly bool) (Folder, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return Folder{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Folder{}, err
	}
	folder := Folder{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		AdminOnly: l.directoryAdminOnlyFromAbs(rel, abs),
	}
	if info, err := os.Stat(abs); err == nil {
		folder.ModTime = info.ModTime()
	}
	directMediaCount := 0
	for _, entry := range entries {
		if ignoredName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			folder.DirCount++
			continue
		}
		kind, ok := supportedKind(entry.Name())
		if ok && isMediaKind(kind) {
			directMediaCount++
		}
	}
	if tags, ok := l.folderTags(rel); ok {
		folder.Tags = tags
	}
	folder.MediaCount = directMediaCount
	folder.DirectMediaCount = directMediaCount
	_ = l.saveFolder(folder)
	folder.MediaCount = l.filesystemRecursiveMediaCount(abs, includeAdminOnly)
	return folder, nil
}

func (l *Library) filesystemRecursiveMediaCount(abs string, includeAdminOnly bool) int {
	count := 0
	_ = walkPhotoFilesystem(context.Background(), photoFilesystemWalkOptions{
		Root:             abs,
		IncludeAdminOnly: includeAdminOnly,
	}, func(_ string, _ os.DirEntry, kind string) error {
		if isMediaKind(kind) {
			count++
		}
		return nil
	})
	return count
}

func (l *Library) filterAdminOnlyListing(listing *Listing) {
	if l == nil || listing == nil {
		return
	}
	filteredFolders := listing.Folders[:0]
	for _, folder := range listing.Folders {
		adminOnly, err := l.FolderAdminOnly(folder.Path)
		if err == nil && adminOnly {
			continue
		}
		filteredFolders = append(filteredFolders, folder)
	}
	listing.Folders = filteredFolders

	originalMedia := len(listing.Media)
	listing.Media = l.filterAdminOnlyMedia(listing.Media)
	removedMedia := originalMedia - len(listing.Media)
	if removedMedia > 0 {
		if listing.Total >= removedMedia {
			listing.Total -= removedMedia
		} else {
			listing.Total = len(listing.Media)
		}
	}

	filteredBlogs := listing.Blogs[:0]
	for _, post := range listing.Blogs {
		adminOnly, err := l.fileAdminOnly(post.Path)
		if err == nil && adminOnly {
			continue
		}
		filteredBlogs = append(filteredBlogs, post)
	}
	listing.Blogs = filteredBlogs
}

func (l *Library) filterAdminOnlyMedia(items []Media) []Media {
	if l == nil || len(items) == 0 {
		return items
	}
	paths := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.AdminOnly || item.Path == "" {
			continue
		}
		if _, ok := seen[item.Path]; ok {
			continue
		}
		seen[item.Path] = struct{}{}
		paths = append(paths, item.Path)
	}
	adminOnlyByPath, err := l.MediaAdminOnlyBatch(paths)
	batchOK := err == nil
	filtered := items[:0]
	for _, item := range items {
		adminOnly := item.AdminOnly
		if !adminOnly && batchOK {
			adminOnly = adminOnlyByPath[item.Path]
		}
		if !adminOnly && !batchOK {
			checked, err := l.MediaAdminOnly(item.Path)
			adminOnly = err == nil && checked
		}
		if adminOnly {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
