package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func TestUploadDuplicateMetadataRequiresDocumentRead(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(config.Config{Addr: "127.0.0.1:0", DataDir: t.TempDir(), MaxUploadBytes: 1 << 20, Auth: config.AuthConfig{
		Credentials: []config.AuthCredential{
			{Username: "admin", Password: "secret", Role: account.RoleAdmin},
			{Username: "uploader", Password: "secret", Role: account.RoleAPIUploader},
			{Username: "custom", Password: "secret", Permissions: []string{account.PermissionDocumentsUpload}},
			{Username: "reader-uploader", Password: "secret", Role: account.RoleAPIUploader, Permissions: []string{account.PermissionDocumentsRead}},
		},
	}}, repo, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	content := []byte("%PDF-1.7\nprivate duplicate fixture")
	upload := func(path, username, filename string) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := multipartUploadBody(t, "files", filename, content)
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.SetBasicAuth(username, "secret")
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		return rec
	}
	var created uploadOutcome
	if err := json.Unmarshal(upload("/api/upload", "admin", "confidential-original.pdf").Body.Bytes(), &created); err != nil || len(created.Uploaded) != 1 {
		t.Fatalf("initial upload = %#v, error = %v", created, err)
	}
	for _, path := range []string{"/api/upload", "/upload"} {
		for _, username := range []string{"uploader", "custom", "reader-uploader", "admin"} {
			t.Run(path+"/"+username, func(t *testing.T) {
				var result struct {
					Duplicates []map[string]any `json:"duplicates"`
				}
				if err := json.Unmarshal(upload(path, username, "probe.pdf").Body.Bytes(), &result); err != nil || len(result.Duplicates) != 1 {
					t.Fatalf("duplicate response = %#v, error = %v", result, err)
				}
				duplicate := result.Duplicates[0]
				if duplicate["filename"] != "probe.pdf" {
					t.Fatalf("submitted filename = %v", duplicate["filename"])
				}
				canRead := username == "reader-uploader" || username == "admin"
				for _, field := range []string{"existing_id", "existing_filename", "document_url"} {
					if _, present := duplicate[field]; present != canRead {
						t.Errorf("%s present = %v, document read permission = %v", field, present, canRead)
					}
				}
				if canRead && (duplicate["existing_id"] != float64(created.Uploaded[0].ID) || duplicate["existing_filename"] != "confidential-original.pdf") {
					t.Errorf("reader duplicate metadata = %#v", duplicate)
				}
				req := httptest.NewRequest(http.MethodGet, created.Uploaded[0].DownloadURL, nil)
				req.SetBasicAuth(username, "secret")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if !canRead && rec.Code != http.StatusForbidden {
					t.Errorf("upload-only download status = %d", rec.Code)
				}
			})
		}
	}
}

func TestHandleUploadWebXHRStoresWebUploadWay(t *testing.T) {
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

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, contentType := multipartUploadBody(t, "files", "web-upload.pdf", []byte("%PDF-1.7\nweb upload"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rec := httptest.NewRecorder()

	server.handleUploadWeb(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}

	var payload uploadOutcome
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Uploaded) != 1 {
		t.Fatalf("uploaded = %#v", payload.Uploaded)
	}
	if payload.Uploaded[0].UploadWay != document.UploadWayWeb {
		t.Fatalf("response upload way = %q", payload.Uploaded[0].UploadWay)
	}
	if payload.Uploaded[0].ContentTextSource != document.ContentTextSourceNone {
		t.Fatalf("response content text source = %q", payload.Uploaded[0].ContentTextSource)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].UploadWay != document.UploadWayWeb {
		t.Fatalf("stored upload way = %#v", docs)
	}
}

func TestHandleUploadWebRedirectsToDocuments(t *testing.T) {
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

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, contentType := multipartUploadBody(t, "files", "web-upload.pdf", []byte("%PDF-1.7\nweb upload"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadWeb(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/documents?notice=") {
		t.Fatalf("location = %q", location)
	}
}

func TestHandleUploadAPIReportsMixedUploadErrors(t *testing.T) {
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

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, contentType := multipartUploadBodyWithFiles(t, "files", []uploadTestFile{
		{name: "not-supported.json", content: []byte(`{"not":"a supported document"}`)},
		{name: "ok.pdf", content: []byte("%PDF-1.7\nok")},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadAPI(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.StatusCode, rec.Body.String())
	}
	var payload uploadOutcome
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Uploaded) != 1 || payload.Uploaded[0].Filename != "ok.pdf" {
		t.Fatalf("uploaded = %#v", payload.Uploaded)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Filename != "not-supported.json" {
		t.Fatalf("errors = %#v", payload.Errors)
	}
	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].OriginalName != "ok.pdf" || docs[0].UploadWay != document.UploadWayAPI {
		t.Fatalf("docs = %#v", docs)
	}
}

func TestHandleUploadAPIRejectsOversizedOnlyUpload(t *testing.T) {
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

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 32},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, contentType := multipartUploadBody(t, "files", "too-big.pdf", append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 64)...))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadAPI(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", res.StatusCode, rec.Body.String())
	}
	var payload uploadOutcome
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Uploaded) != 0 || len(payload.Errors) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("docs = %#v", docs)
	}
}

func TestHandleUploadAPIMasksImportErrors(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, contentType := multipartUploadBody(t, "files", "internal.pdf", []byte("%PDF-1.7\nok"))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadAPI(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", res.StatusCode, rec.Body.String())
	}
	var payload uploadOutcome
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Errors[0].Error != "Dokument konnte nicht importiert werden" {
		t.Fatalf("error = %q", payload.Errors[0].Error)
	}
	if strings.Contains(rec.Body.String(), "sql") || strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("internal import detail leaked: %s", rec.Body.String())
	}
}

type uploadTestFile struct {
	name    string
	content []byte
}

func multipartUploadBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	return multipartUploadBodyWithFiles(t, field, []uploadTestFile{{name: filename, content: content}})
}

func multipartUploadBodyWithFiles(t *testing.T, field string, files []uploadTestFile) (*bytes.Buffer, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, item := range files {
		file, err := writer.CreateFormFile(field, item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(item.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
