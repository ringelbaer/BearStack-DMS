package server

import (
	"encoding/json"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPhotoMapRouteIsReadOnlyAndValidatesViewport(t *testing.T) {
	s := faceTestServer(t)
	for _, q := range []string{"", "south=-90&west=-180&north=90&east=180", "south=-10&west=170&north=10&east=-170"} {
		w := labelRequest(s, "GET", "/api/photos/v1/map?"+q, "reader", "")
		var result photos.MapResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Markers == nil || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("map: %d %s", w.Code, w.Body.String())
		}
	}
	for _, q := range []string{"south=0", "south=NaN&west=0&north=1&east=1", "south=-91&west=0&north=1&east=1", "south=0&west=0&north=1&east=0",
		"south=0&west=0&north=Inf&east=1", "south=0&west=0&north=x&east=1", "q=" + strings.Repeat("a", 801), "type=sql", "path=../outside"} {
		w := labelRequest(s, "GET", "/api/photos/v1/map?"+q, "reader", "")
		if w.Code != 400 {
			t.Errorf("%s: %d %s", q, w.Code, w.Body.String())
		}
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/map", "", ""); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := labelRequest(s, "POST", "/api/photos/v1/map", "reader", "{}"); w.Code != 405 {
		t.Fatalf("map mutation: %d", w.Code)
	}
	for _, test := range []struct {
		query  string
		status int
	}{
		{"south=-90&west=-180&north=90&east=180", 200},
		{"", 400}, {"south=1", 400}, {"south=-90&west=-180&north=90&east=180&page=0", 400},
		{"south=-90&west=-180&north=90&east=180&page=1000001", 400},
	} {
		w := labelRequest(s, "GET", "/api/photos/v1/map/media?"+test.query, "reader", "")
		if w.Code != test.status {
			t.Errorf("map media %s: %d %s", test.query, w.Code, w.Body.String())
		}
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/map/media", "", ""); w.Code != 401 {
		t.Fatalf("map media auth: %d", w.Code)
	}
}
