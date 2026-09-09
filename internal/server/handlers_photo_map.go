package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoCatalogMap(w http.ResponseWriter, r *http.Request) {
	s.handlePhotoMap(w, r, false)
}

func (s *Server) handlePhotoMapMedia(w http.ResponseWriter, r *http.Request) {
	s.handlePhotoMap(w, r, true)
}

func (s *Server) handlePhotoMap(w http.ResponseWriter, r *http.Request, media bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query()
	bounds, count, valid := photoMapBounds(q)
	if !valid {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_bounds"})
		return
	}
	if len(q.Get("q")) > 800 {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_query"})
		return
	}
	kind := q.Get("type")
	if kind != "" && kind != photos.MediaTypeImage && kind != photos.MediaTypeVideo && kind != photos.MediaTypeAudio {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_type"})
		return
	}
	opts := photos.ListOptions{Path: q.Get("path"), Query: q.Get("q"), MediaType: kind, Page: 1}
	if media {
		if count != 4 {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_bounds"})
			return
		}
		if q.Has("page") {
			page, err := strconv.Atoi(q.Get("page"))
			if err != nil || page < 1 || page > 1000000 {
				_ = writeJSON(w, 400, map[string]string{"code": "invalid_page"})
				return
			}
			opts.Page = page
		}
	}
	var result any
	var err error
	if media {
		var items []photos.Media
		var total int
		items, total, err = s.photos.MapMedia(r.Context(), opts, bounds)
		out := []photoCatalogMedia{}
		for _, item := range items {
			out = append(out, catalogMedia(item))
		}
		result = map[string]any{"media": out, "total": total, "page": opts.Page, "has_next": opts.Page*96 < total}
	} else {
		result, err = s.photos.Map(r.Context(), opts, bounds)
	}
	if errors.Is(err, photos.ErrMapIndexUnavailable) {
		w.Header().Set("Retry-After", "10")
		_ = writeJSON(w, 503, map[string]string{"code": "map_index_not_ready"})
		return
	}
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, result)
}

// Shared validation for marker, selection and track-geometry viewports.
func photoMapBounds(q url.Values) (photos.MapBounds, int, bool) {
	bounds := photos.MapBounds{South: -90, West: -180, North: 90, East: 180}
	count := 0
	for name, target := range map[string]*float64{"south": &bounds.South, "west": &bounds.West, "north": &bounds.North, "east": &bounds.East} {
		if !q.Has(name) {
			continue
		}
		value, err := strconv.ParseFloat(q.Get(name), 64)
		if err != nil {
			return bounds, count, false
		}
		*target = value
		count++
	}
	return bounds, count, (count == 0 || count == 4) && bounds.Valid()
}

func (s *Server) handlePhotoMapTracks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query()
	if len(q.Get("path")) > 4096 {
		s.photoMapTrackError(w, photos.ErrPathEscapesRoot())
		return
	}
	var result photos.GPXFilePage
	var err error
	switch q.Get("before") {
	case "", "0":
		result, err = s.photos.GPXFiles(r.Context(), q.Get("path"), q.Get("cursor"), false)
	case "1":
		result, err = s.photos.GPXFilesBefore(r.Context(), q.Get("path"), q.Get("cursor"), false)
	default:
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_cursor"})
		return
	}
	if err != nil {
		s.photoMapTrackError(w, err)
		return
	}
	_ = writeJSON(w, 200, result)
}

func (s *Server) handlePhotoMapTrack(w http.ResponseWriter, r *http.Request) {
	s.handlePhotoMapGeometry(w, r, false)
}

func (s *Server) handlePhotoMapRoute(w http.ResponseWriter, r *http.Request) {
	s.handlePhotoMapGeometry(w, r, true)
}

func (s *Server) handlePhotoMapGeometry(w http.ResponseWriter, r *http.Request, route bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query()
	bounds, _, valid := photoMapBounds(q)
	if !valid {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_bounds"})
		return
	}
	points := 2048
	if q.Has("points") {
		value, err := strconv.Atoi(q.Get("points"))
		if err != nil || value < 32 || value > 8192 {
			s.photoMapTrackError(w, photos.ErrMapPointLimit)
			return
		}
		points = value
	}
	if len(q.Get("path")) > 4096 {
		s.photoMapTrackError(w, photos.ErrPathEscapesRoot())
		return
	}
	var result any
	var err error
	if route {
		kind := q.Get("type")
		if len(q.Get("q")) > 800 {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_query"})
			return
		}
		if kind != "" && kind != photos.MediaTypeImage && kind != photos.MediaTypeVideo && kind != photos.MediaTypeAudio {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_type"})
			return
		}
		settings, e := s.photoSettings(r.Context())
		if e != nil {
			s.catalogError(w, e)
			return
		}
		result, err = s.photos.MapPhotoRoute(r.Context(), photos.ListOptions{Path: q.Get("path"), Query: q.Get("q"), MediaType: kind,
			RouteClusterRadiusMeters: settings.MapTrackResolutionMeters}, bounds, points)
	} else {
		result, err = s.photos.GPXGeometry(r.Context(), q.Get("path"), bounds, points, false)
	}
	if err != nil {
		s.photoMapTrackError(w, err)
		return
	}
	_ = writeJSON(w, 200, result)
}

func (s *Server) photoMapTrackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, photos.ErrMapCursor):
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_cursor"})
	case errors.Is(err, photos.ErrMapPointLimit):
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_points"})
	case errors.Is(err, photos.ErrGPXTooLarge()):
		_ = writeJSON(w, 422, map[string]string{"code": "gpx_too_large"})
	case errors.Is(err, photos.ErrGPXInvalid):
		_ = writeJSON(w, 422, map[string]string{"code": "invalid_gpx"})
	case errors.Is(err, photos.ErrMapIndexUnavailable):
		w.Header().Set("Retry-After", "10")
		_ = writeJSON(w, 503, map[string]string{"code": "map_index_not_ready"})
	default:
		s.catalogError(w, err)
	}
}
