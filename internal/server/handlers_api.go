// Datei enthaelt API-Handler fuer programmatische Zugriffe abseits der klassischen HTML-Seiten.
package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/document"
)

func (s *Server) handleAPIFields(w http.ResponseWriter, r *http.Request) {
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Fields []customFieldAPIResponse `json:"fields"`
	}{Fields: customFieldAPIResponsesFrom(fields)})
}

func (s *Server) handleAPIDocuments(w http.ResponseWriter, r *http.Request) {
	perPage, err := s.apiDocumentPageSize(r.Context(), r)
	if err != nil {
		s.renderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter := filterFromRequest(r, apiTrashFromRequest(r), perPage)

	total, err := s.countDocuments(r.Context(), filter)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	docs, err := s.repo.ListDocuments(r.Context(), filter)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	documents, err := s.documentAPIResponses(r.Context(), docs)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Documents  []documentAPIResponse       `json:"documents"`
		Filter     documentAPIFilterResponse   `json:"filter"`
		Pagination documentAPIPaginationResult `json:"pagination"`
	}{
		Documents:  documents,
		Filter:     documentAPIFilterResponseFrom(filter),
		Pagination: documentAPIPaginationFrom(filter, len(docs), total),
	})
}

func (s *Server) handleAPIFolders(w http.ResponseWriter, r *http.Request) {
	perPage, err := s.apiDocumentPageSize(r.Context(), r)
	if err != nil {
		s.renderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	rawPath := folderPathValues(r.URL.Query()["tags"])
	if len(rawPath) > 0 && isSearchFavoritesFolderName(rawPath[0]) {
		s.handleAPISearchFavoriteFolders(w, r, rawPath, perPage)
		return
	}

	selection, err := virtualFolderSelectionFromRequest(r)
	if err != nil {
		s.renderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if selection.HasCustomFieldValues() {
		fields, err := s.repo.ListCustomFields(r.Context())
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		selection = selection.WithFieldLabels(fields)
	}
	selectedTags := selection.Tags()
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	filter := document.ListFilter{
		Sort:      document.ListSortDate,
		Direction: document.ListDirectionDescending,
		Limit:     perPage,
		Offset:    (page - 1) * perPage,
		Page:      page,
	}
	filter = selection.ApplyToFilter(filter)

	folders, err := s.folderService().Items(r.Context(), selection, filter)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var docs []document.Document
	total := 0
	if selection.Depth() > 0 {
		total, err = s.countDocuments(r.Context(), filter)
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		docs, err = s.repo.ListDocuments(r.Context(), filter)
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	documents, err := s.documentAPIResponses(r.Context(), docs)
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	folderResponses := folderAPIResponsesFromViewItems(folders, s.requestTagDisplayMode(r))

	writeJSON(w, http.StatusOK, struct {
		SelectedTags []string                    `json:"selected_tags"`
		SelectedPath []folderPathAPIResponse     `json:"selected_path"`
		Folders      []tagAPIResponse            `json:"folders"`
		Documents    []documentAPIResponse       `json:"documents"`
		Pagination   documentAPIPaginationResult `json:"pagination"`
	}{
		SelectedTags: selectedTags,
		SelectedPath: folderPathAPIResponsesFrom(selection),
		Folders:      folderResponses,
		Documents:    documents,
		Pagination:   documentAPIPaginationFrom(filter, len(docs), total),
	})
}

func (s *Server) handleAPISearchFavoriteFolders(w http.ResponseWriter, r *http.Request, selectedPath []string, perPage int) {
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	switch len(selectedPath) {
	case 1:
		folders, err := s.folderService().SearchFavoriteItems(r.Context(), time.Now())
		if errors.Is(err, sql.ErrNoRows) {
			s.renderJSONError(w, http.StatusNotFound, "Suchfavoriten nicht gefunden")
			return
		}
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		filter := document.ListFilter{Limit: perPage, Page: page}
		writeJSON(w, http.StatusOK, struct {
			SelectedTags []string                    `json:"selected_tags"`
			Folders      []tagAPIResponse            `json:"folders"`
			Documents    []documentAPIResponse       `json:"documents"`
			Pagination   documentAPIPaginationResult `json:"pagination"`
		}{
			SelectedTags: selectedPath,
			Folders:      folderAPIResponsesFromViewItems(folders, s.requestTagDisplayMode(r)),
			Documents:    []documentAPIResponse{},
			Pagination:   documentAPIPaginationFrom(filter, 0, 0),
		})
	case 2:
		filter, err := s.folderService().SearchFavoriteFilter(r.Context(), selectedPath[1], time.Now(), page, perPage)
		if errors.Is(err, sql.ErrNoRows) {
			s.renderJSONError(w, http.StatusNotFound, "Suchfavorit nicht gefunden")
			return
		}
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, err := s.countDocuments(r.Context(), filter)
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		docs, err := s.repo.ListDocuments(r.Context(), filter)
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		documents, err := s.documentAPIResponses(r.Context(), docs)
		if err != nil {
			s.renderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, struct {
			SelectedTags []string                    `json:"selected_tags"`
			Folders      []tagAPIResponse            `json:"folders"`
			Documents    []documentAPIResponse       `json:"documents"`
			Pagination   documentAPIPaginationResult `json:"pagination"`
		}{
			SelectedTags: selectedPath,
			Folders:      []tagAPIResponse{},
			Documents:    documents,
			Pagination:   documentAPIPaginationFrom(filter, len(docs), total),
		})
	default:
		s.renderJSONError(w, http.StatusNotFound, "Ordner nicht gefunden")
	}
}

func (s *Server) documentAPIResponses(ctx context.Context, docs []document.Document) ([]documentAPIResponse, error) {
	if len(docs) == 0 {
		return []documentAPIResponse{}, nil
	}
	fields, err := s.repo.ListCustomFields(ctx)
	if err != nil {
		return nil, err
	}
	return documentAPIResponsesFrom(docs, fields), nil
}

func folderAPIResponsesFromViewItems(items []folderViewItem, displayMode ...string) []tagAPIResponse {
	responses := make([]tagAPIResponse, len(items))
	for i, item := range items {
		switch item.Kind {
		case folderViewKindFieldValue:
			responses[i] = tagAPIResponse{
				ID:         item.FieldID,
				Name:       item.Name,
				Kind:       folderViewKindFieldValue,
				FieldID:    item.FieldID,
				FieldLabel: item.FieldLabel,
				Value:      item.Value,
				Count:      item.Count,
			}
		case folderViewKindSearchFavoritesRoot:
			responses[i] = tagAPIResponse{
				Name:  item.Name,
				Kind:  folderViewKindSearchFavoritesRoot,
				Count: item.Count,
			}
		case folderViewKindSearchFavorite:
			responses[i] = tagAPIResponse{
				ID:         item.FavoriteID,
				Name:       item.Name,
				Kind:       folderViewKindSearchFavorite,
				FavoriteID: item.FavoriteID,
				Count:      item.Count,
			}
		default:
			responses[i] = tagAPIResponseFrom(item.Tag, displayMode...)
		}
	}
	return responses
}

func (s *Server) apiDocumentPageSize(ctx context.Context, r *http.Request) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get("page_size"))
	if value == "" {
		return s.documentPageSize(ctx)
	}
	size, ok := documentPageSizeFromString(value)
	if !ok {
		return 0, errors.New("ungültige Anzahl pro Seite")
	}
	return size, nil
}

func apiTrashFromRequest(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("trash"))) {
	case "1", "true", "yes", "ja":
		return true
	default:
		return false
	}
}
