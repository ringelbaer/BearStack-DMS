// Datei behandelt Foto-Tag-Aktionen und verbindet sie mit Fotoindex und HTTP-Antworten.
package server

import (
	"net/http"

	"bearstack/internal/document"
	"bearstack/internal/photos"
)

func (s *Server) listPhotoTagViews(r *http.Request) ([]document.Tag, error) {
	if s == nil || s.photos == nil {
		return nil, nil
	}
	tags, err := s.photos.ListTags(r.Context(), s.requestPhotoAdminOnlyVisible(r))
	if err != nil {
		return nil, err
	}
	return photoTagViews(tags), nil
}

func photoTagViews(tags []photos.Tag) []document.Tag {
	items := make([]document.Tag, 0, len(tags))
	for _, tag := range tags {
		items = append(items, document.Tag{
			Name:  tag.Name,
			Color: tag.Color,
			Count: tag.Count,
		})
	}
	return items
}
