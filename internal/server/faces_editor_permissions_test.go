package server

import (
	"net/url"
	"testing"
)

func TestPhotoEditorCannotManageFaceRecognition(t *testing.T) {
	s := faceTestServer(t)
	for _, route := range []struct{ method, path string }{
		{"GET", "/settings/photos/faces"},
		{"POST", "/settings/photos/faces"},
		{"POST", "/settings/photos/faces/pause"},
		{"POST", "/settings/photos/faces/resume"},
		{"POST", "/settings/photos/faces/retry"},
		{"POST", "/settings/photos/faces/clear"},
	} {
		w := faceRequest(s, route.method, route.path, "editor", url.Values{"confirm": {"delete"}, "password": {"secret"}})
		if w.Code != 403 {
			t.Errorf("editor %s %s: %d", route.method, route.path, w.Code)
		}
	}
	for _, path := range []string{"/photos/people/999999/rename", "/photos/people/999999/merge", "/photos/faces/edit"} {
		form := url.Values{"name": {"Petra"}, "target": {"999998"}, "face_id": {"999999"}, "action": {"ignore"}}
		for _, user := range []string{"reader", "editor", "manager"} {
			w := faceRequest(s, "POST", path, user, form)
			if user == "reader" {
				if w.Code != 403 {
					t.Errorf("reader %s: %d", path, w.Code)
				}
			} else if w.Code != 400 && w.Code != 404 {
				t.Errorf("%s %s should reach validation: %d", user, path, w.Code)
			}
		}
	}
}
