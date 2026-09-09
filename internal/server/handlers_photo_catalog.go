package server

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/photos"
)

// The native API carries source values rather than localized browser labels.
// Paths are library-relative; clients build URLs under their configured base.
type photoCatalogMedia struct {
	Path           string                  `json:"path"`
	Name           string                  `json:"name"`
	Type           string                  `json:"type"`
	MIME           string                  `json:"mime"`
	Version        string                  `json:"version"`
	Modified       time.Time               `json:"modified"`
	Captured       *time.Time              `json:"captured,omitempty"`
	Bytes          int64                   `json:"bytes"`
	Width          int                     `json:"width"`
	Height         int                     `json:"height"`
	Camera         string                  `json:"camera,omitempty"`
	Lens           string                  `json:"lens,omitempty"`
	Latitude       *float64                `json:"latitude,omitempty"`
	Longitude      *float64                `json:"longitude,omitempty"`
	Rating         *float64                `json:"rating,omitempty"`
	Tags           []string                `json:"tags,omitempty"`
	Keywords       []string                `json:"keywords,omitempty"`
	Faces          []photos.Face           `json:"faces,omitempty"`
	AutomaticFaces []photos.RecognizedFace `json:"automatic_faces,omitempty"`
}

func catalogMedia(media photos.Media) photoCatalogMedia {
	return photoCatalogMedia{Path: media.Path, Name: media.Name, Type: media.Type, MIME: media.MIMEType,
		Version: strconv.FormatInt(media.ModTime.UnixNano(), 10), Modified: media.ModTime, Captured: media.CapturedAt,
		Bytes: media.SizeBytes, Width: media.Width, Height: media.Height, Camera: media.Camera, Lens: media.Lens,
		Latitude: media.Latitude, Longitude: media.Longitude, Rating: media.Rating, Tags: media.Tags,
		Keywords: media.Keywords, Faces: media.Faces, AutomaticFaces: media.AutomaticFaces}
}

type photoCatalogFolder struct {
	Path        string              `json:"path"`
	Name        string              `json:"name"`
	Date        *time.Time          `json:"date,omitempty"`
	MediaCount  int                 `json:"media_count"`
	Approximate bool                `json:"approximate"`
	FolderCount int                 `json:"folder_count"`
	Previews    []photoCatalogMedia `json:"previews"`
}
type photoCatalogBlog struct {
	Path     string     `json:"path"`
	Name     string     `json:"name"`
	Date     *time.Time `json:"date,omitempty"`
	Modified time.Time  `json:"modified"`
	Text     string     `json:"text,omitempty"`
	HTML     string     `json:"html,omitempty"`
}

func catalogBlog(post photos.BlogPost) photoCatalogBlog {
	return photoCatalogBlog{post.Path, post.Name, post.Date, post.ModTime, post.Text, string(post.HTML)}
}

type photoCatalogPage struct {
	Path          string               `json:"path"`
	Parent        string               `json:"parent"`
	Page          int                  `json:"page"`
	Total         int                  `json:"total"`
	HasNext       bool                 `json:"has_next"`
	FolderTotal   int                  `json:"folder_total"`
	FolderHasNext bool                 `json:"folder_has_next"`
	BlogHasNext   bool                 `json:"blog_has_next"`
	Media         []photoCatalogMedia  `json:"media"`
	Folders       []photoCatalogFolder `json:"folders"`
	Blogs         []photoCatalogBlog   `json:"blogs"`
}

func (s *Server) catalogError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, photos.ErrPathEscapesRoot()):
		status, code = 400, "invalid_path"
	case errors.Is(err, photos.ErrSearchTooBroad()):
		status, code = 422, "search_too_broad"
	case errors.Is(err, photos.ErrAdminOnly()):
		status, code = 403, "forbidden"
	case errors.Is(err, os.ErrNotExist), errors.Is(err, sql.ErrNoRows):
		status, code = 404, "not_found"
	}
	_ = writeJSON(w, status, map[string]string{"code": code})
}

func (s *Server) handlePhotoCatalogSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	identity, err := s.photos.LabelSession(r.Context())
	if err != nil {
		s.catalogError(w, err)
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, map[string]any{
		"protocol": 1, "instance": identity.Instance, "dataset": identity.Dataset, "account": labelActor(r),
		"can_manage_people": s.requestHasCapabilities(r, authCapPhotosEdit),
		"settings": map[string]int{"thumbnail_size": settings.ThumbnailSize, "folder_thumbnail_size": settings.FolderThumbnailSize,
			"preview_size": settings.PreviewSize, "large_preview_size": settings.LargePreviewSize,
			"slideshow_seconds": settings.SlideshowSeconds, "frame_seconds": settings.FrameSeconds},
	})
}

func (s *Server) handlePhotoCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query()
	page := 1
	if q.Has("page") {
		value, err := strconv.Atoi(q.Get("page"))
		if err != nil || value < 1 || value > 1000000 {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_page"})
			return
		}
		page = value
	}
	section := q.Get("section")
	if section != "" && section != "media" && section != "folders" && section != "blogs" {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_section"})
		return
	}
	if len(q.Get("q")) > 800 {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_query"})
		return
	}
	sort := q.Get("sort")
	if sort == "" {
		sort = "descending_date"
	}
	if sort != "ascending_date" && sort != "descending_date" && sort != "ascending_name" && sort != "descending_name" {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_sort"})
		return
	}
	mediaType := q.Get("type")
	if mediaType != "" && mediaType != photos.MediaTypeImage && mediaType != photos.MediaTypeVideo && mediaType != photos.MediaTypeAudio {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_type"})
		return
	}
	for _, flag := range []string{"recursive", "gps"} {
		if value := q.Get(flag); value != "" && value != "0" && value != "1" {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_filter"})
			return
		}
	}
	opts := photos.ListOptions{Path: q.Get("path"), Query: strings.TrimSpace(q.Get("q")), Sort: sort, Page: page,
		MediaType: mediaType, Recursive: q.Get("recursive") == "1", GPSOnly: q.Get("gps") == "1",
		FolderPageSize: 24, BlogPageSize: 20, BlogSummaries: true,
		SkipFolders: section != "" && section != "folders", SkipBlogs: section != "" && section != "blogs", SkipMedia: section != "" && section != "media"}
	listing, _, err := s.photoService().Listing(r.Context(), photoListingRequest{Options: opts, PageSize: 96, FolderPreviews: 2, OmitPeople: true})
	if err != nil {
		s.catalogError(w, err)
		return
	}
	out := photoCatalogPage{Path: listing.Path, Parent: listing.ParentPath, Page: listing.Page, Total: listing.Total, HasNext: listing.HasNext,
		FolderTotal: listing.FolderTotal, FolderHasNext: listing.FolderHasNext, BlogHasNext: listing.BlogHasNext,
		Media: []photoCatalogMedia{}, Folders: []photoCatalogFolder{}, Blogs: []photoCatalogBlog{}}
	for _, item := range listing.Media {
		out.Media = append(out.Media, catalogMedia(item))
	}
	for _, folder := range listing.Folders {
		item := photoCatalogFolder{Path: folder.Path, Name: folder.DisplayName, Date: folder.DisplayDate,
			MediaCount: folder.MediaCount, Approximate: folder.MediaCountApproximate, FolderCount: folder.DirCount, Previews: []photoCatalogMedia{}}
		for _, preview := range folder.Previews {
			item.Previews = append(item.Previews, catalogMedia(preview))
		}
		out.Folders = append(out.Folders, item)
	}
	for _, post := range listing.Blogs {
		out.Blogs = append(out.Blogs, catalogBlog(post))
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handlePhotoCatalogInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	path := r.URL.Query().Get("path")
	if err := (photoAccessPolicy{library: s.photos, allowAdminOnly: s.requestIsPhotoAdmin(r)}).RequireMedia(path); err != nil {
		s.catalogError(w, err)
		return
	}
	media, err := s.photos.MediaContext(r.Context(), path)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, map[string]any{"media": catalogMedia(media)})
}

func (s *Server) handlePhotoCatalogBlog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	post, err := s.photos.Blog(r.Context(), r.URL.Query().Get("path"), s.requestIsPhotoAdmin(r))
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, map[string]any{"blog": catalogBlog(post)})
}
