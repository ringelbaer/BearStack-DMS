// Datei baut Seitendaten fuer Dokumentlisten, Papierkorb und partielle Tabellenupdates.
package server

import (
	"context"
	"net/http"
	"time"

	"bearstack/internal/document"
)

type documentListPageBuilder struct {
	server *Server
}

type documentListPageResult struct {
	data        PageData
	partial     bool
	redirectURL string
}

func (s *Server) documentListPageBuilder() documentListPageBuilder {
	return documentListPageBuilder{server: s}
}

func (builder documentListPageBuilder) Build(ctx context.Context, r *http.Request, options documentListPageOptions) (documentListPageResult, error) {
	s := builder.server
	partial := wantsDocumentListPartial(r)
	perPage, err := s.documentPageSize(ctx)
	if err != nil {
		return documentListPageResult{}, err
	}
	filter := filterFromRequest(r, options.trash, perPage)
	desktopPreviewMode, err := s.desktopPreviewMode(ctx)
	if err != nil {
		return documentListPageResult{}, err
	}
	listResult, err := s.documentListService().List(ctx, r, filter, perPage)
	if err != nil {
		return documentListPageResult{}, err
	}
	if listResult.RedirectURL != "" {
		return documentListPageResult{redirectURL: listResult.RedirectURL}, nil
	}
	tags, err := s.repo.ListTags(ctx)
	if err != nil {
		return documentListPageResult{}, err
	}

	data := PageData{
		Title:                options.title,
		Active:               options.active,
		Assets:               documentPageAssets(),
		Documents:            listResult.Documents,
		DocumentOCRJobs:      listResult.OCRJobs,
		Tags:                 tags,
		TagDescriptions:      tagDescriptionMap(tags),
		TagStyles:            tagStyleMap(tags),
		TagListHidden:        tagListHiddenMap(tags),
		Filter:               filter,
		DocumentFilterActive: documentFilterActive(filter),
		FilterDates:          datesForFilter(filter),
		DesktopPreviewMode:   desktopPreviewMode,
		InlineDesktopPreview: desktopPreviewMode == desktopPreviewModeInline,
		HighlightID:          highlightIDFromRequest(r),
		Notice:               r.URL.Query().Get("notice"),
		ReturnURL:            currentReturnURL(r),
		Pagination:           listResult.Pagination,
		SortLinks:            listResult.SortLinks,
	}

	if options.loadColumns {
		fields, err := s.repo.ListCustomFields(ctx)
		if err != nil {
			return documentListPageResult{}, err
		}
		columnSettings, err := s.documentColumnSettings(ctx, fields)
		if err != nil {
			return documentListPageResult{}, err
		}
		data.CustomFields = fields
		data.VisibleColumns = columnSettings.visible
		data.DocumentColumns = columnSettings.table
		data.ColumnOptions = columnSettings.options
		data.DesktopDateUnderTitle = columnSettings.desktopDateUnderTitle
	}

	if options.loadDateYears && !partial {
		dateYears, err := s.repo.DocumentDateYears(ctx, options.trash)
		if err != nil {
			return documentListPageResult{}, err
		}
		data.DateYears = dateYears
		data.DateYearLinks, data.DateOverflowYears = documentYearLinks(r, dateYears, filter)
		if filter.Year > 0 {
			dateMonths, err := s.repo.DocumentDateMonths(ctx, options.trash, filter.Year)
			if err != nil {
				return documentListPageResult{}, err
			}
			data.DateMonthLinks = documentMonthLinks(r, dateMonths, filter)
		}
	}

	if options.loadFavorites && !partial {
		favorites, err := s.repo.ListSearchFavorites(ctx)
		if err != nil {
			return documentListPageResult{}, err
		}
		data.SearchFavorites = searchFavoriteViews(favorites, time.Now())
	}

	if options.showUpload {
		data.MaxUploadMB = s.cfg.MaxUploadBytes / 1024 / 1024
	}

	return documentListPageResult{
		data:    data,
		partial: partial,
	}, nil
}

func documentIDs(docs []document.Document) []int64 {
	ids := make([]int64, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	return ids
}
