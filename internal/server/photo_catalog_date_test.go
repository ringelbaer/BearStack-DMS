package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/photos"
)

func TestPhotoCatalogDateReaderValidationAndFreshVisibility(t *testing.T) {
	s := faceTestServer(t)
	for _, test := range []struct {
		date, user string
		status     int
	}{
		{"2026-01-01", "", 401}, {"2026-02-30", "reader", 400}, {"", "reader", 400},
		{"2026-01-01", "reader", 200}, {"2026-01-01", "editor", 200}, {"2026-01-01", "manager", 200},
	} {
		w := labelRequest(s, "GET", "/api/photos/v1/browse/date?date="+test.date, test.user, "")
		if w.Code != test.status {
			t.Fatalf("%+v: %d %s", test, w.Code, w.Body.String())
		}
		if w.Code == 200 {
			var out photos.CatalogDatePosition
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Path != "one.jpg" || out.Page != 1 || out.Date == "" || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("%+v", out)
			}
		}
	}
	// A newly locked folder is checked before returning its date, path or rank.
	if err := os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	w := labelRequest(s, "GET", "/api/photos/v1/browse/date?date=2026-01-01", "reader", "")
	if w.Code != 403 || strings.Contains(w.Body.String(), "one.jpg") {
		t.Fatalf("private: %d %s", w.Code, w.Body.String())
	}
}

func TestPhotoCatalogDateExcludesIndexedPrivateMediaAndMatchesBrowse(t *testing.T) {
	s := faceTestServer(t)
	root := s.photos.Root()
	image, err := os.ReadFile(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(root, "private")
	if err = os.Mkdir(private, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(private, "hidden.jpg"), image, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(private, ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	if err = os.Chtimes(filepath.Join(private, "hidden.jpg"), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := labelRequest(s, "GET", "/api/photos/v1/browse/date?date=2030-01-01", "reader", "")
	var out photos.CatalogDatePosition
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Path != "one.jpg" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", "/api/photos/v1/browse?recursive=1&sort=descending_date&section=media&page=1", "reader", "")
	var page photoCatalogPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Media) != 1 || page.Media[0].Path != out.Path {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
