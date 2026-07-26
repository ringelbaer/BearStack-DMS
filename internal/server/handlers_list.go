// Datei rendert Dokumentlisten mit Filtern, Suche, Sortierung und Paginierung.
package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"bearstack"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "api.html", PageData{
		Title:  "API",
		Active: "api",
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="openapi.yaml"`)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, bearstack.OpenAPISpec())
}

type documentListPageOptions struct {
	template      string
	title         string
	active        string
	trash         bool
	loadColumns   bool
	loadDateYears bool
	loadFavorites bool
	showUpload    bool
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	page, err := s.resolvedHomePage(r.Context(), authPermissionsForRequest(s, r))
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	redirect(w, r, homeRedirectURL(r, homePageURL(page)))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.photos != nil &&
		!s.requestHasCapabilities(r, authCapDocumentsRead) &&
		s.requestHasCapabilities(r, authCapPhotosRead) {
		redirect(w, r, "/photos")
		return
	}
	s.renderDocumentListPage(w, r, documentListPageOptions{
		template:      "index.html",
		title:         "Dokumente",
		active:        "documents",
		loadColumns:   true,
		loadDateYears: true,
		loadFavorites: true,
		showUpload:    true,
	})
}

func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	s.renderDocumentListPage(w, r, documentListPageOptions{
		template: "trash.html",
		title:    "Papierkorb",
		active:   "trash",
		trash:    true,
	})
}

func (s *Server) renderDocumentListPage(w http.ResponseWriter, r *http.Request, options documentListPageOptions) {
	result, err := s.documentListPageBuilder().Build(r.Context(), r, options)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if result.redirectURL != "" {
		redirect(w, r, result.redirectURL)
		return
	}
	if result.partial {
		s.renderPartial(w, r, "document_table", result.data)
		return
	}

	s.render(w, r, options.template, result.data)
}

func homeRedirectURL(r *http.Request, target string) string {
	if r == nil || r.URL == nil || r.URL.RawQuery == "" {
		return target
	}
	return target + "?" + r.URL.RawQuery
}

func wantsDocumentListPartial(r *http.Request) bool {
	return r.Header.Get("X-BearStack-Partial") == "document-list"
}

func (s *Server) handleSavePageSize(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	size, ok := documentPageSizeFromString(r.FormValue("page_size"))
	if !ok {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Anzahl pro Seite"), formReturnURL(r))
		return
	}
	if err := s.repo.SaveSetting(r.Context(), documentPageSizeSettingKey, strconv.Itoa(size)); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	redirect(w, r, formReturnURL(r))
}

func (s *Server) documentPageSize(ctx context.Context) (int, error) {
	value, ok, err := s.repo.GetSetting(ctx, documentPageSizeSettingKey)
	if err != nil {
		return 0, err
	}
	if !ok {
		return defaultDocumentPageSize, nil
	}
	if size, ok := documentPageSizeFromString(value); ok {
		return size, nil
	}
	return defaultDocumentPageSize, nil
}

func documentPageRedirectURL(r *http.Request, page, perPage, total int) string {
	if total <= 0 || page <= 1 {
		return ""
	}
	lastPage := (total + perPage - 1) / perPage
	if page <= lastPage {
		return ""
	}
	return pageURL(r, lastPage)
}

func (s *Server) handleDuplicates(w http.ResponseWriter, r *http.Request) {
	groups, err := s.repo.DuplicateGroups(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "duplicates.html", PageData{
		Title:      "Duplikate",
		Active:     "duplicates",
		Assets:     documentPageAssets(),
		Duplicates: groups,
		Notice:     r.URL.Query().Get("notice"),
	})
}
