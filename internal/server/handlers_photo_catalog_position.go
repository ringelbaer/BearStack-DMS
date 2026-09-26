package server

import (
	"errors"
	"net/http"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoCatalogPosition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	path := r.URL.Query().Get("path")
	access := photoAccessPolicy{library: s.photos, allowAdminOnly: false}
	if err := access.RequireMedia(path); err != nil {
		s.catalogError(w, err)
		return
	}
	out, err := s.photos.CatalogFolderPosition(r.Context(), path)
	if errors.Is(err, photos.ErrCatalogIndexUnavailable) {
		w.Header().Set("Retry-After", "10")
		_ = writeJSON(w, 503, map[string]string{"code": "catalog_index_not_ready"})
		return
	}
	if err == nil {
		err = access.RequireMedia(out.Path)
	}
	if err != nil {
		s.catalogError(w, err)
		return
	}
	_ = writeJSON(w, 200, out)
}
