// Datei verarbeitet Foto-Tag-API und Bulk-Tag-Aktionen.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoTags(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	path := r.URL.Query().Get("path")
	tags := normalizeTagValues(r.Form["tags"], "")
	saved, err := s.photoService().SetTags(r.Context(), s.requestIsPhotoAdmin(r), kind, path, tags)
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return
	}
	if err != nil {
		s.renderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.invalidatePhotoStatisticsCache()
	setAuditTarget(r, kind+":"+path)
	if err := writeJSON(w, http.StatusOK, struct {
		Tags []string `json:"tags"`
	}{Tags: saved}); err != nil {
		s.log.Warn("photo tag response failed", "kind", kind, "path", path, "error", err)
	}
}

func (s *Server) handlePhotoTagOptions(w http.ResponseWriter, r *http.Request) {
	tags, err := s.listPhotoTagViews(r)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Tags []tagAPIResponse `json:"tags"`
	}{Tags: tagAPIResponsesFrom(tags, s.requestTagDisplayMode(r))}); err != nil {
		s.log.Warn("photo tag options response failed", "error", err)
	}
}

func (s *Server) handleAddPhotoTags(w http.ResponseWriter, r *http.Request) {
	s.handleBulkPhotoTags(w, r, true)
}

func (s *Server) handleRemovePhotoTags(w http.ResponseWriter, r *http.Request) {
	s.handleBulkPhotoTags(w, r, false)
}

func (s *Server) handleBulkPhotoTags(w http.ResponseWriter, r *http.Request, add bool) {
	if err := r.ParseForm(); err != nil {
		s.renderPhotoBulkError(w, r, http.StatusBadRequest, err)
		return
	}
	paths := uniquePhotoPaths(r.Form["ids"])
	if len(paths) == 0 {
		s.renderPhotoBulkError(w, r, http.StatusBadRequest, errors.New("mindestens ein Bild auswählen"))
		return
	}
	tags := normalizeTagValues(r.Form["tags"], "")
	if len(tags) == 0 {
		s.renderPhotoBulkError(w, r, http.StatusBadRequest, errors.New("mindestens einen Tag auswählen"))
		return
	}

	updated, err := s.photoService().UpdateMediaTags(r.Context(), s.requestIsPhotoAdmin(r), paths, tags, add)
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return
	}
	if err != nil {
		s.renderPhotoBulkError(w, r, http.StatusBadRequest, err)
		return
	}
	if updated > 0 {
		s.invalidatePhotoStatisticsCache()
	}
	setAuditTarget(r, strings.Join(paths, ", "))
	if wantsJSONResponse(r) {
		if err := writeJSON(w, http.StatusOK, struct {
			Updated int      `json:"updated"`
			Tags    []string `json:"tags"`
		}{Updated: updated, Tags: tags}); err != nil {
			s.log.Warn("photo bulk tag response failed", "updated", updated, "error", err)
		}
		return
	}
	redirectWithNotice(w, r, formReturnURL(r), fmt.Sprintf("%d Bild(er) aktualisiert.", updated))
}

func uniquePhotoPaths(values []string) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			path := strings.TrimSpace(part)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}
