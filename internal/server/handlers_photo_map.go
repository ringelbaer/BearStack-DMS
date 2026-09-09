package server

import (
	"errors"
	"net/http"
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
	bounds := photos.MapBounds{South: -90, West: -180, North: 90, East: 180}
	count := 0
	for name, target := range map[string]*float64{"south": &bounds.South, "west": &bounds.West, "north": &bounds.North, "east": &bounds.East} {
		if !q.Has(name) {
			continue
		}
		value, err := strconv.ParseFloat(q.Get(name), 64)
		if err != nil {
			_ = writeJSON(w, 400, map[string]string{"code": "invalid_bounds"})
			return
		}
		*target = value
		count++
	}
	if (count != 0 && count != 4) || !bounds.Valid() {
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
