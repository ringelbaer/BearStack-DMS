package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func TestHandleAPIDocumentsUsesUIFilters(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kundennummer", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}

	docDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   "2026/03/rechnung.pdf",
		Title:        "Rechnung",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "api-doc-main",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	linkedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "anlage.pdf",
		StoredPath:   "2026/03/anlage.pdf",
		Title:        "Anlage",
		MIMEType:     "application/pdf",
		SizeBytes:    24,
		SHA256:       "api-doc-linked",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateDocument(ctx, document.Document{
		OriginalName: "vertrag.pdf",
		StoredPath:   "2026/03/vertrag.pdf",
		Title:        "Vertrag",
		MIMEType:     "application/pdf",
		SizeBytes:    11,
		SHA256:       "api-doc-other",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, docID, "Rechnung", "", &docDate, []string{"steuer"}, map[int64]string{fields[0].ID: "ACME-42"}); err != nil {
		t.Fatal(err)
	}
	decoyID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung-beta.pdf",
		StoredPath:   "2026/03/rechnung-beta.pdf",
		Title:        "Rechnung Beta",
		MIMEType:     "application/pdf",
		SizeBytes:    13,
		SHA256:       "api-doc-decoy",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, decoyID, "Rechnung Beta", "", &docDate, []string{"steuer"}, map[int64]string{fields[0].ID: "BETA-42"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.LinkDocuments(ctx, []int64{docID, linkedID}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/documents?q=rechnung&tags=steuer&field_%d=ACME&year=2026&sort=title&dir=asc&page_size=25", fields[0].ID), nil)
	rec := httptest.NewRecorder()

	server.handleAPIDocuments(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Documents []struct {
			ID           int64 `json:"id"`
			LinkedCount  int   `json:"linked_count"`
			CustomValues []struct {
				FieldID int64  `json:"field_id"`
				Label   string `json:"label"`
				Value   string `json:"value"`
			} `json:"custom_values"`
		} `json:"documents"`
		Filter struct {
			Query        string   `json:"query"`
			Tags         []string `json:"tags"`
			CustomFields []struct {
				FieldID int64  `json:"field_id"`
				Value   string `json:"value"`
			} `json:"custom_fields"`
			Year      int    `json:"year"`
			Sort      string `json:"sort"`
			Direction string `json:"direction"`
		} `json:"filter"`
		Pagination struct {
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
			Total   int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Documents) != 1 || payload.Documents[0].ID != docID {
		t.Fatalf("documents = %#v", payload.Documents)
	}
	if payload.Documents[0].LinkedCount != 1 {
		t.Fatalf("linked count = %d", payload.Documents[0].LinkedCount)
	}
	if len(payload.Documents[0].CustomValues) != 1 || payload.Documents[0].CustomValues[0].Label != "Kundennummer" || payload.Documents[0].CustomValues[0].Value != "ACME-42" {
		t.Fatalf("custom values = %#v", payload.Documents[0].CustomValues)
	}
	if payload.Filter.Query != "rechnung" || len(payload.Filter.Tags) != 1 || payload.Filter.Tags[0] != "steuer" || payload.Filter.Year != 2026 || payload.Filter.Sort != document.ListSortTitle || payload.Filter.Direction != document.ListDirectionAscending {
		t.Fatalf("filter = %#v", payload.Filter)
	}
	if len(payload.Filter.CustomFields) != 1 || payload.Filter.CustomFields[0].FieldID != fields[0].ID || payload.Filter.CustomFields[0].Value != "ACME" {
		t.Fatalf("custom field filters = %#v", payload.Filter.CustomFields)
	}
	if payload.Pagination.Page != 1 || payload.Pagination.PerPage != 25 || payload.Pagination.Total != 1 {
		t.Fatalf("pagination = %#v", payload.Pagination)
	}
}

func TestHandleAPIFoldersRootListsFoldersOnly(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	for i, doc := range []document.Document{
		{OriginalName: "steuer.pdf", StoredPath: "steuer.pdf", Title: "Steuer", Tags: []string{"steuer"}, MIMEType: "application/pdf", SizeBytes: 1},
		{OriginalName: "privat.pdf", StoredPath: "privat.pdf", Title: "Privat", Tags: []string{"privat"}, MIMEType: "application/pdf", SizeBytes: 1},
	} {
		doc.SHA256 = fmt.Sprintf("api-folder-root-%d", i)
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/folders?year=2026&q=ignored&page_size=25", nil)
	rec := httptest.NewRecorder()

	server.handleAPIFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		SelectedTags []string        `json:"selected_tags"`
		Folders      []apiFolderTest `json:"folders"`
		Documents    []struct {
			ID int64 `json:"id"`
		} `json:"documents"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.SelectedTags) != 0 || len(payload.Documents) != 0 || payload.Pagination.Total != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if got := folderNames(payload.Folders); strings.Join(got, ",") != "privat,steuer" {
		t.Fatalf("folders = %#v", payload.Folders)
	}
}

func TestHandleAPIFoldersUsesTagFoldersWithoutDateSearchOrTrashFilters(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	date2024 := time.Date(2024, time.February, 12, 0, 0, 0, 0, time.UTC)
	date2023 := time.Date(2023, time.March, 7, 0, 0, 0, 0, time.UTC)
	for i, doc := range []document.Document{
		{OriginalName: "steuer-privat.pdf", StoredPath: "a.pdf", Title: "Steuer privat", Tags: []string{"steuer", "privat"}, MIMEType: "application/pdf", SizeBytes: 1, DocumentDate: &date2024},
		{OriginalName: "steuer-bank.pdf", StoredPath: "b.pdf", Title: "Steuer Bank", Tags: []string{"steuer", "bank"}, MIMEType: "application/pdf", SizeBytes: 1, DocumentDate: &date2023},
		{OriginalName: "privat-bank.pdf", StoredPath: "c.pdf", Title: "Privat Bank", Tags: []string{"privat", "bank"}, MIMEType: "application/pdf", SizeBytes: 1, DocumentDate: &date2024},
	} {
		doc.SHA256 = fmt.Sprintf("api-folder-filter-%d", i)
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/folders?tags=steuer&q=nomatch&from=2024-01-01&to=2024-12-31&trash=1&page_size=25", nil)
	rec := httptest.NewRecorder()

	server.handleAPIFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		SelectedTags []string        `json:"selected_tags"`
		Folders      []apiFolderTest `json:"folders"`
		Documents    []struct {
			ID           int64  `json:"id"`
			DocumentDate string `json:"document_date"`
		} `json:"documents"`
		Pagination struct {
			Page    int  `json:"page"`
			PerPage int  `json:"per_page"`
			Total   int  `json:"total"`
			HasNext bool `json:"has_next"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.SelectedTags) != 1 || payload.SelectedTags[0] != "steuer" {
		t.Fatalf("selected tags = %#v", payload.SelectedTags)
	}
	if got := folderNames(payload.Folders); strings.Join(got, ",") != "bank,privat" {
		t.Fatalf("folders = %#v", payload.Folders)
	}
	if len(payload.Documents) != 2 || payload.Pagination.Total != 2 || payload.Pagination.PerPage != 25 || payload.Pagination.HasNext {
		t.Fatalf("documents/pagination = %#v %#v", payload.Documents, payload.Pagination)
	}
	dates := []string{payload.Documents[0].DocumentDate, payload.Documents[1].DocumentDate}
	if !containsString(dates, "2023-03-07") || !containsString(dates, "2024-02-12") {
		t.Fatalf("dates show date filter was applied: %#v", dates)
	}
}

func TestHandleAPIFoldersListsConfiguredCustomFieldValueFolders(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false, 5); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kundeID, projektID int64
	for _, field := range fields {
		switch field.Label {
		case "Kunde":
			kundeID = field.ID
		case "Projekt":
			projektID = field.ID
		}
	}
	for i, item := range []struct {
		name   string
		tags   []string
		values map[int64]string
	}{
		{"a.pdf", []string{"steuer", "privat"}, map[int64]string{kundeID: "ACME", projektID: "Alpha"}},
		{"b.pdf", []string{"steuer"}, map[int64]string{kundeID: "Beta", projektID: "Alpha"}},
		{"c.pdf", []string{"privat"}, map[int64]string{kundeID: "ACME", projektID: "Alpha"}},
	} {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			Tags:         item.tags,
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("api-folder-field-value-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, item.tags, item.values); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/folders?tags=steuer&page_size=25", nil)
	rec := httptest.NewRecorder()

	server.handleAPIFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Folders []struct {
			Name       string `json:"name"`
			Kind       string `json:"kind"`
			FieldID    int64  `json:"field_id"`
			FieldLabel string `json:"field_label"`
			Value      string `json:"value"`
			Count      int    `json:"count"`
		} `json:"folders"`
		Documents []struct {
			OriginalName string `json:"original_name"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	folders := map[string]struct {
		Kind  string
		Count int
	}{}
	for _, folder := range payload.Folders {
		folders[folder.Name] = struct {
			Kind  string
			Count int
		}{Kind: folder.Kind, Count: folder.Count}
		if folder.Name == "ACME" && (folder.FieldID != kundeID || folder.FieldLabel != "Kunde" || folder.Value != "ACME") {
			t.Fatalf("custom folder metadata = %#v", folder)
		}
	}
	var folderOrder []string
	for _, folder := range payload.Folders {
		folderOrder = append(folderOrder, folder.Name)
	}
	if strings.Join(folderOrder, ",") != "ACME,Beta,privat" {
		t.Fatalf("folder order = %#v", payload.Folders)
	}
	if folders["privat"].Count != 1 || folders["ACME"].Kind != "field_value" || folders["ACME"].Count != 1 || folders["Beta"].Count != 1 {
		t.Fatalf("folders = %#v", payload.Folders)
	}
	for _, folder := range payload.Folders {
		if folder.FieldLabel == "Projekt" {
			t.Fatalf("thresholded field value folder leaked: %#v", payload.Folders)
		}
	}
	if len(payload.Documents) != 2 {
		t.Fatalf("documents = %#v", payload.Documents)
	}

	q := url.Values{}
	q.Add("path", "tag:steuer")
	q.Add("path", fmt.Sprintf("field:%d:ACME", kundeID))
	q.Set("page_size", "25")
	req = httptest.NewRequest(http.MethodGet, "/api/folders?"+q.Encode(), nil)
	rec = httptest.NewRecorder()

	server.handleAPIFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("field path status = %d body = %s", rec.Code, rec.Body.String())
	}
	var selectedPayload struct {
		SelectedPath []struct {
			Kind  string `json:"kind"`
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"selected_path"`
		Folders []struct {
			Name string `json:"name"`
		} `json:"folders"`
		Documents []struct {
			OriginalName string `json:"original_name"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&selectedPayload); err != nil {
		t.Fatal(err)
	}
	if len(selectedPayload.SelectedPath) != 2 || selectedPayload.SelectedPath[1].Label != "Kunde: ACME" || selectedPayload.SelectedPath[1].Value != "ACME" {
		t.Fatalf("selected path = %#v", selectedPayload.SelectedPath)
	}
	for _, folder := range selectedPayload.Folders {
		if folder.Name == "ACME" || folder.Name == "Beta" {
			t.Fatalf("selected field appears again as child folder: %#v", selectedPayload.Folders)
		}
	}
	if len(selectedPayload.Documents) != 1 || selectedPayload.Documents[0].OriginalName != "a.pdf" {
		t.Fatalf("field path documents = %#v", selectedPayload.Documents)
	}
}

func TestHandleAPIFoldersPaginatesDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	for i := 0; i < 26; i++ {
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: fmt.Sprintf("doc-%02d.pdf", i),
			StoredPath:   fmt.Sprintf("doc-%02d.pdf", i),
			Title:        fmt.Sprintf("Doc %02d", i),
			Tags:         []string{"steuer"},
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("api-folder-page-%02d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/folders?tags=steuer&page_size=25", nil)
	rec := httptest.NewRecorder()

	server.handleAPIFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Documents []struct {
			ID int64 `json:"id"`
		} `json:"documents"`
		Pagination struct {
			Page    int  `json:"page"`
			PerPage int  `json:"per_page"`
			Total   int  `json:"total"`
			HasNext bool `json:"has_next"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Documents) != 25 || payload.Pagination.Page != 1 || payload.Pagination.PerPage != 25 || payload.Pagination.Total != 26 || !payload.Pagination.HasNext {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleAPIDownloadAliasServesStoredFile(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	storedPath := "2026/05/rechnung.pdf"
	content := []byte("%PDF test")
	if err := os.MkdirAll(filepath.Join(store.Root(), "2026", "05"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "2026", "05", "rechnung.pdf"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   storedPath,
		Title:        "Rechnung",
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len(content)),
		SHA256:       "api-download-alias",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/documents/%d/download", docID), nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, content) {
		t.Fatalf("body = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "rechnung.pdf") {
		t.Fatalf("content disposition = %q", got)
	}
}

type apiFolderTest struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func folderNames(folders []apiFolderTest) []string {
	names := make([]string, len(folders))
	for i, folder := range folders {
		names[i] = folder.Name
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
