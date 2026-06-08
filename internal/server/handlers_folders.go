// Datei rendert Ordneransichten und behandelt Navigation in der Dokumentablage.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/document"
)

const searchFavoritesFolderWebLabel = "Suchfavoriten"

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	rawPath := folderPathValues(r.URL.Query()["tags"])
	if len(rawPath) > 0 && isSearchFavoritesFolderName(rawPath[0]) {
		s.handleSearchFavoriteFolders(w, r, rawPath)
		return
	}

	selection, err := virtualFolderSelectionFromRequest(r)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err)
		return
	}
	perPage, err := s.documentPageSize(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	selection = selection.WithFieldLabels(fields)
	filter := folderDocumentFilterFromRequest(r, perPage)
	filter.Tags = nil
	filter = selection.ApplyToFilter(filter)
	allTags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	folderItems, err := s.folderService().ItemsWithRootTags(r.Context(), selection, filter, allTags)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	selectedTags := selection.Tags()
	data, err := s.folderPageData(r.Context(), r, allTags, selectedTags, virtualFolderBreadcrumb(r, selection), fields)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data.FolderTags = folderTagsFromViewItems(r, folderItems)
	data.Filter = filter
	data.FilterDates = datesForFilter(filter)
	data.ShowFolderDocuments = selection.Depth() > 0

	dateYears, err := s.repo.DocumentDateYears(r.Context(), false)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data.DateYears = dateYears
	data.DateYearLinks, data.DateOverflowYears = documentYearLinks(r, dateYears, filter)
	data.DateResetURL = dateFilterResetURL(r)
	if filter.Year > 0 {
		dateMonths, err := s.repo.DocumentDateMonths(r.Context(), false, filter.Year)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		data.DateMonthLinks = documentMonthLinks(r, dateMonths, filter)
	}

	if selection.Depth() > 0 {
		listResult, err := s.documentListService().List(r.Context(), r, filter, perPage)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		if listResult.RedirectURL != "" {
			redirect(w, r, listResult.RedirectURL)
			return
		}
		data.Documents = listResult.Documents
		data.DocumentOCRJobs = listResult.OCRJobs
		data.Filter = filter
		data.HighlightID = highlightIDFromRequest(r)
		data.Pagination = listResult.Pagination
		data.SortLinks = listResult.SortLinks
	}

	s.render(w, r, "folders.html", data)
}

func (s *Server) handleSearchFavoriteFolders(w http.ResponseWriter, r *http.Request, selectedPath []string) {
	perPage, err := s.documentPageSize(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	allTags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data, err := s.folderPageData(r.Context(), r, allTags, folderPathLabels(selectedPath), folderBreadcrumb(r, selectedPath), nil)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}

	switch len(selectedPath) {
	case 1:
		folders, err := s.folderService().SearchFavoriteItems(r.Context(), time.Now())
		if errors.Is(err, sql.ErrNoRows) {
			s.renderError(w, r, http.StatusNotFound, errors.New("Ordner nicht gefunden"))
			return
		}
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		data.FolderTags = folderTagsFromViewItems(r, folders)
	case 2:
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		filter, err := s.folderService().SearchFavoriteFilter(r.Context(), selectedPath[1], time.Now(), page, perPage)
		if errors.Is(err, sql.ErrNoRows) {
			s.renderError(w, r, http.StatusNotFound, errors.New("Suchfavorit nicht gefunden"))
			return
		}
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		if strings.TrimSpace(r.URL.Query().Get("sort")) != "" {
			sort := document.NormalizeListSort(r.URL.Query().Get("sort"))
			filter.Sort = sort
			filter.Direction = document.NormalizeListDirection(r.URL.Query().Get("dir"), sort)
		}
		listResult, err := s.documentListService().List(r.Context(), r, filter, perPage)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		if listResult.RedirectURL != "" {
			redirect(w, r, listResult.RedirectURL)
			return
		}
		data.Documents = listResult.Documents
		data.DocumentOCRJobs = listResult.OCRJobs
		data.Filter = filter
		data.FilterDates = datesForFilter(filter)
		data.HighlightID = highlightIDFromRequest(r)
		data.HideFolderPanel = true
		data.ShowFolderDocuments = true
		data.Pagination = listResult.Pagination
		data.SortLinks = listResult.SortLinks
	default:
		s.renderError(w, r, http.StatusNotFound, errors.New("Ordner nicht gefunden"))
		return
	}

	s.render(w, r, "folders.html", data)
}

func (s *Server) folderPageData(ctx context.Context, r *http.Request, allTags []document.Tag, selectedTags []string, breadcrumb []FolderCrumb, fields []document.CustomField) (PageData, error) {
	desktopPreviewMode, err := s.desktopPreviewMode(ctx)
	if err != nil {
		return PageData{}, err
	}
	if fields == nil {
		fields, err = s.repo.ListCustomFields(ctx)
		if err != nil {
			return PageData{}, err
		}
	}
	columnSettings, err := s.documentColumnSettings(ctx, fields)
	if err != nil {
		return PageData{}, err
	}
	return PageData{
		Title:                 "Ordner",
		Active:                "folders",
		Assets:                documentPageAssets(),
		Tags:                  allTags,
		SelectedTags:          selectedTags,
		FolderBreadcrumb:      breadcrumb,
		TagDescriptions:       tagDescriptionMap(allTags),
		TagStyles:             tagStyleMap(allTags),
		TagListHidden:         tagListHiddenMap(allTags),
		CustomFields:          fields,
		VisibleColumns:        columnSettings.visible,
		DocumentColumns:       columnSettings.table,
		ColumnOptions:         columnSettings.options,
		DesktopDateUnderTitle: columnSettings.desktopDateUnderTitle,
		DesktopPreviewMode:    desktopPreviewMode,
		InlineDesktopPreview:  desktopPreviewMode == desktopPreviewModeInline,
		Notice:                r.URL.Query().Get("notice"),
		ReturnURL:             currentReturnURL(r),
	}, nil
}

func folderTagsFromViewItems(r *http.Request, items []folderViewItem) []FolderTag {
	out := make([]FolderTag, len(items))
	for i, item := range items {
		label, subLabel, countLabel := folderViewHTMLLabels(item)
		out[i] = FolderTag{
			Tag:        item.Tag,
			URL:        folderURL(r, item.Selection),
			Label:      label,
			SubLabel:   subLabel,
			Kind:       folderViewHTMLKind(item.Kind),
			CountLabel: countLabel,
			Redundant:  item.Redundant,
		}
	}
	return out
}

func folderViewHTMLLabels(item folderViewItem) (label string, subLabel string, countLabel string) {
	switch item.Kind {
	case folderViewKindFieldValue:
		return item.Name, item.FieldLabel, ""
	case folderViewKindSearchFavoritesRoot:
		return searchFavoritesFolderWebLabel, "", searchFavoritesCountLabel(item.Count)
	default:
		return "", "", ""
	}
}

func folderViewHTMLKind(kind string) string {
	switch kind {
	case folderViewKindFieldValue:
		return "field-value"
	case folderViewKindSearchFavoritesRoot, folderViewKindSearchFavorite:
		return "search-favorites"
	default:
		return ""
	}
}

func folderDocumentFilterFromRequest(r *http.Request, perPage int) document.ListFilter {
	filter := filterFromRequest(r, false, perPage)
	if strings.TrimSpace(r.URL.Query().Get("sort")) == "" {
		filter.Sort = document.ListSortDate
		filter.Direction = document.ListDirectionDescending
	}
	return filter
}

func searchFavoritesCountLabel(count int) string {
	if count == 1 {
		return "1 Suchfavorit"
	}
	return fmt.Sprintf("%d Suchfavoriten", count)
}

func folderBreadcrumb(r *http.Request, selectedTags []string) []FolderCrumb {
	crumbs := []FolderCrumb{{Label: "Ordner", URL: folderURLForTags(r, nil)}}
	searchFavoritePath := len(selectedTags) > 0 && isSearchFavoritesFolderName(selectedTags[0])
	for i, tag := range selectedTags {
		crumbs = append(crumbs, FolderCrumb{
			Label: folderPathLabel(tag),
			URL:   folderURLForTags(r, selectedTags[:i+1]),
			IsTag: !searchFavoritePath && !isSearchFavoritesFolderName(tag),
		})
	}
	crumbs[len(crumbs)-1].Current = true
	return crumbs
}

func folderPathLabels(tags []string) []string {
	labels := make([]string, len(tags))
	for i, tag := range tags {
		labels[i] = folderPathLabel(tag)
	}
	return labels
}

func folderPathLabel(tag string) string {
	if isSearchFavoritesFolderName(tag) {
		return searchFavoritesFolderWebLabel
	}
	return tag
}

func folderURLForTags(r *http.Request, tags []string) string {
	segments := make([]virtualFolderSegment, len(tags))
	for i, tag := range tags {
		segments[i] = virtualFolderSegment{Kind: virtualFolderSegmentTag, Tag: tag}
	}
	return folderURL(r, virtualFolderSelection{Segments: segments})
}

func folderURL(r *http.Request, selection virtualFolderSelection) string {
	q := clearQueryKeys(r.URL.Query(), "notice", "highlight", "page", "tags", "path")
	if tags, ok := selection.LegacyTagsOnly(); ok {
		for _, tag := range tags {
			q.Add("tags", tag)
		}
	} else {
		for _, value := range selection.PathValues() {
			q.Add("path", value)
		}
	}
	return pathWithQuery("/folders", q)
}
