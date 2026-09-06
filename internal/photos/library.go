// Datei definiert die Foto-Library als zentrale Fassade fuer Dateisystem, Index und Medienzugriff.
package photos

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bearstack/internal/photos/photopath"
)

const (
	defaultPageSize             = 120
	folderPreviewSize           = MaxFolderPreviewCount
	folderPreviewIndexBatchSize = 250
	folderPreviewFallbackLimit  = 64
	fastFolderSummaryEntryLimit = 500
	maxBlogBytes                = 1 << 20
)

func (l *Library) List(ctx context.Context, opts ListOptions) (Listing, error) {
	if l == nil {
		return Listing{}, errors.New("photo library is not configured")
	}
	finishPrepare := StartListTraceStep(ctx, "photos.library.prepare")
	rel, err := CleanPath(opts.Path)
	if err != nil {
		finishPrepare(ListTraceString("error", err.Error()))
		return Listing{}, err
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = l.PageSize()
	}
	if opts.FolderPreviewSize <= 0 {
		opts.FolderPreviewSize = folderPreviewSize
	}
	opts.FolderPreviewSize = NormalizeFolderPreviewCount(opts.FolderPreviewSize)
	opts.MediaType = normalizeMediaType(opts.MediaType)
	opts.Sort = normalizeSort(opts.Sort)
	opts.Query = strings.TrimSpace(opts.Query)
	if opts.Query != "" {
		rel = ""
		opts.Path = ""
	}
	finishPrepare(
		ListTraceString("path", rel),
		ListTraceString("query", opts.Query),
		ListTraceString("media_type", opts.MediaType),
		ListTraceBool("recursive", opts.Recursive),
		ListTraceBool("map", opts.IncludeMapData),
		ListTraceBool("admin_only", opts.IncludeAdminOnly),
		ListTraceInt("page", opts.Page),
		ListTraceInt("page_size", opts.PageSize),
	)

	listing := newListing(rel, opts)
	if !opts.FullFilesystem {
		finishIndex := StartListTraceStep(ctx, "photos.library.index_listing", ListTraceString("path", rel))
		indexed, err := l.listFromIndex(ctx, rel, opts, &listing)
		finishIndex(
			ListTraceBool("indexed", indexed),
			ListTraceInt("folders", len(listing.Folders)),
			ListTraceInt("blogs", len(listing.Blogs)),
			ListTraceInt("media", len(listing.Media)),
			ListTraceInt("total", listing.Total),
		)
		if indexed || err != nil {
			if err != nil {
				return Listing{}, err
			}
			if opts.IncludeMapData {
				abs, err := l.Resolve(rel)
				if err != nil {
					return Listing{}, err
				}
				finishGPX := StartListTraceStep(ctx, "photos.library.index_gpx_tracks", ListTraceString("path", rel))
				if err := l.populateIndexedGPXTracks(ctx, abs, opts, &listing); err != nil {
					finishGPX(ListTraceString("error", err.Error()))
					return Listing{}, err
				}
				finishGPX(ListTraceInt("tracks", len(listing.GPXTracks)))
			}
			if err := l.finishListing(ctx, opts, &listing, indexedListingSource); err != nil {
				return Listing{}, err
			}
			return listing, nil
		}
	}

	finishResolve := StartListTraceStep(ctx, "photos.library.resolve_filesystem", ListTraceString("path", rel))
	abs, err := l.Resolve(rel)
	if err != nil {
		finishResolve(ListTraceString("error", err.Error()))
		return Listing{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		finishResolve(ListTraceString("error", err.Error()))
		return Listing{}, err
	}
	if !info.IsDir() {
		finishResolve(ListTraceString("error", "not_directory"))
		return Listing{}, fmt.Errorf("photo path is not a directory: %s", rel)
	}
	finishResolve()
	if !opts.IncludeAdminOnly {
		if l.index.available() {
			finishAdmin := StartListTraceStep(ctx, "photos.library.admin_visibility", ListTraceString("path", rel))
			adminOnly, known, err := l.indexFolderAdminOnly(ctx, rel)
			if err != nil {
				finishAdmin(ListTraceString("error", err.Error()))
				return Listing{}, err
			}
			if known && adminOnly {
				finishAdmin(ListTraceBool("known", known), ListTraceBool("admin_only", adminOnly))
				return Listing{}, errAdminOnly
			}
			if !known && rel != "" && l.directoryAdminOnlyFromAbs(rel, abs) {
				finishAdmin(ListTraceBool("known", known), ListTraceBool("admin_only", true))
				return Listing{}, errAdminOnly
			}
			finishAdmin(ListTraceBool("known", known), ListTraceBool("admin_only", adminOnly))
		} else if l.directoryAdminOnlyFromAbs(rel, abs) {
			return Listing{}, errAdminOnly
		}
	}

	fastFallback := l.index.available() && !opts.FullFilesystem && opts.Query == "" && !opts.Recursive && !opts.GPSOnly
	finishFilesystem := StartListTraceStep(ctx, "photos.library.filesystem_listing", ListTraceBool("fast", fastFallback))
	if err := l.listFromFilesystem(ctx, rel, abs, opts, &listing, fastFallback); err != nil {
		finishFilesystem(ListTraceString("error", err.Error()))
		return Listing{}, err
	}
	finishFilesystem(
		ListTraceInt("folders", len(listing.Folders)),
		ListTraceInt("blogs", len(listing.Blogs)),
		ListTraceInt("media", len(listing.Media)),
	)
	source := filesystemListingSource
	if fastFallback {
		source = fastFilesystemListingSource
	}
	if err := l.finishListing(ctx, opts, &listing, source); err != nil {
		return Listing{}, err
	}
	return listing, nil
}

type listingSourceState struct {
	adminFiltered                  bool
	mediaFiltered                  bool
	mediaSorted                    bool
	mediaPaged                     bool
	recursiveFolderPreviewFallback bool
	previewFilesystemFallback      bool
}

var (
	indexedListingSource = listingSourceState{
		adminFiltered:             true,
		mediaFiltered:             true,
		mediaSorted:               true,
		mediaPaged:                true,
		previewFilesystemFallback: false,
	}
	filesystemListingSource = listingSourceState{
		adminFiltered:                  true,
		recursiveFolderPreviewFallback: true,
		previewFilesystemFallback:      true,
	}
	fastFilesystemListingSource = listingSourceState{
		adminFiltered:             true,
		mediaFiltered:             true,
		mediaSorted:               true,
		mediaPaged:                true,
		previewFilesystemFallback: true,
	}
)

func newListing(rel string, opts ListOptions) Listing {
	return Listing{
		Path:        rel,
		ParentPath:  parentPath(rel),
		Breadcrumbs: breadcrumbs(rel),
		Query:       opts.Query,
		MediaType:   opts.MediaType,
		GPSOnly:     opts.GPSOnly,
		Sort:        opts.Sort,
		Page:        opts.Page,
		PageSize:    opts.PageSize,
	}
}

func (l *Library) finishListing(ctx context.Context, opts ListOptions, listing *Listing, source listingSourceState) error {
	if !opts.IncludeAdminOnly && !source.adminFiltered {
		finishAdmin := StartListTraceStep(ctx, "photos.library.filter_admin_only")
		l.filterAdminOnlyListing(listing)
		finishAdmin(
			ListTraceInt("folders", len(listing.Folders)),
			ListTraceInt("media", len(listing.Media)),
			ListTraceInt("blogs", len(listing.Blogs)),
		)
	}
	if !opts.Recursive {
		finishPreviews := StartListTraceStep(ctx, "photos.library.folder_previews", ListTraceInt("folders", len(listing.Folders)), ListTraceInt("limit", opts.FolderPreviewSize))
		if err := l.populateFolderPreviews(ctx, listing.Folders, opts.FolderPreviewSize, opts.IncludeAdminOnly, source.recursiveFolderPreviewFallback, source.previewFilesystemFallback); err != nil {
			finishPreviews(ListTraceString("error", err.Error()))
			return err
		}
		finishPreviews(ListTraceInt("preview_items", folderPreviewItemCount(listing.Folders)))
	}
	finishDisplay := StartListTraceStep(ctx, "photos.library.display_names", ListTraceInt("folders", len(listing.Folders)), ListTraceInt("breadcrumbs", len(listing.Breadcrumbs)))
	decorateListingDisplay(listing)
	finishDisplay()
	finishFolderSort := StartListTraceStep(ctx, "photos.library.sort_folders", ListTraceInt("count", len(listing.Folders)), ListTraceString("order", listing.Order), ListTraceString("sort", opts.Sort))
	sortFolders(listing.Folders, listing.Order, opts.Sort)
	finishFolderSort()
	if !source.mediaFiltered && queryHasPerson(opts.Query) {
		if err := l.AddAutomaticFaces(ctx, listing.Media); err != nil {
			return err
		}
	}
	if !source.mediaFiltered {
		finishFilter := StartListTraceStep(ctx, "photos.library.filter_media", ListTraceInt("before", len(listing.Media)))
		listing.Media = filterMedia(listing.Media, opts)
		finishFilter(ListTraceInt("after", len(listing.Media)))
	}
	if !source.mediaSorted {
		finishSort := StartListTraceStep(ctx, "photos.library.sort_media", ListTraceInt("count", len(listing.Media)), ListTraceString("sort", opts.Sort))
		sortMedia(listing.Media, listing.Order, opts.Sort)
		finishSort()
	}
	if !opts.IncludeMapData {
		listing.GPXTracks = nil
		listing.RoutePoints = nil
	}
	if source.mediaPaged {
		if opts.IncludeMapData {
			finishMap := StartListTraceStep(ctx, "photos.library.route_points", ListTraceInt("media", len(listing.Media)), ListTraceInt("tracks", len(listing.GPXTracks)))
			sort.SliceStable(listing.GPXTracks, func(i, j int) bool {
				return strings.ToLower(listing.GPXTracks[i].Path) < strings.ToLower(listing.GPXTracks[j].Path)
			})
			decorateGPXTracks(listing.GPXTracks)
			listing.RoutePoints = routePointsFromMedia(listing.Media, opts.RouteClusterRadiusMeters)
			finishMap(ListTraceInt("route_points", len(listing.RoutePoints)))
		}
		listing.HasPrev = opts.Page > 1
		listing.HasNext = opts.Page*opts.PageSize < listing.Total
		return nil
	}
	listing.Total = len(listing.Media)
	if opts.IncludeMapData {
		finishMap := StartListTraceStep(ctx, "photos.library.route_points", ListTraceInt("media", len(listing.Media)), ListTraceInt("tracks", len(listing.GPXTracks)))
		listing.RoutePoints = routePointsFromMedia(listing.Media, opts.RouteClusterRadiusMeters)
		sort.SliceStable(listing.GPXTracks, func(i, j int) bool {
			return strings.ToLower(listing.GPXTracks[i].Path) < strings.ToLower(listing.GPXTracks[j].Path)
		})
		decorateGPXTracks(listing.GPXTracks)
		finishMap(ListTraceInt("route_points", len(listing.RoutePoints)))
	}
	finishPaginate := StartListTraceStep(ctx, "photos.library.paginate_media", ListTraceInt("total", len(listing.Media)), ListTraceInt("page", opts.Page), ListTraceInt("page_size", opts.PageSize))
	paginateListingMedia(listing, opts)
	finishPaginate(ListTraceInt("page_count", len(listing.Media)))
	return nil
}

func folderPreviewItemCount(folders []Folder) int {
	count := 0
	for _, folder := range folders {
		count += len(folder.Previews)
	}
	return count
}

func previewMapItemCount(previews map[string][]Media) int {
	count := 0
	for _, items := range previews {
		count += len(items)
	}
	return count
}

func paginateListingMedia(listing *Listing, opts ListOptions) {
	start := (opts.Page - 1) * opts.PageSize
	if start > len(listing.Media) {
		start = len(listing.Media)
	}
	end := start + opts.PageSize
	if end > len(listing.Media) {
		end = len(listing.Media)
	}
	listing.HasPrev = opts.Page > 1
	listing.HasNext = end < len(listing.Media)
	listing.Media = append([]Media(nil), listing.Media[start:end]...)
}

func (l *Library) populateIndexedGPXTracks(ctx context.Context, abs string, opts ListOptions, listing *Listing) error {
	if !opts.IncludeMapData {
		return nil
	}
	tracks, err := l.collectGPXTracks(ctx, abs, opts.Recursive, opts.IncludeAdminOnly)
	if err != nil {
		return err
	}
	listing.GPXTracks = append(listing.GPXTracks, tracks...)
	return nil
}

func (l *Library) collectGPXTracks(ctx context.Context, abs string, recursive, includeAdminOnly bool) ([]GPXTrack, error) {
	var tracks []GPXTrack
	addTrack := func(path string, info os.FileInfo) error {
		childRel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		track, err := l.gpxFromPathInfo(filepath.ToSlash(childRel), info)
		if err == nil && len(track.Points) > 0 {
			tracks = append(tracks, track)
		}
		return nil
	}
	if recursive {
		err := walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
			Root:             abs,
			IncludeAdminOnly: includeAdminOnly,
		}, func(path string, entry os.DirEntry, kind string) error {
			if kind != mediaKindGPX {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			return addTrack(path, info)
		})
		return tracks, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || ignoredName(entry.Name()) || !strings.EqualFold(filepath.Ext(entry.Name()), ".gpx") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if err := addTrack(filepath.Join(abs, entry.Name()), info); err != nil {
			return nil, err
		}
	}
	return tracks, nil
}

func (l *Library) Resolve(rel string) (string, error) {
	return photopath.Resolve(l.root, rel)
}

func CleanPath(value string) (string, error) {
	return photopath.Clean(value)
}

func filterMedia(items []Media, opts ListOptions) []Media {
	out := items[:0]
	for _, item := range items {
		if opts.MediaType != "" && item.Type != opts.MediaType {
			continue
		}
		if opts.GPSOnly && (item.Latitude == nil || item.Longitude == nil) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func directoryOrder(entries []os.DirEntry) string {
	for _, entry := range entries {
		name := entry.Name()
		if !isOrderFileName(name) {
			continue
		}
		order := strings.TrimSuffix(strings.TrimPrefix(name, ".order_"), ".pg2conf")
		switch order {
		case "descending_name", "ascending_name", "descending_date", "ascending_date", "random":
			return order
		}
	}
	return ""
}

func normalizeDirectoryOrderMode(order string) string {
	switch order {
	case "descending_name", "ascending_name", "descending_date", "ascending_date", "random":
		return order
	default:
		return "ascending_date"
	}
}

func (l *Library) directoryOrderForPath(rel string) string {
	abs, err := l.Resolve(rel)
	if err != nil {
		return "ascending_date"
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "ascending_date"
	}
	return normalizeDirectoryOrderMode(directoryOrder(entries))
}

func sortMedia(items []Media, folderOrder, requestSort string) {
	order := folderOrder
	if requestSort != "" {
		order = requestSort
	}
	switch order {
	case "ascending_name":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	case "descending_name":
		sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Name) > strings.ToLower(items[j].Name) })
	case "ascending_date":
		sort.SliceStable(items, func(i, j int) bool { return mediaDate(items[i]).Before(mediaDate(items[j])) })
	case "descending_date":
		sort.SliceStable(items, func(i, j int) bool { return mediaDate(items[i]).After(mediaDate(items[j])) })
	case "random":
		sort.SliceStable(items, func(i, j int) bool { return stableHash(items[i].Path) < stableHash(items[j].Path) })
	default:
		sort.SliceStable(items, func(i, j int) bool { return mediaDate(items[i]).Before(mediaDate(items[j])) })
	}
}

func sortFolders(items []Folder, folderOrder, requestSort string) {
	order := folderOrder
	if requestSort != "" {
		order = requestSort
	}
	switch order {
	case "ascending_name":
		sort.SliceStable(items, func(i, j int) bool { return folderNameLess(items[i], items[j], false) })
	case "descending_name":
		sort.SliceStable(items, func(i, j int) bool { return folderNameLess(items[i], items[j], true) })
	case "ascending_date":
		sort.SliceStable(items, func(i, j int) bool { return folderDateLess(items[i], items[j], false) })
	case "descending_date":
		sort.SliceStable(items, func(i, j int) bool { return folderDateLess(items[i], items[j], true) })
	case "random":
		sort.SliceStable(items, func(i, j int) bool {
			left, right := stableHash(items[i].Path), stableHash(items[j].Path)
			if left != right {
				return left < right
			}
			return items[i].Path < items[j].Path
		})
	default:
		sort.SliceStable(items, func(i, j int) bool { return folderDateLess(items[i], items[j], false) })
	}
}

func folderNameLess(left, right Folder, descending bool) bool {
	leftName := strings.ToLower(left.Name)
	rightName := strings.ToLower(right.Name)
	if leftName != rightName {
		if descending {
			return leftName > rightName
		}
		return leftName < rightName
	}
	if descending {
		return left.Path > right.Path
	}
	return left.Path < right.Path
}

func folderDateLess(left, right Folder, descending bool) bool {
	if left.DisplayDate != nil && right.DisplayDate != nil {
		if !left.DisplayDate.Equal(*right.DisplayDate) {
			if descending {
				return left.DisplayDate.After(*right.DisplayDate)
			}
			return left.DisplayDate.Before(*right.DisplayDate)
		}
		return folderNameLess(left, right, descending)
	}
	if left.DisplayDate != nil {
		return true
	}
	if right.DisplayDate != nil {
		return false
	}
	return folderNameLess(left, right, descending)
}

func stableHash(value string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return h.Sum64()
}

func stableHashKey(value string) string {
	return fmt.Sprintf("%020d", stableHash(value))
}

func mediaDate(item Media) time.Time {
	if item.CapturedAt != nil {
		return *item.CapturedAt
	}
	return item.ModTime
}

func ignoredName(name string) bool {
	if name == "." || name == ".." {
		return true
	}
	switch strings.ToLower(name) {
	case "$recycle.bin", "@eadir", "__macosx", "thumbs.db":
		return true
	}
	if strings.HasPrefix(name, ".") {
		return !strings.HasPrefix(name, ".order_")
	}
	return false
}

func joinPath(base, name string) string {
	if base == "" {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(base), name))
}

func parentPath(rel string) string {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return ""
	}
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if parent == "." {
		return ""
	}
	return parent
}

func breadcrumbs(rel string) []Crumb {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	crumbs := []Crumb{{Name: "Fotos", Path: "", DisplayName: "Fotos"}}
	if rel == "" {
		return crumbs
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		displayName, displayDate := folderNameDisplay(part)
		crumbs = append(crumbs, Crumb{
			Name:        part,
			Path:        strings.Join(parts[:i+1], "/"),
			DisplayName: displayName,
			DisplayDate: displayDate,
		})
	}
	return crumbs
}

func normalizeMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MediaTypeImage:
		return MediaTypeImage
	case MediaTypeVideo:
		return MediaTypeVideo
	case MediaTypeAudio:
		return MediaTypeAudio
	default:
		return ""
	}
}

func normalizeSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ascending_name", "descending_name", "ascending_date", "descending_date", "random":
		return value
	default:
		return ""
	}
}
