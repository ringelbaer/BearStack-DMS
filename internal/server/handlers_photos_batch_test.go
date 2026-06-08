package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/photos"
)

func TestHandlePhotoThumbnailStatusBatchNormalizesAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()

	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodPost, "/photos/thumbnail/status", bytes.NewBufferString(`{"items":[{"path":"one.jpg","size":32},{"path":"./one.jpg","size":32},{"path":"one.jpg","size":32}]}`))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.handlePhotoThumbnailStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			Path  string `json:"path"`
			Size  int    `json:"size"`
			Ready bool   `json:"ready"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %#v", payload.Items)
	}
	if payload.Items[0].Path != "one.jpg" {
		t.Fatalf("path = %q", payload.Items[0].Path)
	}
	if payload.Items[0].Size != photos.MinThumbnailSize {
		t.Fatalf("size = %d, want %d", payload.Items[0].Size, photos.MinThumbnailSize)
	}
}

func TestHandlePhotoThumbnailStatusBatchRejectsInvalidPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()

	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodPost, "/photos/thumbnail/status", bytes.NewBufferString(`{"items":[{"path":"../one.jpg","size":420}]}`))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.handlePhotoThumbnailStatus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
