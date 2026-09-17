package server

import (
	"context"
	"reflect"
	"testing"
	"time"

	"bearstack/internal/document"
)

type folderRepositoryStub struct {
	calls   []string
	filters []document.ListFilter
}

func (r *folderRepositoryStub) ListFolderTags(_ context.Context, filter document.ListFilter) ([]document.Tag, error) {
	r.calls = append(r.calls, "tags")
	r.filters = append(r.filters, filter)
	return []document.Tag{{Name: "tax", Count: 4}}, nil
}
func (r *folderRepositoryStub) ListFolderCustomFieldValues(_ context.Context, filter document.ListFilter) ([]document.CustomFieldValueFolder, error) {
	r.calls = append(r.calls, "fields")
	r.filters = append(r.filters, filter)
	return []document.CustomFieldValueFolder{{FieldID: 1, FieldLabel: "year", Value: "2026", Count: 4}}, nil
}
func (r *folderRepositoryStub) ListSearchFavorites(context.Context) ([]document.SearchFavorite, error) {
	r.calls = append(r.calls, "favorites")
	return []document.SearchFavorite{{ID: 1, Name: "Bills", Query: "invoice"}, {ID: 2, Name: "Taxes", Tags: []string{"tax"}}}, nil
}
func (r *folderRepositoryStub) CountDocuments(_ context.Context, filter document.ListFilter) (int, error) {
	r.calls = append(r.calls, "count")
	r.filters = append(r.filters, filter)
	return 4, nil
}

func TestFolderServiceQueryBudgetAndFilters(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection virtualFolderSelection
		filter    document.ListFilter
		cached    []document.Tag
		calls     []string
		items     int
	}{
		{"root", virtualFolderSelection{}, document.ListFilter{}, nil, []string{"tags", "count", "favorites"}, 2},
		{"cached root", virtualFolderSelection{}, document.ListFilter{}, []document.Tag{{Name: "cached", Count: 4}}, []string{"count", "favorites"}, 2},
		{"filtered root ignores cache", virtualFolderSelection{}, document.ListFilter{Query: "invoice"}, []document.Tag{}, []string{"tags", "count", "favorites"}, 2},
		{"nested", (virtualFolderSelection{}).AppendTag("work"), document.ListFilter{Tags: []string{"work"}}, nil, []string{"tags", "fields", "count"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &folderRepositoryStub{}
			items, err := (folderApplicationService{repo: repo}).ItemsWithRootTags(context.Background(), tc.selection, tc.filter, tc.cached)
			if err != nil || len(items) != tc.items {
				t.Fatalf("items=%+v error=%v", items, err)
			}
			if !reflect.DeepEqual(repo.calls, tc.calls) {
				t.Fatalf("queries=%v, want %v", repo.calls, tc.calls)
			}
			for _, got := range repo.filters {
				if !reflect.DeepEqual(got, tc.filter) {
					t.Fatalf("filter changed: %+v", got)
				}
			}
			if tc.name == "cached root" && items[1].Name != "cached" {
				t.Fatalf("cached tags ignored: %+v", items)
			}
		})
	}
}

func TestFolderServiceUsesBatchFavoriteCounts(t *testing.T) {
	repo := &folderRepositoryStub{}
	var batches int
	svc := folderApplicationService{repo: repo, countDocumentFilters: func(_ context.Context, filters []document.ListFilter) ([]int, error) {
		batches++
		if len(filters) != 2 || filters[0].Query != "invoice" || !reflect.DeepEqual(filters[1].Tags, []string{"tax"}) {
			t.Fatalf("favorite filters=%+v", filters)
		}
		return []int{7, 9}, nil
	}}
	items, err := svc.SearchFavoriteItems(context.Background(), time.Now())
	if err != nil || len(items) != 2 || items[0].Count != 7 || items[1].Count != 9 {
		t.Fatalf("favorites=%+v error=%v", items, err)
	}
	if batches != 1 || !reflect.DeepEqual(repo.calls, []string{"favorites"}) {
		t.Fatalf("queries=%v, batches=%d", repo.calls, batches)
	}
}
