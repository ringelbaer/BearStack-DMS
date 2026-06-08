package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

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

func TestFriendlyUploadErrorMasksUnexpectedDetails(t *testing.T) {
	got := friendlyUploadError(errors.New("open /private/tmp/bearstack-secret/documents/.tmp: permission denied"))
	if got != "Datei konnte nicht verarbeitet werden" {
		t.Fatalf("error = %q", got)
	}
	if strings.Contains(got, "/private/tmp") || strings.Contains(got, "permission denied") {
		t.Fatalf("internal detail leaked: %q", got)
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
