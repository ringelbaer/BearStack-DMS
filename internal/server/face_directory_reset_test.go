package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func directoryResetServerFixture(t *testing.T) (*Server, photos.GroupPhoto) {
	t.Helper()
	s := faceTestServer(t)
	root := s.photos.Root()
	image, err := os.ReadFile(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	path := "2011/20111015-Party/one.jpg"
	if err = os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, path), image, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.PrepareFacePhoto(ctx, path, facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	detections := make([]facerec.Detection, 2)
	for i := range detections {
		v := make([]float32, 128)
		v[i] = 1
		detections[i] = facerec.Detection{X: .1 + float64(i)*.3, Y: .1, Width: .2, Height: .2, Confidence: .99, Embedding: v}
	}
	if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: detections}); err != nil {
		t.Fatal(err)
	}
	photo, err := s.photos.PhotoFaces(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.photos.RenamePerson(ctx, photo.Faces[1].PersonID, "Anna"); err != nil {
		t.Fatal(err)
	}
	if err = s.photos.EditFaces(ctx, []int64{photo.Faces[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	return s, photo
}

func TestResetIgnoredDirectoryHTTPPermissionsAndScope(t *testing.T) {
	for _, scenario := range []string{"editor", "manager", "reader", "anonymous", "csrf", "root", "year", "traversal", "virtual", "missing", "private", "get", "html", "html-invalid", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			s, photo := directoryResetServerFixture(t)
			directory := "2011/20111015-Party"
			path, user, method, want := directory, "editor", "POST", 200
			switch scenario {
			case "manager":
				user = "manager"
			case "reader":
				user = "reader"
				want = 403
			case "anonymous":
				user = ""
				want = 401
			case "csrf":
				want = 403
			case "root":
				path = ""
				want = 400
			case "year":
				path = "2011"
				want = 400
			case "traversal":
				path = "2011/20111015-Party/.."
				want = 400
			case "virtual":
				path = ".people/all"
				want = 400
			case "missing":
				path = "2011/missing"
				want = 404
			case "private":
				if err := os.WriteFile(filepath.Join(s.photos.Root(), directory, ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				want = 403
			case "get":
				method = "GET"
				want = 405
			case "html":
				want = 303
			case "html-invalid":
				path, want = "2011", 400
			case "symlink":
				if err := os.Symlink(filepath.Join(s.photos.Root(), directory), filepath.Join(s.photos.Root(), "2011/link")); err != nil {
					t.Fatal(err)
				}
				path, want = "2011/link", 400
			}
			r := httptest.NewRequest(method, "/photos/faces/reset-ignored-directory", strings.NewReader(url.Values{"path": {path}, "return": {"https://evil.test/"}}.Encode()))
			if user != "" {
				r.SetBasicAuth(user, "secret")
			}
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if scenario != "html" && scenario != "html-invalid" {
				r.Header.Set("Accept", "application/json")
			}
			if scenario == "csrf" {
				r.Header.Set("Origin", "https://evil.test")
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
			}
			if want == 200 {
				var result struct {
					OK       bool `json:"ok"`
					Restored int  `json:"restored"`
				}
				if json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.OK || result.Restored != 1 {
					t.Fatalf("response: %s", w.Body.String())
				}
			}
			if want == 303 {
				u, err := url.Parse(w.Header().Get("Location"))
				if err != nil || u.Host != "" || u.Path != "/photos" || u.Query().Get("path") != directory || !strings.Contains(u.Query().Get("notice"), "1 ignorierte") {
					t.Fatalf("redirect: %s %v", u, err)
				}
				follow := httptest.NewRequest("GET", u.String(), nil)
				follow.SetBasicAuth(user, "secret")
				page := httptest.NewRecorder()
				s.Handler().ServeHTTP(page, follow)
				if page.Code != 200 || !strings.Contains(page.Body.String(), "1 ignorierte Gesichter im Ordner und seinen Unterordnern zurückgesetzt.") {
					t.Fatalf("missing result notice: %d", page.Code)
				}
			}
			if want == 200 || want == 303 {
				if w.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("cache policy")
				}
				face, err := s.photos.Face(context.Background(), photo.Faces[0].ID)
				if err != nil || face.Ignored || face.Name != "" {
					t.Fatalf("restored: %+v %v", face, err)
				}
				named, err := s.photos.Face(context.Background(), photo.Faces[1].ID)
				if err != nil || named.Name != "Anna" || named.Ignored {
					t.Fatalf("named changed: %+v %v", named, err)
				}
			} else if scenario != "private" {
				face, err := s.photos.Face(context.Background(), photo.Faces[0].ID)
				if err != nil || !face.Ignored {
					t.Fatalf("rejected request wrote face: %+v %v", face, err)
				}
			}
		})
	}
}

func TestResetIgnoredDirectoryMenuDepthAndPermissions(t *testing.T) {
	s, _ := directoryResetServerFixture(t)
	for _, user := range []string{"reader", "editor", "manager"} {
		for _, path := range []string{"", "2011", "2011/20111015-Party"} {
			r := httptest.NewRequest("GET", "/photos?path="+url.QueryEscape(path), nil)
			r.SetBasicAuth(user, "secret")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			want := user != "reader" && path == "2011/20111015-Party"
			if w.Code != 200 || strings.Contains(w.Body.String(), "Alle ignorierten Gesichter zurücksetzen") != want {
				t.Fatalf("%s %s: %d menu=%v", user, path, w.Code, want)
			}
		}
	}
	for _, path := range []string{".people", ".people/all", ".people/all/123"} {
		filter := photoFilterFromRequest(httptest.NewRequest("GET", "/photos", nil), photos.Listing{Path: path})
		if filter.CanResetIgnoredFaces {
			t.Fatalf("virtual folder: %s", path)
		}
	}
}
