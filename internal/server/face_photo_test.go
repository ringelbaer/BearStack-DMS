package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"bearstack/internal/facerec"
)

func TestAnalyzePhotoFacesPermissionsAndPausedWorker(t *testing.T) {
	s := faceTestServer(t)
	w := faceRequest(s, "GET", "/photos/faces?path=one.jpg", "editor", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"faces":[]`) {
		t.Fatalf("unanalyzed photo: %d %s", w.Code, w.Body.String())
	}
	var calls atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		vector := make([]float32, 128)
		vector[0] = 1
		_ = json.NewEncoder(w).Encode(facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .2, Y: .2, Width: .3, Height: .3, Confidence: .99, Embedding: vector}}})
	}))
	defer service.Close()
	s.cfg.Photos.FaceServiceURL = service.URL
	s.cfg.Photos.FaceServiceToken = strings.Repeat("x", 32)
	for _, user := range []string{"reader", "editor", "manager"} {
		w := faceRequest(s, "POST", "/photos/faces/analyze", user, url.Values{"path": {"one.jpg"}})
		if user == "reader" {
			if w.Code != 403 || calls.Load() != 0 {
				t.Fatalf("reader: %d %d", w.Code, calls.Load())
			}
			continue
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), `"faces":[{`) {
			t.Fatalf("%s: %d %s", user, w.Code, w.Body.String())
		}
	}
	settings, err := s.faceSettings(context.Background())
	if err != nil || settings.Enabled {
		t.Fatalf("enabled background processing: %+v %v", settings, err)
	}
	w = faceRequest(s, "POST", "/photos/faces/analyze", "editor", url.Values{"path": {"../one.jpg"}})
	if w.Code != 400 {
		t.Fatalf("traversal: %d", w.Code)
	}
	s.cfg.Photos.FaceServiceURL = ""
	w = faceRequest(s, "POST", "/photos/faces/analyze", "editor", url.Values{"path": {"one.jpg"}})
	if w.Code != 503 {
		t.Fatalf("missing service: %d", w.Code)
	}
}

func TestPhotoFacesEmptyResultAndReadPermissions(t *testing.T) {
	s := faceTestServer(t)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{}})
	}))
	defer service.Close()
	s.cfg.Photos.FaceServiceURL = service.URL
	s.cfg.Photos.FaceServiceToken = strings.Repeat("x", 32)
	w := faceRequest(s, "POST", "/photos/faces/analyze", "editor", url.Values{"path": {"one.jpg"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"faces":[]`) {
		t.Fatalf("no detections: %d %s", w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		user string
		path string
		code int
	}{{"editor", "one.jpg", 200}, {"reader", "one.jpg", 403}, {"editor", "../one.jpg", 400}, {"editor", "missing.jpg", 404}} {
		w = faceRequest(s, "GET", "/photos/faces?path="+url.QueryEscape(tc.path), tc.user, nil)
		if w.Code != tc.code {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}

func TestFaceAnalysisGateCanBeCancelled(t *testing.T) {
	s := faceTestServer(t)
	release, err := s.acquireFaceAnalysis(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.acquireFaceAnalysis(ctx); err != context.Canceled {
		t.Fatalf("cancelled wait: %v", err)
	}
}
