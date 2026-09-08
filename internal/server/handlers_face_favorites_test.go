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

func favoriteServerFixture(t *testing.T) (*Server, photos.RecognizedFace) {
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
	vector := make([]float32, 128)
	vector[0] = 1
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: vector}}}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, "one.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces %v %v", faces, err)
	}
	return s, faces[0]
}

func TestFaceFavoriteAPI(t *testing.T) {
	s, f := favoriteServerFixture(t)
	path := fmt.Sprintf("/api/photos/labeling/v1/faces/%d/favorite", f.ID)
	body := fmt.Sprintf(`{"person_id":%d,"favorite":true}`, f.PersonID)
	for _, user := range []string{"", "reader", "editor", "manager"} {
		want := 200
		if user == "" {
			want = 401
		}
		if user == "reader" {
			want = 403
		}
		for _, method := range []string{"GET", "PUT"} {
			w := labelRequest(s, method, path, user, body)
			if w.Code != want {
				t.Fatalf("%s %s: %d %s", user, method, w.Code, w.Body.String())
			}
		}
	}
	for _, body := range []string{`{}`, `{"person_id":1}`, `{"favorite":true}`, `{"person_id":1,"favorite":null}`, `{"person_id":1,"favorite":"true"}`, `{"person_id":1,"favorite":false,"unexpected":1}`, `{"person_id":1,"favorite":false} {}`, `null`, strings.Repeat(" ", 17<<10)} {
		if w := labelRequest(s, "PUT", path, "editor", body); w.Code != 400 {
			t.Errorf("invalid body %q: %d", body[:min(len(body), 80)], w.Code)
		}
	}
	if w := labelRequest(s, "PUT", path, "editor", fmt.Sprintf(`{"person_id":%d,"favorite":false}`, f.PersonID+1)); w.Code != 409 {
		t.Fatalf("membership %d", w.Code)
	}
	if w := labelRequest(s, "PUT", "/api/photos/labeling/v1/faces/9999/favorite", "editor", body); w.Code != 404 {
		t.Fatalf("absent %d", w.Code)
	}
	for _, method := range []string{"GET", "PUT"} {
		w := labelRequest(s, method, path, "editor", body)
		var out photos.FaceFavorite
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || !out.Favorite || out.ID != f.ID || out.PersonID != f.PersonID {
			t.Fatalf("state %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("missing no-store")
		}
	}
	session := labelRequest(s, "GET", "/api/photos/labeling/v1/session", "editor", "")
	if !strings.Contains(session.Body.String(), `"face_favorites":true`) {
		t.Fatalf("feature discovery: %s", session.Body.String())
	}
	group := labelRequest(s, "GET", fmt.Sprintf("/api/photos/labeling/v1/people/%d", f.PersonID), "editor", "")
	if group.Code != 200 || !strings.Contains(group.Body.String(), `"favorite":true`) {
		t.Fatalf("group metadata: %d %s", group.Code, group.Body.String())
	}
	w := labelRequest(s, "PUT", path, "editor", fmt.Sprintf(`{"person_id":%d,"favorite":false}`, f.PersonID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"favorite":false`) {
		t.Fatalf("explicit false: %d %s", w.Code, w.Body.String())
	}
}

func TestFaceFavoriteProtectionAndWeb(t *testing.T) {
	for _, scenario := range []string{"web", "ignored", "private", "deleted", "csrf"} {
		t.Run(scenario, func(t *testing.T) {
			s, f := favoriteServerFixture(t)
			ctx := context.Background()
			endpoint := fmt.Sprintf("/api/photos/labeling/v1/faces/%d/favorite", f.ID)
			body := fmt.Sprintf(`{"person_id":%d,"favorite":true}`, f.PersonID)
			switch scenario {
			case "web":
				if err := s.photos.RenamePerson(ctx, f.PersonID, "Ada"); err != nil {
					t.Fatal(err)
				}
				if _, err := s.photos.SetFaceFavorite(ctx, f.ID, f.PersonID, true); err != nil {
					t.Fatal(err)
				}
				for _, user := range []string{"editor", "reader"} {
					r := httptest.NewRequest("GET", fmt.Sprintf("/photos/people/%d", f.PersonID), nil)
					r.SetBasicAuth(user, "secret")
					w := httptest.NewRecorder()
					s.Handler().ServeHTTP(w, r)
					if w.Code != 200 {
						t.Fatalf("html %d %s", w.Code, w.Body.String())
					}
					if strings.Contains(w.Body.String(), `data-face-favorite="`) != (user == "editor") {
						t.Fatalf("favorite control for %s", user)
					}
					if !strings.Contains(w.Body.String(), "Favorisiert") && !strings.Contains(w.Body.String(), "Favorisierung aufheben") {
						t.Fatal("favorite status missing")
					}
				}
				form := url.Values{"person_id": {fmt.Sprint(f.PersonID)}, "favorite": {"0"}, "page": {"3"}}
				w := faceRequest(s, "POST", fmt.Sprintf("/photos/faces/%d/favorite", f.ID), "editor", form)
				if w.Code != 303 || w.Header().Get("Location") != fmt.Sprintf("/photos/people/%d?page=3", f.PersonID) {
					t.Fatalf("fallback %d %s", w.Code, w.Header().Get("Location"))
				}
				face, err := s.photos.Face(ctx, f.ID)
				if err != nil || face.Favorite {
					t.Fatal("fallback did not unfavorite")
				}
				return
			case "ignored":
				if err := s.photos.EditFaces(ctx, []int64{f.ID}, 0, true, ""); err != nil {
					t.Fatal(err)
				}
			case "private":
				if err := os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := os.Remove(filepath.Join(s.photos.Root(), "one.jpg")); err != nil {
					t.Fatal(err)
				}
			case "csrf":
				r := httptest.NewRequest("PUT", endpoint, strings.NewReader(body))
				r.SetBasicAuth("editor", "secret")
				r.Header.Set("Origin", "https://foreign.example")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				if w.Code != 403 {
					t.Fatalf("CSRF %d", w.Code)
				}
				face, err := s.photos.Face(ctx, f.ID)
				if err != nil || face.Favorite {
					t.Fatal("CSRF changed favorite")
				}
				return
			}
			for _, method := range []string{"GET", "PUT"} {
				w := labelRequest(s, method, endpoint, "editor", body)
				if w.Code != 403 && w.Code != 404 && !(scenario == "ignored" && method == "PUT" && w.Code == 409) {
					t.Fatalf("excluded %s: %d %s", method, w.Code, w.Body.String())
				}
			}
		})
	}
}
