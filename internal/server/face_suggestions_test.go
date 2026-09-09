package server

import (
	"bearstack/internal/facerec"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
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

func TestFacePersonSuggestionsStream(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, 128)
	vector[0] = 1
	detections := []facerec.Detection{{X: .1, Y: .1, Width: .2, Height: .2, Confidence: .99, Embedding: vector}, {X: .5, Y: .1, Width: .2, Height: .2, Confidence: .99, Embedding: vector}}
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: detections}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, job.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.photos.RenamePerson(ctx, faces[1].PersonID, "Match"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/photos/faces/%d/suggestions", faces[0].ID), nil)
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Accept", "application/x-ndjson")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !w.Flushed || w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("stream headers: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("no interim frame: %s", w.Body.String())
	}
	for i, line := range lines {
		var event struct {
			People []struct {
				Name string `json:"name"`
			} `json:"people"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Done != (i == 1) || len(event.People) != 1 || event.People[0].Name != "Match" {
			t.Fatalf("frame: %s %v", line, err)
		}
	}
}
