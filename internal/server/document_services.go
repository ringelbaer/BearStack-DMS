// Datei buendelt fachliche Dokumentservices fuer Handler, Validierung und Repository-Aufrufe.
package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

type documentListService struct {
	repo           *repository.Repository
	countDocuments func(context.Context, document.ListFilter) (int, error)
}

type documentListResult struct {
	Documents   []document.Document
	OCRJobs     map[int64]*document.OCRJob
	Total       int
	RedirectURL string
	Pagination  PaginationData
	SortLinks   map[string]SortLink
}

func (s *Server) documentListService() documentListService {
	return documentListService{
		repo:           s.repo,
		countDocuments: s.countDocuments,
	}
}

func (svc documentListService) List(ctx context.Context, r *http.Request, filter document.ListFilter, perPage int) (documentListResult, error) {
	total, err := svc.count(ctx, filter)
	if err != nil {
		return documentListResult{}, err
	}
	if target := documentPageRedirectURL(r, filter.Page, perPage, total); target != "" {
		return documentListResult{Total: total, RedirectURL: target}, nil
	}
	docs, err := svc.repo.ListDocuments(ctx, filter)
	if err != nil {
		return documentListResult{}, err
	}
	ocrJobs := map[int64]*document.OCRJob{}
	if len(docs) > 0 {
		ocrJobs, err = svc.repo.LatestRelevantOCRJobsForDocuments(ctx, documentIDs(docs))
		if err != nil {
			return documentListResult{}, err
		}
	}
	return documentListResult{
		Documents:  docs,
		OCRJobs:    ocrJobs,
		Total:      total,
		Pagination: documentListPaginationData(r, filter.Page, perPage, len(docs), total),
		SortLinks:  documentSortLinks(r, filter),
	}, nil
}

func (svc documentListService) count(ctx context.Context, filter document.ListFilter) (int, error) {
	if svc.countDocuments != nil {
		return svc.countDocuments(ctx, filter)
	}
	return svc.repo.CountDocuments(ctx, filter)
}

type folderApplicationService struct {
	repo                        *repository.Repository
	listFolderCustomFieldValues func(context.Context, document.ListFilter) ([]document.CustomFieldValueFolder, error)
	folderTagMinDocuments       func(context.Context) (int, error)
	countDocuments              func(context.Context, document.ListFilter) (int, error)
	countDocumentFilters        func(context.Context, []document.ListFilter) ([]int, error)
}

func (s *Server) folderService() folderApplicationService {
	return folderApplicationService{
		repo:                        s.repo,
		listFolderCustomFieldValues: s.listFolderCustomFieldValues,
		folderTagMinDocuments:       s.folderTagMinDocuments,
		countDocuments:              s.countDocuments,
		countDocumentFilters:        s.countDocumentFilters,
	}
}

func (svc folderApplicationService) Items(ctx context.Context, selection virtualFolderSelection, filter document.ListFilter) ([]folderViewItem, error) {
	return svc.ItemsWithRootTags(ctx, selection, filter, nil)
}

func (svc folderApplicationService) ItemsWithRootTags(ctx context.Context, selection virtualFolderSelection, filter document.ListFilter, rootTags []document.Tag) ([]folderViewItem, error) {
	var folderTags []document.Tag
	var err error
	if selection.Depth() == 0 && !folderDocumentFilterActive(filter) && rootTags != nil {
		folderTags = rootTags
	} else {
		folderTags, err = svc.repo.ListFolderTags(ctx, filter)
	}
	if err != nil {
		return nil, err
	}
	if selection.Depth() == 0 {
		minDocuments, err := svc.rootTagMinDocuments(ctx)
		if err != nil {
			return nil, err
		}
		folderTags = filterFolderTagsByMinDocuments(folderTags, minDocuments)
	}
	var fieldValueFolders []document.CustomFieldValueFolder
	if selection.Depth() > 0 {
		fieldValueFolders, err = svc.listCustomFieldValues(ctx, filter)
		if err != nil {
			return nil, err
		}
	}
	folderItems := folderViewItemsFromTags(folderTags, selection)
	folderItems = append(folderItems, folderViewItemsFromCustomFieldValues(fieldValueFolders, selection)...)
	if len(folderItems) > 0 {
		currentLevelCount, err := svc.count(ctx, filter)
		if err != nil {
			return nil, err
		}
		markRedundantFolderViewItems(folderItems, currentLevelCount)
	}
	sortFolderViewItems(folderItems)
	if selection.Depth() == 0 {
		favorites, err := svc.repo.ListSearchFavorites(ctx)
		if err != nil {
			return nil, err
		}
		if len(favorites) > 0 {
			folderItems = append([]folderViewItem{searchFavoritesRootViewItem(len(favorites))}, folderItems...)
		}
	}
	return folderItems, nil
}

func (svc folderApplicationService) rootTagMinDocuments(ctx context.Context) (int, error) {
	if svc.folderTagMinDocuments == nil {
		return 0, nil
	}
	return svc.folderTagMinDocuments(ctx)
}

func filterFolderTagsByMinDocuments(tags []document.Tag, minDocuments int) []document.Tag {
	if minDocuments <= 0 {
		return tags
	}
	filtered := make([]document.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag.Count >= minDocuments {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}

func folderDocumentFilterActive(filter document.ListFilter) bool {
	if filter.Query != "" || filter.From != nil || filter.To != nil || filter.Trash {
		return true
	}
	for _, customField := range filter.CustomFields {
		if customField.FieldID > 0 && document.CleanCustomFieldFilterValue(customField.Value) != "" {
			return true
		}
	}
	return false
}

func (svc folderApplicationService) listCustomFieldValues(ctx context.Context, filter document.ListFilter) ([]document.CustomFieldValueFolder, error) {
	if svc.listFolderCustomFieldValues != nil {
		return svc.listFolderCustomFieldValues(ctx, filter)
	}
	return svc.repo.ListFolderCustomFieldValues(ctx, filter)
}

func (svc folderApplicationService) SearchFavoriteItems(ctx context.Context, now time.Time) ([]folderViewItem, error) {
	favorites, err := svc.repo.ListSearchFavorites(ctx)
	if err != nil {
		return nil, err
	}
	if len(favorites) == 0 {
		return nil, sql.ErrNoRows
	}
	filters := make([]document.ListFilter, 0, len(favorites))
	for _, favorite := range favorites {
		filters = append(filters, searchFavoriteFilter(favorite, now, 0, 0))
	}
	counts, err := svc.countMany(ctx, filters)
	if err != nil {
		return nil, err
	}
	if len(counts) != len(favorites) {
		return nil, errors.New("search favorite count mismatch")
	}
	items := make([]folderViewItem, 0, len(favorites))
	for i, favorite := range favorites {
		items = append(items, searchFavoriteViewItem(favorite, counts[i]))
	}
	return items, nil
}

func (svc folderApplicationService) SearchFavorite(ctx context.Context, name string) (document.SearchFavorite, error) {
	favorites, err := svc.repo.ListSearchFavorites(ctx)
	if err != nil {
		return document.SearchFavorite{}, err
	}
	if len(favorites) == 0 {
		return document.SearchFavorite{}, sql.ErrNoRows
	}
	favorite, ok := findSearchFavoriteByName(favorites, name)
	if !ok {
		return document.SearchFavorite{}, sql.ErrNoRows
	}
	return favorite, nil
}

func (svc folderApplicationService) SearchFavoriteFilter(ctx context.Context, name string, now time.Time, page, perPage int) (document.ListFilter, error) {
	favorite, err := svc.SearchFavorite(ctx, name)
	if err != nil {
		return document.ListFilter{}, err
	}
	return searchFavoriteFilter(favorite, now, page, perPage), nil
}

func (svc folderApplicationService) count(ctx context.Context, filter document.ListFilter) (int, error) {
	if svc.countDocuments != nil {
		return svc.countDocuments(ctx, filter)
	}
	return svc.repo.CountDocuments(ctx, filter)
}

func (svc folderApplicationService) countMany(ctx context.Context, filters []document.ListFilter) ([]int, error) {
	if svc.countDocumentFilters != nil {
		return svc.countDocumentFilters(ctx, filters)
	}
	counts := make([]int, 0, len(filters))
	for _, filter := range filters {
		count, err := svc.count(ctx, filter)
		if err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, nil
}
