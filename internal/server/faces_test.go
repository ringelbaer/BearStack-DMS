package server

import (
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/config"
	"bearstack/internal/facerec"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func faceTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "photos")
	if err := os.Mkdir(root, 0750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
		t.Fatal(err)
	}
	f.Close()
	repo, err := repository.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	store, err := storage.New(filepath.Join(dir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: dir, Addr: "127.0.0.1:0", Photos: config.PhotoConfig{Enabled: true, RootDir: root, DataDir: dir, CacheDir: filepath.Join(dir, "cache"), DBPath: filepath.Join(dir, "photos.db"), PageSize: 60}, Auth: config.AuthConfig{Credentials: []config.AuthCredential{{Username: "reader", Password: "secret", Role: "photos_read"}, {Username: "manager", Password: "secret", Role: "photos_manager"}, {Username: "editor", Password: "secret", Role: "photos_editor"}}}}
	s, err := New(cfg, repo, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.stopFaceRun(); s.faceWorker.run.Lock(); s.faceWorker.run.Unlock(); s.photos.Close() })
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}
func faceRequest(s *Server, method, path, user string, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	r.SetBasicAuth(user, "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestFaceRoutesPermissionsAndDisabledDefault(t *testing.T) {
	s := faceTestServer(t)
	for _, user := range []string{"reader", "editor", "manager"} {
		w := faceRequest(s, "GET", "/photos/people?format=json", user, nil)
		if w.Code != 200 {
			t.Fatalf("people %s %d %s", user, w.Code, w.Body.String())
		}
		w = faceRequest(s, "POST", "/photos/faces/edit", user, url.Values{"face_id": {"1"}, "action": {"ignore"}})
		if user != "manager" && w.Code != 403 {
			t.Errorf("edit permission %s %d", user, w.Code)
		}
	}
	w := faceRequest(s, "GET", "/settings/photos/faces?format=json", "manager", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("defaults %d %s", w.Code, w.Body.String())
	}
	w = faceRequest(s, "POST", "/settings/photos/faces", "manager", url.Values{"enabled": {"1"}})
	if w.Code == 303 {
		t.Fatal("enabled without service")
	}
	settings, _ := s.faceSettings(context.Background())
	if settings.Enabled {
		t.Fatal("failed activation persisted")
	}
	for _, path := range []string{"/photos/people", "/settings/photos/faces"} {
		r := httptest.NewRequest("GET", path, nil)
		r.SetBasicAuth("manager", "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatalf("render %s %d %s", path, w.Code, w.Body.String())
		}
	}
}
func TestFaceWorkerAndAPI(t *testing.T) {
	s := faceTestServer(t)
	token := strings.Repeat("a", 32)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("token")
		}
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(facerec.Health{Ready: true, Protocol: 1, Model: facerec.Model})
			return
		}
		v := make([]float32, 128)
		v[0] = 1
		_ = json.NewEncoder(w).Encode(facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: v}}})
	}))
	defer service.Close()
	s.cfg.Photos.FaceServiceURL = service.URL
	s.cfg.Photos.FaceServiceToken = token
	w := faceRequest(s, "POST", "/settings/photos/faces", "manager", url.Values{"enabled": {"1"}, "batch_size": {"1"}, "delay_millis": {"100"}})
	if w.Code != 303 {
		t.Fatalf("activation %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := s.photos.FaceStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if st.Done == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker not done %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
	f, err := s.photos.AutomaticFaces(context.Background(), "one.jpg")
	if err != nil || len(f) != 1 {
		t.Fatalf("faces %+v %v", f, err)
	}
	if err = s.photos.RenamePerson(context.Background(), f[0].PersonID, "Marie"); err != nil {
		t.Fatal(err)
	}
	w = faceRequest(s, "GET", "/photos/media/info?path=one.jpg", "reader", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"automatic_faces"`) || strings.Contains(w.Body.String(), "embedding") {
		t.Fatalf("API %d %s", w.Code, w.Body.String())
	}
	w = faceRequest(s, "POST", "/settings/photos/faces/clear", "manager", nil)
	if w.Code != 400 {
		t.Fatal("unconfirmed clear")
	}
	w = faceRequest(s, "POST", "/settings/photos/faces/clear", "manager", url.Values{"confirm": {"delete"}})
	if w.Code != 303 {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
	st, _ := s.photos.FaceStatus(context.Background())
	if st.Faces != 0 || st.Done != 0 {
		t.Fatalf("clear %+v", st)
	}
}

func TestFacePauseCancelsInference(t *testing.T) {
	s := faceTestServer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(facerec.Health{Ready: true, Protocol: 1, Model: facerec.Model})
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer service.Close()
	defer close(release)
	s.cfg.Photos.FaceServiceURL = service.URL
	s.cfg.Photos.FaceServiceToken = strings.Repeat("t", 32)
	if err := s.saveFaceSettings(context.Background(), FaceSettings{Enabled: true, BatchSize: 1, DelayMillis: 100, IntervalMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	if !s.startFaceRun() {
		t.Fatal("worker not started")
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("inference not started")
	}
	if s.startFaceRun() {
		t.Fatal("overlapping worker started")
	}
	w := faceRequest(s, "POST", "/settings/photos/faces/pause", "manager", nil)
	if w.Code != 303 {
		t.Fatalf("pause %d", w.Code)
	}
	finished := make(chan struct{})
	go func() { s.faceWorker.run.Lock(); s.faceWorker.run.Unlock(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("pause did not cancel inference")
	}
	status, err := s.photos.FaceStatus(context.Background())
	if err != nil || status.Queued != 1 || status.Done != 0 {
		t.Fatalf("cancel changed queue %+v %v", status, err)
	}
}

func TestFaceWorkerWaitsAfterBatchCompletion(t *testing.T) {
	finished := time.Date(2026, 9, 6, 10, 15, 0, 0, time.UTC)
	settings := FaceSettings{Enabled: true, IntervalMinutes: 15}
	start, wait := faceWorkerSchedule(settings, false, finished, finished.Add(5*time.Minute))
	if start || wait != 10*time.Minute {
		t.Fatalf("interval must follow completion: start=%v wait=%s", start, wait)
	}
	start, _ = faceWorkerSchedule(settings, false, finished, finished.Add(15*time.Minute))
	if !start {
		t.Fatal("due batch not scheduled")
	}
	start, _ = faceWorkerSchedule(settings, true, finished, finished.Add(time.Hour))
	if start {
		t.Fatal("overlapping batch scheduled")
	}
}
