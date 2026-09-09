package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestBrowserPhotoRouteHTTPUsesSharedCache(t *testing.T) {
	s := faceTestServer(t)
	root := s.photos.Root()
	image, err := os.ReadFile(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "2026", "trip")
	if err = os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.jpg", "two.jpg"} {
		if err = os.WriteFile(filepath.Join(dir, name), image, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", s.photos.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE media_index SET latitude=CASE name WHEN 'one.jpg' THEN 52 ELSE 53 END,longitude=13 WHERE directory='2026/trip'`); err != nil {
		t.Fatal(err)
	}
	checkBrowser := func() {
		t.Helper()
		w := labelRequest(s, "GET", "/photos?path=2026/trip&view=map", "reader", "")
		if w.Code != 200 || strings.Count(w.Body.String(), "data-photo-route-point data-lat=") != 2 {
			t.Fatalf("browser route: %d %s", w.Code, w.Body.String())
		}
	}
	checkBrowser()
	files, err := filepath.Glob(filepath.Join(s.photos.CacheDir(), "photo-routes", "v1", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("browser cache: %v %v", files, err)
	}
	before, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DROP INDEX idx_media_index_route_time`); err != nil {
		t.Fatal(err)
	}
	checkBrowser()
	w := labelRequest(s, "GET", "/api/photos/v1/map/route?path=2026/trip", "reader", "")
	var route photos.MapPhotoRoute
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &route) != nil || route.TotalMedia != 2 {
		t.Fatalf("native shared cache: %d %s", w.Code, w.Body.String())
	}
	after, err := os.Stat(files[0])
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("shared cache replaced: %v", err)
	}
}

func TestPhotoMapTracksAreReadOnlyBoundedAndPrivate(t *testing.T) {
	s := faceTestServer(t)
	root := s.photos.Root()
	if err := os.Mkdir(filepath.Join(root, "trip"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "trip", "track.gpx"), []byte(`<gpx><trkseg><trkpt lat="1" lon="2"/><trkpt lat="2" lon="3"/></trkseg><trkseg><trkpt lat="4" lon="5"/></trkseg></gpx>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.gpx"), []byte(`<gpx><`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/photos/v1/map/tracks?path=trip", "/api/photos/v1/map/track?path=trip/track.gpx&points=32"} {
		w := labelRequest(s, "GET", path, "reader", "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("read %s: %d %s", path, w.Code, w.Body.String())
		}
		if w := labelRequest(s, "GET", path, "", ""); w.Code != 401 {
			t.Fatalf("auth: %d", w.Code)
		}
		if w := labelRequest(s, "POST", path, "reader", "{}"); w.Code != 405 {
			t.Fatalf("write: %d", w.Code)
		}
	}
	w := labelRequest(s, "GET", "/api/photos/v1/map/track?path=trip/track.gpx", "reader", "")
	var geometry photos.MapTrackGeometry
	if err := json.Unmarshal(w.Body.Bytes(), &geometry); err != nil || len(geometry.Segments) != 2 || geometry.TotalPoints != 3 {
		t.Fatalf("geometry: %s", w.Body.String())
	}
	for _, test := range []struct {
		url  string
		code int
	}{
		{"map/tracks?cursor=secret", 400}, {"map/tracks?path=../outside", 400},
		{"map/track?path=trip/track.gpx&points=31", 400}, {"map/track?path=trip/track.gpx&south=0", 400},
		{"map/track?path=missing.gpx", 404}, {"map/track?path=bad.gpx", 422},
	} {
		if w := labelRequest(s, "GET", "/api/photos/v1/"+test.url, "reader", ""); w.Code != test.code {
			t.Errorf("%s: %d %s", test.url, w.Code, w.Body.String())
		}
	}
	if err := os.WriteFile(filepath.Join(root, "trip", photos.AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/map/track?path=trip/track.gpx", "reader", ""); w.Code != 403 {
		t.Fatalf("private geometry: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", "/api/photos/v1/map/tracks", "reader", "")
	var inventory photos.GPXFilePage
	if err := json.Unmarshal(w.Body.Bytes(), &inventory); err != nil || len(inventory.Files) != 1 || inventory.Files[0].Path != "bad.gpx" {
		t.Fatalf("private inventory: %s", w.Body.String())
	}
}

func TestNativePhotoRouteUsesReadPermissionAndConfiguredRadius(t *testing.T) {
	s := faceTestServer(t)
	if _, err := s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	seedPhotoRoutePosition(t, s)
	settings, err := s.photoSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.MapTrackResolutionMeters = 3000
	if err = s.savePhotoSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	w := labelRequest(s, "GET", "/api/photos/v1/map/route?points=32", "reader", "")
	var route photos.MapPhotoRoute
	if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("route: %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &route); err != nil || route.RadiusMeters != 3000 || route.TotalMedia != 1 || len(route.Segments) != 1 {
		t.Fatalf("settings: %s", w.Body.String())
	}
	for _, test := range []struct {
		path, user, method string
		code               int
	}{
		{"map/route", "", "GET", 401}, {"map/route", "reader", "POST", 405},
		{"map/route?points=31", "reader", "GET", 400}, {"map/route?south=0", "reader", "GET", 400},
		{"map/route?path=../outside", "reader", "GET", 400}, {"map/route?type=unknown", "reader", "GET", 400},
	} {
		if w := labelRequest(s, test.method, "/api/photos/v1/"+test.path, test.user, ""); w.Code != test.code {
			t.Fatalf("%s: %d %s", test.path, w.Code, w.Body.String())
		}
	}
	if err = os.Mkdir(filepath.Join(s.photos.Root(), "private-route"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.photos.Root(), "private-route", photos.AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/map/route?path=private-route", "reader", ""); w.Code != 403 {
		t.Fatalf("private route: %d", w.Code)
	}
}

// Seed a known indexed position in the disposable fixture without depending on
// a camera-specific JPEG encoder. Native routing reads exactly these index rows.
func seedPhotoRoutePosition(t *testing.T, s *Server) {
	t.Helper()
	db, err := sql.Open("sqlite", s.photos.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE media_index SET latitude=52.5,longitude=13.4 WHERE path='one.jpg'`); err != nil {
		t.Fatal(err)
	}
}
