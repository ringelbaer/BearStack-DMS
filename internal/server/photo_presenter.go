// Datei formatiert Foto-, Ordner- und Medienmodelle fuer Templates und API-Antworten.
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/photos"
	"bearstack/internal/textmeta"
)

type PhotoListingView struct {
	Path        string
	ParentPath  string
	Breadcrumbs []photos.Crumb
	Folders     []PhotoFolderView
	Media       []PhotoMediaView
	Blogs       []photos.BlogPost
	GPXTracks   []photos.GPXTrack
	RoutePoints []photos.RoutePoint
	Query       string
	MediaType   string
	GPSOnly     bool
	Sort        string
	Order       string
	Page        int
	PageSize    int
	Total       int
	HasPrev     bool
	HasNext     bool
}

type PhotoFolderView struct {
	photos.Folder
	URL             string
	MediaCountLabel string
	MediaCountTitle string
	Previews        []PhotoMediaView
}

type PhotoMediaView struct {
	photos.Media
	MediaURL        string
	ThumbURL        string
	ThumbReady      bool
	PreviewURL      string
	LargePreviewURL string
}

type PhotoMediaGroup struct {
	Key   string
	Label string
	Media []PhotoMediaView
}

func newPhotoListingView(ctx context.Context, library *photos.Library, listing photos.Listing, settings PhotoSettings) PhotoListingView {
	settings = normalizePhotoPresentationSettings(settings)
	view := PhotoListingView{
		Path:        listing.Path,
		ParentPath:  listing.ParentPath,
		Breadcrumbs: listing.Breadcrumbs,
		Blogs:       listing.Blogs,
		GPXTracks:   listing.GPXTracks,
		RoutePoints: listing.RoutePoints,
		Query:       listing.Query,
		MediaType:   listing.MediaType,
		GPSOnly:     listing.GPSOnly,
		Sort:        listing.Sort,
		Order:       listing.Order,
		Page:        listing.Page,
		PageSize:    listing.PageSize,
		Total:       listing.Total,
		HasPrev:     listing.HasPrev,
		HasNext:     listing.HasNext,
	}

	thumbnailGroups := map[int]*photoThumbnailReadyGroup{}
	finishFolders := photos.StartListTraceStep(ctx, "photos.presenter.folders", photos.ListTraceInt("folders", len(listing.Folders)))
	if len(listing.Folders) > 0 {
		view.Folders = make([]PhotoFolderView, len(listing.Folders))
	}
	folderPreviewTargets := 0
	for i, folder := range listing.Folders {
		viewFolder := PhotoFolderView{
			Folder:          folder,
			URL:             photoPageURL(url.Values{"path": {folder.Path}}),
			MediaCountLabel: photoFolderMediaCountLabel(folder),
			MediaCountTitle: photoFolderMediaCountTitle(folder),
		}
		if len(folder.Previews) > 0 {
			viewFolder.Previews = make([]PhotoMediaView, len(folder.Previews))
		}
		for j, item := range folder.Previews {
			preview := photoFolderPreviewView(item, settings)
			viewFolder.Previews[j] = preview
			if preview.ThumbURL != "" {
				addPhotoThumbnailReadyTarget(thumbnailGroups, item, &viewFolder.Previews[j], settings.FolderThumbnailSize)
				folderPreviewTargets++
			}
		}
		view.Folders[i] = viewFolder
	}
	finishFolders(photos.ListTraceInt("preview_targets", folderPreviewTargets))

	finishMedia := photos.StartListTraceStep(ctx, "photos.presenter.media", photos.ListTraceInt("media", len(listing.Media)))
	if len(listing.Media) > 0 {
		view.Media = make([]PhotoMediaView, len(listing.Media))
	}
	mediaThumbnailTargets := 0
	for i, item := range listing.Media {
		media := photoMediaView(item, settings)
		view.Media[i] = media
		if media.ThumbURL != "" {
			addPhotoThumbnailReadyTarget(thumbnailGroups, item, &view.Media[i], settings.ThumbnailSize)
			mediaThumbnailTargets++
		}
	}
	finishMedia(photos.ListTraceInt("thumbnail_targets", mediaThumbnailTargets))

	for size, group := range thumbnailGroups {
		finishReady := photos.StartListTraceStep(ctx, "photos.presenter.thumbnail_ready", photos.ListTraceInt("size", size), photos.ListTraceInt("items", len(group.media)))
		markPhotoThumbnailsReady(ctx, library, group.media, group.targets, size)
		finishReady()
	}
	return view
}

func photoFolderMediaCountLabel(folder photos.Folder) string {
	count := fmt.Sprintf("%d", folder.MediaCount)
	if folder.MediaCountApproximate {
		count += "+"
	}
	unit := "Medien"
	if folder.MediaCount == 1 {
		unit = "Medium"
	}
	label := count + " " + unit
	if folder.DirCount > 0 {
		label += " gesamt"
	}
	return label
}

func photoFolderMediaCountTitle(folder photos.Folder) string {
	if folder.DirCount == 0 {
		return photoFolderMediaCountLabel(folder)
	}
	count := fmt.Sprintf("%d", folder.MediaCount)
	if folder.MediaCountApproximate {
		count += "+"
	}
	unit := "Medien"
	if folder.MediaCount == 1 {
		unit = "Medium"
	}
	return count + " " + unit + " inklusive Unterordner"
}

func normalizePhotoPresentationSettings(settings PhotoSettings) PhotoSettings {
	defaults := defaultPhotoSettings()
	if settings.FolderThumbnailSize <= 0 {
		settings.FolderThumbnailSize = defaults.FolderThumbnailSize
	}
	if settings.ThumbnailSize <= 0 {
		settings.ThumbnailSize = defaults.ThumbnailSize
	}
	if settings.PreviewSize <= 0 {
		settings.PreviewSize = defaults.PreviewSize
	}
	if settings.LargePreviewSize <= 0 {
		settings.LargePreviewSize = defaults.LargePreviewSize
	}
	return settings
}

type photoThumbnailReadyGroup struct {
	media   []photos.Media
	targets []*PhotoMediaView
}

func addPhotoThumbnailReadyTarget(groups map[int]*photoThumbnailReadyGroup, media photos.Media, target *PhotoMediaView, size int) {
	group := groups[size]
	if group == nil {
		group = &photoThumbnailReadyGroup{}
		groups[size] = group
	}
	group.media = append(group.media, media)
	group.targets = append(group.targets, target)
}

func markPhotoThumbnailsReady(ctx context.Context, library *photos.Library, media []photos.Media, targets []*PhotoMediaView, size int) {
	if library == nil || len(media) == 0 || len(targets) == 0 {
		return
	}
	ready := library.CachedThumbnailsReadyForMediaContext(ctx, media, size)
	for _, item := range targets {
		item.ThumbReady = ready[item.Path]
	}
}

func photoMediaView(item photos.Media, settings PhotoSettings) PhotoMediaView {
	settings = normalizePhotoPresentationSettings(settings)
	view := PhotoMediaView{
		Media:    item,
		MediaURL: photoMediaURLVersioned(item.Path, item.ModTime),
	}
	if item.Type == photos.MediaTypeImage && photos.CanThumbnail(item.Path) {
		view.ThumbURL = photoThumbnailURLVersioned(item.Path, settings.ThumbnailSize, item.ModTime)
		view.PreviewURL = photoThumbnailURLVersioned(item.Path, settings.PreviewSize, item.ModTime)
		view.LargePreviewURL = photoThumbnailURLVersioned(item.Path, settings.LargePreviewSize, item.ModTime)
	} else if item.Type == photos.MediaTypeVideo && photos.CanThumbnail(item.Path) {
		view.ThumbURL = photoThumbnailURLVersioned(item.Path, settings.ThumbnailSize, item.ModTime)
	}
	return view
}

func photoFolderPreviewView(item photos.Media, settings PhotoSettings) PhotoMediaView {
	settings = normalizePhotoPresentationSettings(settings)
	view := PhotoMediaView{
		Media:    item,
		MediaURL: photoMediaURLVersioned(item.Path, item.ModTime),
	}
	if photos.CanThumbnail(item.Path) {
		view.ThumbURL = photoThumbnailURLVersioned(item.Path, settings.FolderThumbnailSize, item.ModTime)
	}
	return view
}

func photoMediaGroups(listing PhotoListingView) []PhotoMediaGroup {
	if len(listing.Media) == 0 {
		return nil
	}
	effectiveSort := listing.Sort
	if effectiveSort == "" {
		effectiveSort = listing.Order
	}
	if effectiveSort != "ascending_date" && effectiveSort != "descending_date" {
		return []PhotoMediaGroup{{Media: listing.Media}}
	}
	groups := make([]PhotoMediaGroup, 0)
	for _, item := range listing.Media {
		key, label := photoMediaGroupDate(item)
		if len(groups) == 0 || groups[len(groups)-1].Key != key {
			groups = append(groups, PhotoMediaGroup{Key: key, Label: label})
		}
		groups[len(groups)-1].Media = append(groups[len(groups)-1].Media, item)
	}
	return groups
}

func photoMediaGroupDate(item PhotoMediaView) (string, string) {
	date := item.ModTime
	if item.CapturedAt != nil {
		date = *item.CapturedAt
	}
	if date.IsZero() {
		return "unknown", "Ohne Datum"
	}
	local := date.Local()
	return local.Format("2006-01-02"), local.Format("02.01.2006")
}

type photoMediaAPIResponse struct {
	Path           string                  `json:"path"`
	Name           string                  `json:"name"`
	Title          string                  `json:"title"`
	Src            string                  `json:"src"`
	Original       string                  `json:"original"`
	Preview        string                  `json:"preview"`
	LargePreview   string                  `json:"large_preview"`
	Thumb          string                  `json:"thumb"`
	Type           string                  `json:"type"`
	Date           string                  `json:"date"`
	DateTime       string                  `json:"date_time"`
	Camera         string                  `json:"camera"`
	Lens           string                  `json:"lens"`
	Rating         string                  `json:"rating"`
	Size           string                  `json:"size"`
	Resolution     string                  `json:"resolution"`
	Coords         string                  `json:"coords"`
	Lat            string                  `json:"lat"`
	Lon            string                  `json:"lon"`
	ThumbReady     bool                    `json:"thumb_ready"`
	ThumbnailSize  int                     `json:"thumbnail_size,omitempty"`
	AutomaticFaces []photos.RecognizedFace `json:"automatic_faces,omitempty"`
	Faces          []photoFaceAPIResponse  `json:"faces,omitempty"`
}

type photoFaceAPIResponse struct {
	Name   string  `json:"name"`
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func photoMediaAPIResponsesFrom(media []PhotoMediaView) []photoMediaAPIResponse {
	responses := make([]photoMediaAPIResponse, len(media))
	for i, item := range media {
		responses[i] = photoMediaAPIResponseFrom(item)
	}
	return responses
}

func photoFrameMediaAPIResponsesFrom(media []PhotoMediaView) []photoMediaAPIResponse {
	responses := make([]photoMediaAPIResponse, len(media))
	for i, item := range media {
		response := photoMediaAPIResponseFrom(item)
		response.Title = cleanPhotoFrameTitle(item.Name)
		responses[i] = response
	}
	return responses
}

func cleanPhotoFrameTitle(name string) string {
	title, _ := textmeta.FromFilename(name)
	if title == "" {
		return name
	}
	return title
}

func photoMediaAPIResponseFrom(item PhotoMediaView) photoMediaAPIResponse {
	src := item.MediaURL
	if item.Type == photos.MediaTypeImage {
		src = firstNonEmpty(item.LargePreviewURL, item.PreviewURL, item.ThumbURL, item.MediaURL)
	}
	response := photoMediaAPIResponse{
		Path:           item.Path,
		Name:           item.Name,
		Title:          item.Name,
		Src:            src,
		Original:       item.MediaURL,
		Preview:        item.PreviewURL,
		LargePreview:   item.LargePreviewURL,
		Thumb:          item.ThumbURL,
		Type:           item.Type,
		Date:           formatDate(item.CapturedAt),
		DateTime:       formatPhotoDateTime(item.CapturedAt),
		Camera:         item.Camera,
		Lens:           item.Lens,
		Rating:         formatRatingData(item.Rating),
		Size:           formatBytes(item.SizeBytes),
		ThumbReady:     item.ThumbReady,
		Faces:          photoFaceAPIResponsesFrom(item.Faces),
		AutomaticFaces: item.AutomaticFaces,
	}
	if item.Width > 0 && item.Height > 0 {
		response.Resolution = fmt.Sprintf("%d × %d", item.Width, item.Height)
	}
	if item.Latitude != nil && item.Longitude != nil {
		response.Coords = formatCoord(item.Latitude) + ", " + formatCoord(item.Longitude)
	}
	response.Lat = formatCoord(item.Latitude)
	response.Lon = formatCoord(item.Longitude)
	response.ThumbnailSize = photoURLSize(item.ThumbURL)
	return response
}

func formatPhotoDateTime(capturedAt *time.Time) string {
	if capturedAt == nil {
		return "-"
	}
	return formatDateTime(*capturedAt)
}

func photoFaceAPIResponsesFrom(faces []photos.Face) []photoFaceAPIResponse {
	if len(faces) == 0 {
		return nil
	}
	responses := make([]photoFaceAPIResponse, 0, len(faces))
	for _, face := range faces {
		responses = append(responses, photoFaceAPIResponse{
			Name:   face.Name,
			Left:   face.X,
			Top:    face.Y,
			Width:  face.Width,
			Height: face.Height,
		})
	}
	return responses
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func photoURLSize(value string) int {
	parsed, err := url.Parse(value)
	if err != nil {
		return 0
	}
	return parsePositiveInt(parsed.Query().Get("size"), 0)
}

func photoFilterFromRequest(r *http.Request, listing photos.Listing) PhotoFilter {
	q := clearQueryKeys(r.URL.Query(), "notice")
	traceValue := photoTraceQueryValue(q)
	mapAvailable := photoMapAvailable(listing.Path)
	base := cleanedPhotoValues(q)
	if listing.Path != "" {
		base.Set("path", listing.Path)
	} else {
		base.Del("path")
	}
	sortOptions, sortLabel := photoSortOptions(base, listing.Sort)
	filter := PhotoFilter{
		Path:         listing.Path,
		Query:        listing.Query,
		MediaType:    listing.MediaType,
		GPSOnly:      listing.GPSOnly,
		Sort:         listing.Sort,
		SortLabel:    sortLabel,
		SortOptions:  sortOptions,
		MapView:      q.Get("view") == "map" && mapAvailable,
		MapAvailable: mapAvailable,
	}
	filter.RandomURL = pathWithQuery("/photos/random", base)
	filter.FrameURL = pathWithQuery("/photos/frame", base)
	mapValues := cloneQueryValues(base)
	mapValues.Set("view", "map")
	filter.MapURL = photoPageURL(mapValues)
	galleryValues := cloneQueryValues(base)
	galleryValues.Del("view")
	filter.GalleryURL = photoPageURL(galleryValues)
	clearValues := url.Values{}
	if listing.Path != "" {
		clearValues.Set("path", listing.Path)
	}
	if traceValue != "" {
		clearValues.Set("trace", traceValue)
	}
	filter.ClearURL = photoPageURL(clearValues)
	if listing.ParentPath != listing.Path {
		parent := url.Values{}
		if listing.ParentPath != "" {
			parent.Set("path", listing.ParentPath)
		}
		if traceValue != "" {
			parent.Set("trace", traceValue)
		}
		filter.ParentURL = photoPageURL(parent)
	}
	if listing.HasPrev {
		prev := cloneQueryValues(base)
		if listing.Page > 2 {
			prev.Set("page", strconv.Itoa(listing.Page-1))
		} else {
			prev.Del("page")
		}
		filter.PrevURL = photoPageURL(prev)
	}
	if listing.HasNext {
		next := cloneQueryValues(base)
		next.Set("page", strconv.Itoa(listing.Page+1))
		filter.NextURL = photoPageURL(next)
	}
	filter.PageLinks = photoPageLinks(base, listing.Page, pageCount(listing.Total, listing.PageSize))
	filter.MediaTypeAllURL = photoTypeURL(base, "")
	filter.MediaTypeImageURL = photoTypeURL(base, photos.MediaTypeImage)
	filter.MediaTypeVideoURL = photoTypeURL(base, photos.MediaTypeVideo)
	filter.MediaTypeAudioURL = photoTypeURL(base, photos.MediaTypeAudio)
	gps := cloneQueryValues(base)
	if listing.GPSOnly {
		gps.Del("gps")
	} else {
		gps.Set("gps", "1")
	}
	gps.Del("page")
	filter.GPSURL = photoPageURL(gps)
	return filter
}

func photoPageLinks(base url.Values, currentPage, totalPages int) []PhotoPageLink {
	if totalPages < 4 || currentPage < 1 {
		return nil
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}
	pages := compactPhotoPages(currentPage, totalPages)
	links := make([]PhotoPageLink, 0, len(pages))
	last := 0
	for _, page := range pages {
		if last > 0 && page-last > 1 {
			links = append(links, PhotoPageLink{Ellipsis: true})
		}
		link := PhotoPageLink{
			Page:    page,
			Current: page == currentPage,
		}
		if !link.Current {
			link.URL = photoPageNumberURL(base, page)
		}
		links = append(links, link)
		last = page
	}
	return links
}

func compactPhotoPages(currentPage, totalPages int) []int {
	pages := []int{1}
	start := currentPage - 1
	if start < 2 {
		start = 2
	}
	end := currentPage + 1
	if end > totalPages-1 {
		end = totalPages - 1
	}
	for page := start; page <= end; page++ {
		pages = append(pages, page)
	}
	if totalPages > 1 {
		pages = append(pages, totalPages)
	}
	return pages
}

func photoPageNumberURL(base url.Values, page int) string {
	values := cloneQueryValues(base)
	if page <= 1 {
		values.Del("page")
	} else {
		values.Set("page", strconv.Itoa(page))
	}
	return photoPageURL(values)
}

func photoSortOptions(base url.Values, activeSort string) ([]PhotoSortOption, string) {
	definitions := []PhotoSortOption{
		{Value: "", Label: "Ordnerstandard"},
		{Value: "ascending_date", Label: "Datum aufsteigend"},
		{Value: "descending_date", Label: "Datum absteigend"},
		{Value: "ascending_name", Label: "Name aufsteigend"},
		{Value: "descending_name", Label: "Name absteigend"},
		{Value: "random", Label: "Zufällig"},
	}
	options := make([]PhotoSortOption, 0, len(definitions))
	activeLabel := "Sortieren"
	for _, definition := range definitions {
		values := cloneQueryValues(base)
		values.Set("sort", definition.Value)
		values.Del("page")
		option := definition
		option.URL = photoPageURL(values)
		option.Active = definition.Value == activeSort
		if option.Active {
			activeLabel = option.Label
		}
		options = append(options, option)
	}
	return options, activeLabel
}

func photoMapAvailable(path string) bool {
	return photoFolderDepth(path) >= 2
}

func photoFolderDepth(path string) int {
	path = strings.Trim(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"), "/")
	if path == "" {
		return 0
	}
	depth := 0
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			depth++
		}
	}
	return depth
}

func cleanedPhotoValues(values url.Values) url.Values {
	out := cloneQueryValues(values)
	out.Del("page")
	out.Del("view")
	for key, value := range out {
		if len(value) == 0 || strings.TrimSpace(value[0]) == "" {
			out.Del(key)
		}
	}
	return out
}

func photoTypeURL(base url.Values, mediaType string) string {
	values := cloneQueryValues(base)
	if mediaType == "" {
		values.Del("type")
	} else {
		values.Set("type", mediaType)
	}
	values.Del("page")
	return photoPageURL(values)
}

func photoPageURL(values url.Values) string {
	return pathWithQuery("/photos", values)
}

func photoMediaURL(path string) string {
	return "/photos/media?path=" + url.QueryEscape(path)
}

func photoMediaURLVersioned(path string, modTime time.Time) string {
	return appendPhotoURLVersion(photoMediaURL(path), modTime)
}

func photoThumbnailURL(path string, size int) string {
	return "/photos/thumbnail?path=" + url.QueryEscape(path) + "&size=" + strconv.Itoa(size)
}

func photoThumbnailURLVersioned(path string, size int, modTime time.Time) string {
	return appendPhotoURLVersion(photoThumbnailURL(path, size), modTime)
}

func appendPhotoURLVersion(base string, modTime time.Time) string {
	if modTime.IsZero() {
		return base
	}
	version := modTime.UTC().UnixNano()
	if version <= 0 {
		return base
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + "v=" + strconv.FormatInt(version, 10)
}

func setPhotoMediaCacheControl(w http.ResponseWriter, r *http.Request, fallbackMaxAge int) {
	if strings.TrimSpace(r.URL.Query().Get("v")) != "" {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(fallbackMaxAge))
}
