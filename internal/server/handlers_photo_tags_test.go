package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/config"
	"bearstack/internal/photos"
)

func TestHandleSaveAndRenamePhotoTag(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:       config.Config{Auth: config.AuthConfig{Username: "admin", Password: "secret"}},
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"name": {"Urlaub"}, "color": {"#2f855a"}}
	req := httptest.NewRequest(http.MethodPost, "/photos/tags/library", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.handleSavePhotoTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/tags?") || !strings.Contains(location, "tab=photos") {
		t.Fatalf("save location = %q", location)
	}

	form = url.Values{"old_name": {"Urlaub"}, "name": {"Reise"}, "color": {"#aa00cc"}}
	req = httptest.NewRequest(http.MethodPost, "/photos/tags/library/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	server.handleRenamePhotoTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("rename status = %d body = %s", rec.Code, rec.Body.String())
	}
	tags, err := photoLib.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "reise" || tags[0].Color != "#aa00cc" || tags[0].Count != 0 {
		t.Fatalf("photo tags = %#v", tags)
	}
	req = httptest.NewRequest(http.MethodGet, "/photos/tags/options", nil)
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	server.handlePhotoTagOptions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag options status = %d body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"color":"#aa00cc"`) || !strings.Contains(body, `--tag-color: #aa00cc`) {
		t.Fatalf("tag options missing color/style: %s", body)
	}

	form = url.Values{"name": {"Reise"}, "password": {"wrong"}}
	req = httptest.NewRequest(http.MethodPost, "/photos/tags/library/delete", strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	server.handleDeletePhotoTag(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete forbidden status = %d body = %s", rec.Code, rec.Body.String())
	}
	tags, err = photoLib.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "reise" {
		t.Fatalf("photo tags after rejected delete = %#v", tags)
	}

	form = url.Values{"name": {"Reise"}, "password": {"secret"}}
	req = httptest.NewRequest(http.MethodPost, "/photos/tags/library/delete", strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	server.handleDeletePhotoTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d body = %s", rec.Code, rec.Body.String())
	}
	tags, err = photoLib.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("photo tags after delete = %#v", tags)
	}
}
