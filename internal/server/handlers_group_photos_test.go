package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func groupPhotoServerFixture(t *testing.T) (*Server, photos.GroupPhoto) {
	t.Helper()
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	detections := make([]facerec.Detection, 6)
	for i := range detections {
		vector := make([]float32, 128)
		vector[i] = 1
		detections[i] = facerec.Detection{X: .05 + .23*float64(i%4), Y: .05 + .3*float64(i/4), Width: .2, Height: .25, Confidence: .99, Embedding: vector}
	}
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: detections}); err != nil {
		t.Fatal(err)
	}
	photo, err := s.photos.GroupPhoto(ctx, job.Path)
	if err != nil {
		t.Fatal(err)
	}
	return s, photo
}

func TestGroupPhotosHTTPContract(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	for _, user := range []string{"", "reader", "editor", "manager"} {
		want := 200
		if user == "" {
			want = 401
		}
		if user == "reader" {
			want = 403
		}
		w := labelRequest(s, "GET", "/photos/people/groups?format=json", user, "")
		if w.Code != want {
			t.Fatalf("%s: %d %s", user, w.Code, w.Body.String())
		}
		if want == 200 {
			var result photos.GroupPhotosPage
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Minimum != 5 || result.Photo == nil || result.Photo.Remaining != 6 {
				t.Fatalf("page: %s", w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("cache policy")
			}
		}
	}
	for _, query := range []string{"min=-1", "min=256", "min=x", "path=", "path=..%2Fone.jpg", "after=..%2Fone.jpg"} {
		if w := labelRequest(s, "GET", "/photos/people/groups?format=json&"+query, "editor", ""); w.Code != 400 {
			t.Fatalf("invalid %s: %d %s", query, w.Code, w.Body.String())
		}
	}
	if w := labelRequest(s, "GET", "/photos/people/groups?format=json&min=6", "editor", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"photo":null`) {
		t.Fatalf("threshold: %d %s", w.Code, w.Body.String())
	}
	if w := labelRequest(s, "GET", "/photos/people/groups?format=json&after="+url.QueryEscape(photo.Path), "editor", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"photo":null`) {
		t.Fatalf("cursor: %d %s", w.Code, w.Body.String())
	}
	for _, user := range []string{"editor", "reader"} {
		r := httptest.NewRequest("GET", "/photos/people", nil)
		r.SetBasicAuth(user, "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), `href="/photos/people/groups"`) != (user == "editor") {
			t.Fatalf("entry button %s %d", user, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/photos/people/groups", nil)
	r.SetBasicAuth("editor", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || strings.Count(w.Body.String(), `data-group-face=`) != 6 || !strings.Contains(w.Body.String(), "data-person-dialog") {
		t.Fatalf("HTML: %d %s", w.Code, w.Body.String())
	}
	if err := s.photos.RenamePerson(context.Background(), photo.Faces[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	w = labelRequest(s, "GET", "/photos/people/groups?format=json&path="+url.QueryEscape(photo.Path), "editor", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"remaining":5`) {
		t.Fatalf("retained current: %d %s", w.Code, w.Body.String())
	}
}

func TestGroupPhotoIgnoreHTTPProtection(t *testing.T) {
	for _, single := range []bool{false, true} {
		for _, scenario := range []string{"success", "reader", "csrf", "stale", "invalid", "private", "deleted", "fallback"} {
			t.Run(fmt.Sprintf("single=%t/%s", single, scenario), func(t *testing.T) {
				s, photo := groupPhotoServerFixture(t)
				user := "editor"
				form := url.Values{"path": {photo.Path}, "revision": {photo.Revision}, "min": {"5"}}
				if single {
					form.Set("face_id", fmt.Sprint(photo.Faces[0].ID))
				}
				want := 200
				switch scenario {
				case "reader":
					user = "reader"
					want = 403
				case "csrf":
					want = 403
				case "stale":
					if err := s.photos.RenamePerson(context.Background(), photo.Faces[0].PersonID, "Ada"); err != nil {
						t.Fatal(err)
					}
					want = 409
				case "invalid":
					form.Set("revision", "")
					want = 400
				case "private":
					if err := os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
						t.Fatal(err)
					}
					want = 403
				case "deleted":
					if err := os.Remove(filepath.Join(s.photos.Root(), photo.Path)); err != nil {
						t.Fatal(err)
					}
					want = 404
				case "fallback":
					want = 303
				}
				r := httptest.NewRequest("POST", "/photos/people/groups/ignore", strings.NewReader(form.Encode()))
				r.SetBasicAuth(user, "secret")
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if scenario != "fallback" {
					r.Header.Set("Accept", "application/json")
				}
				if scenario == "csrf" {
					r.Header.Set("Origin", "https://foreign.example")
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				if w.Code != want {
					t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
				}
				if scenario == "fallback" {
					location := "/photos/people/groups?after=one.jpg&min=5"
					if single {
						location = "/photos/people/groups?min=5&path=one.jpg"
					}
					if w.Header().Get("Location") != location {
						t.Fatalf("redirect %s", w.Header().Get("Location"))
					}
				}
				if scenario == "success" {
					count := 6
					if single {
						count = 1
					}
					if !strings.Contains(w.Body.String(), fmt.Sprintf(`"ignored":%d`, count)) {
						t.Fatalf("wrong count: %s", w.Body.String())
					}
				}
				if scenario == "private" || scenario == "deleted" {
					return
				}
				faces, err := s.photos.AutomaticFaces(context.Background(), photo.Path)
				if err != nil {
					t.Fatal(err)
				}
				expected := 6
				if scenario == "success" || scenario == "fallback" {
					expected = 0
					if single {
						expected = 5
					}
				}
				if len(faces) != expected {
					t.Fatalf("mutated protected faces: %d", len(faces))
				}
			})
		}
	}
}

func TestGroupPhotoImagePermissionsAndExclusions(t *testing.T) {
	for _, scenario := range []string{"editor", "reader", "ignored", "private", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			s, photo := groupPhotoServerFixture(t)
			user := "editor"
			want := 200
			switch scenario {
			case "reader":
				user = "reader"
				want = 403
			case "ignored":
				if _, err := s.photos.IgnoreGroupPhoto(context.Background(), photo.Path, photo.Revision); err != nil {
					t.Fatal(err)
				}
			case "private":
				if err := os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				want = 403
			case "deleted":
				if err := os.Remove(filepath.Join(s.photos.Root(), photo.Path)); err != nil {
					t.Fatal(err)
				}
				want = 404
			}
			w := labelRequest(s, "GET", fmt.Sprintf("/photos/people/groups/image/%d", photo.ImageFaceID), user, "")
			if w.Code != want {
				t.Fatalf("image %d want %d %s", w.Code, want, w.Body.String())
			}
			if want == 200 && (!strings.HasPrefix(w.Header().Get("Content-Type"), "image/") || !strings.Contains(w.Header().Get("Cache-Control"), "no-store")) {
				t.Fatalf("image headers %v", w.Header())
			}
		})
	}
}
