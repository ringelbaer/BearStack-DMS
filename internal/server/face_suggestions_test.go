package server

import (
	"fmt"
	"strings"
	"testing"
)

func TestFacePersonSuggestionsHTTP(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	path := fmt.Sprintf("/photos/faces/%d/suggestions", photo.Faces[0].ID)
	for _, tt := range []struct {
		user   string
		status int
	}{{"", 401}, {"reader", 403}, {"editor", 200}, {"manager", 200}} {
		w := labelRequest(s, "GET", path, tt.user, "")
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.user, w.Code, w.Body.String())
		}
		if w.Code == 200 && (w.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(w.Body.String(), `"people":[]`)) {
			t.Fatalf("response: %v %s", w.Header(), w.Body.String())
		}
	}
	for _, tt := range []struct {
		id     string
		status int
	}{{"0", 400}, {"bad", 400}, {"999999", 404}} {
		w := labelRequest(s, "GET", "/photos/faces/"+tt.id+"/suggestions", "editor", "")
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.id, w.Code, w.Body.String())
		}
	}
}
