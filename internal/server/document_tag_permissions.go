// Datei berechnet Tag-Berechtigungen und Sichtbarkeit fuer Dokumentlisten und Formulare.
package server

import (
	"errors"
	"net/http"
	"strings"
)

func (s *Server) unknownDocumentTagsForRequest(r *http.Request, tags []string) ([]string, error) {
	if !s.authEnabled() || s.requestHasCapabilities(r, authCapDocumentsStructure) {
		return nil, nil
	}
	existingTags, err := s.repo.ListTags(r.Context())
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(existingTags))
	for _, tag := range existingTags {
		existing[strings.TrimSpace(strings.ToLower(tag.Name))] = struct{}{}
	}
	unknown := make([]string, 0)
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := existing[tag]; ok {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		unknown = append(unknown, tag)
	}
	return unknown, nil
}

func (s *Server) renderDocumentTagCreationForbidden(w http.ResponseWriter, r *http.Request, unknownTags []string) {
	message := "Neue Dokument-Tags koennen nur mit Struktur-Rechten angelegt werden."
	if len(unknownTags) > 0 {
		message += " Unbekannte Tags: " + strings.Join(unknownTags, ", ")
	}
	if wantsJSON(r) || wantsJSONResponse(r) {
		s.renderJSONError(w, http.StatusForbidden, message)
		return
	}
	returnURL := formReturnURL(r)
	if id, err := idFromRequest(r); err == nil {
		returnURL = documentURL(id, returnURL, "")
	}
	s.renderErrorWithReturn(w, r, http.StatusForbidden, errors.New(message), returnURL)
}
