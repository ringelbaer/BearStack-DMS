package server

import (
	"errors"
	"net/http"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoCatalogDate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	// Native browse always excludes admin-only media, including for admin accounts.
	out, err := s.photos.CatalogDate(r.Context(), r.URL.Query().Get("date"), false)
	if errors.Is(err, photos.ErrCatalogDate) {
		_ = writeJSON(w, 400, map[string]string{"code": "invalid_date"})
		return
	}
	if errors.Is(err, photos.ErrCatalogIndexUnavailable) {
		w.Header().Set("Retry-After", "10")
		_ = writeJSON(w, 503, map[string]string{"code": "catalog_index_not_ready"})
		return
	}
	if err == nil && out.Path != "" {
		// Recheck current filesystem access before exposing even a date or position.
		err = (photoAccessPolicy{library: s.photos, allowAdminOnly: false}).RequireMedia(out.Path)
	}
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, out)
}
