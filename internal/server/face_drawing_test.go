package server

import (
	"context"
	"net/url"
	"testing"
)

func TestFaceDrawingRoutes(t *testing.T) {
	s := faceTestServer(t)
	w := faceRequest(s, "GET", "/photos/faces/drawing-image?path=one.jpg", "editor", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	form := url.Values{"path": {"one.jpg"}, "source_revision": {w.Header().Get("X-Photo-Source-Revision")}, "x": {"0.2"}, "y": {"0.2"}, "width": {"0.3"}, "height": {"0.3"}, "name": {"Manual"}}
	if len(form.Get("source_revision")) != 64 {
		t.Fatal("missing source revision")
	}
	for _, method := range []string{"GET", "POST"} {
		path := "/photos/faces/manual"
		if method == "GET" {
			path = "/photos/faces/drawing-image?path=one.jpg"
		}
		if denied := faceRequest(s, method, path, "reader", form); denied.Code != 403 {
			t.Fatalf("reader %s: %d", method, denied.Code)
		}
	}
	w = faceRequest(s, "POST", "/photos/faces/manual", "editor", form)
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	photo, err := s.photos.PhotoFaces(context.Background(), "one.jpg")
	if err != nil || len(photo.Faces) != 1 || !photo.Faces[0].Drawn || photo.Faces[0].Name != "Manual" {
		t.Fatalf("saved: %+v %v", photo, err)
	}
	w = faceRequest(s, "POST", "/photos/faces/manual", "editor", form)
	if w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	form.Set("width", "NaN")
	w = faceRequest(s, "POST", "/photos/faces/manual", "editor", form)
	if w.Code != 400 {
		t.Fatalf("invalid: %d", w.Code)
	}
	settings, err := s.faceSettings(context.Background())
	if err != nil || settings.Enabled {
		t.Fatalf("global processing changed: %+v %v", settings, err)
	}
}
