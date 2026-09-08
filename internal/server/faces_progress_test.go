package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFaceProgressHTTPUsesCountsWithoutDirectoryScan(t *testing.T) {
	s, _ := groupPhotoServerFixture(t)
	if err := os.Rename(s.photos.Root(), filepath.Join(t.TempDir(), "offline")); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"reader", "editor", "manager"} {
		w := labelRequest(s, "GET", "/settings/photos/faces?format=json&progress=1", user, "")
		want := 403
		if user == "manager" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s: %d %s", user, w.Code, w.Body.String())
		}
		if want == 200 {
			var view FaceSettingsView
			if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Status.Faces != 6 || len(view.Status.Errors) != 0 {
				t.Fatalf("view %+v %v", view, err)
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("cache policy")
			}
		}
	}
	if w := labelRequest(s, "GET", "/settings/photos/faces?format=json", "manager", ""); w.Code == 200 {
		t.Fatal("full status bypassed visibility")
	}
}
