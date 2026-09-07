package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"bearstack/internal/document"
)

type documentListRepositoryStub struct {
	total   int
	docs    []document.Document
	calls   []string
	filters []document.ListFilter
	ocrIDs  []int64
	failAt  string
	err     error
}

func (r *documentListRepositoryStub) call(name string) error {
	r.calls = append(r.calls, name)
	if name == r.failAt {
		return r.err
	}
	return nil
}

func (r *documentListRepositoryStub) CountDocuments(_ context.Context, filter document.ListFilter) (int, error) {
	r.filters = append(r.filters, filter)
	return r.total, r.call("count")
}

func (r *documentListRepositoryStub) ListDocuments(_ context.Context, filter document.ListFilter) ([]document.Document, error) {
	r.filters = append(r.filters, filter)
	return r.docs, r.call("list")
}

func (r *documentListRepositoryStub) LatestRelevantOCRJobsForDocuments(_ context.Context, ids []int64) (map[int64]*document.OCRJob, error) {
	r.ocrIDs = ids
	return map[int64]*document.OCRJob{}, r.call("ocr")
}

func TestDocumentListServiceQueryBudget(t *testing.T) {
	for _, tc := range []struct {
		name        string
		total, page int
		docs        []document.Document
		options     documentListOptions
		calls       []string
	}{
		{"API", 2, 1, []document.Document{{ID: 7}, {ID: 9}}, documentListOptions{}, []string{"count", "list"}},
		{"HTML", 2, 1, []document.Document{{ID: 7}, {ID: 9}}, documentListOptions{IncludeOCRJobs: true, SkipOutOfRange: true}, []string{"count", "list", "ocr"}},
		{"HTML out of range", 2, 3, nil, documentListOptions{IncludeOCRJobs: true, SkipOutOfRange: true}, []string{"count"}},
		{"API out of range", 2, 3, nil, documentListOptions{}, []string{"count", "list"}},
		{"HTML empty archive", 0, 3, nil, documentListOptions{IncludeOCRJobs: true, SkipOutOfRange: true}, []string{"count", "list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &documentListRepositoryStub{total: tc.total, docs: tc.docs}
			filter := document.ListFilter{Query: "invoice", Tags: []string{"tax"}, Trash: true, Limit: 25, Offset: (tc.page - 1) * 25, Page: tc.page}
			result, err := (documentListService{repo: repo}).List(context.Background(), filter, tc.options)
			if err != nil || result.Total != tc.total || !reflect.DeepEqual(result.Documents, tc.docs) {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if !reflect.DeepEqual(repo.calls, tc.calls) {
				t.Fatalf("queries = %v, want %v", repo.calls, tc.calls)
			}
			for _, got := range repo.filters {
				if !reflect.DeepEqual(got, filter) {
					t.Fatalf("filter changed: %+v", got)
				}
			}
			if tc.options.IncludeOCRJobs && len(tc.docs) > 0 && !reflect.DeepEqual(repo.ocrIDs, []int64{7, 9}) {
				t.Fatalf("OCR must load one batch: %v", repo.ocrIDs)
			}
		})
	}
}

func TestDocumentListServicePropagatesErrors(t *testing.T) {
	want := errors.New("query failed")
	for _, operation := range []string{"count", "list", "ocr"} {
		repo := &documentListRepositoryStub{total: 1, docs: []document.Document{{ID: 7}}, failAt: operation, err: want}
		_, err := (documentListService{repo: repo}).List(context.Background(), document.ListFilter{Limit: 25, Page: 1}, documentListOptions{IncludeOCRJobs: true})
		if !errors.Is(err, want) {
			t.Fatalf("%s: error = %v", operation, err)
		}
	}
}

func TestDocumentListHTMLAndAPIKeepDifferentOutOfRangeResponses(t *testing.T) {
	ctx := context.Background()
	repo := openAuthSecurityRepository(t)
	if _, err := repo.CreateDocument(ctx, document.Document{OriginalName: "invoice.pdf", StoredPath: "invoice.pdf", Title: "Invoice", Tags: []string{"tax"}, MIMEType: "application/pdf", SizeBytes: 1, SHA256: "list-page-test"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{repo: repo}
	r := httptest.NewRequest(http.MethodGet, "/documents?tags=tax&page=4&sort=title&notice=old&highlight=9", nil)
	view, err := s.documentListView(ctx, r, filterFromRequest(r, false, 25), 25)
	if err != nil || view.RedirectURL != "/documents?sort=title&tags=tax" || len(view.Documents) != 0 {
		t.Fatalf("HTML result = %+v, err = %v", view, err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/documents?tags=tax&page=4&page_size=25", nil)
	w := httptest.NewRecorder()
	s.handleAPIDocuments(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" {
		t.Fatalf("API status = %d, headers = %v", w.Code, w.Header())
	}
	var response struct {
		Documents  []documentAPIResponse       `json:"documents"`
		Pagination documentAPIPaginationResult `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Documents == nil || len(response.Documents) != 0 || response.Pagination.Page != 4 || response.Pagination.Total != 1 {
		t.Fatalf("API response = %+v", response)
	}
}
