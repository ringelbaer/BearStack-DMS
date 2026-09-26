package server

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestPhotoCatalogPositionAndDisplayPaths(t *testing.T) {
	s := faceTestServer(t)
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(spec)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.photos.Root(), "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"20240102_Family_Trip/nested_album/two.jpg", "private/secret.jpg"} {
		full := filepath.Join(s.photos.Root(), file)
		if err = os.MkdirAll(filepath.Dir(full), 0750); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(full, data, 0444); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(s.photos.Root(), "private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, user string
		status     int
	}{
		{"one.jpg", "", 401}, {"one.jpg", "reader", 200}, {"20240102_Family_Trip/nested_album/two.jpg", "reader", 200},
		{"private/secret.jpg", "reader", 403}, {"missing.jpg", "reader", 404}, {"../outside.jpg", "reader", 400},
	} {
		w := labelRequest(s, "GET", "/api/photos/v1/browse/position?path="+url.QueryEscape(tc.path), tc.user, "")
		if err := contract.ValidateResponse("/api/photos/v1/browse/position", "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body)
		}
		if w.Code != 200 {
			continue
		}
		var out photos.CatalogFolderPosition
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Path != tc.path || out.Page != 1 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%+v %v", out, err)
		}
		info := labelRequest(s, "GET", "/api/photos/v1/media/info?path="+url.QueryEscape(tc.path), tc.user, "")
		var detail struct {
			Media photoCatalogMedia `json:"media"`
		}
		if err = json.Unmarshal(info.Body.Bytes(), &detail); err != nil || detail.Media.DisplayPath != photos.MediaDisplayPath(tc.path) || detail.Media.FolderName != photos.MediaFolderName(tc.path) {
			t.Fatalf("%s %v", info.Body, err)
		}
	}
	if err = os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/browse/position?path=one.jpg", "reader", ""); w.Code != 403 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
