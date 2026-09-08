package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/config"
	"bearstack/internal/photos"
)

func TestPhotoRoutesRejectBrokenPrivacyMarkerForReaders(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	const content = "private photo content"
	if err := os.WriteFile(filepath.Join(root, "secret", "private.jpg"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "reader", Password: "secret", Role: "photos_read"},
			{Username: "admin", Password: "secret", Role: "admin"},
		}}},
		photos: l, templates: templates, log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := s.Handler()
	marker := filepath.Join(root, "secret", photos.AdminOnlyMarkerName)
	for _, target := range []string{"missing", photos.AdminOnlyMarkerName} {
		t.Run(target, func(t *testing.T) {
			if err := os.Symlink(target, marker); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(marker)
			for _, route := range []struct{ method, path, body string }{
				{http.MethodGet, "/photos?path=secret", ""},
				{http.MethodGet, "/photos/media?path=secret/private.jpg", ""},
				{http.MethodGet, "/photos/media/info?path=secret/private.jpg", ""},
				{http.MethodGet, "/photos/thumbnail?path=secret/private.jpg", ""},
				{http.MethodPost, "/photos/media/info", `{"paths":["secret/private.jpg"]}`},
				{http.MethodPost, "/photos/thumbnail/status", `{"items":[{"path":"secret/private.jpg","size":420}]}`},
			} {
				r := httptest.NewRequest(route.method, "http://localhost"+route.path, strings.NewReader(route.body))
				r.SetBasicAuth("reader", "secret")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), content) {
					t.Errorf("%s %s: status %d, want 403 without private data", route.method, route.path, w.Code)
				}
			}
			r := httptest.NewRequest(http.MethodGet, "http://localhost/photos/media?path=secret/private.jpg", nil)
			r.SetBasicAuth("admin", "secret")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Body.String() != content {
				t.Errorf("admin direct access = %d, want original media", w.Code)
			}
		})
	}
}
