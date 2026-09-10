package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnignorePhotoFacesHTTPPermissionsAndRevision(t *testing.T) {
	for _, scenario := range []string{"editor", "reader", "csrf", "stale", "private", "deleted", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			s, photo := groupPhotoServerFixture(t)
			ctx := context.Background()
			if err := s.photos.RenamePerson(ctx, photo.Faces[0].PersonID, "Anna"); err != nil {
				t.Fatal(err)
			}
			if err := s.photos.EditFaces(ctx, []int64{photo.Faces[0].ID, photo.Faces[1].ID}, 0, true, ""); err != nil {
				t.Fatal(err)
			}
			photo, _ = s.photos.PhotoFaces(ctx, photo.Path)
			form := url.Values{"path": {photo.Path}, "revision": {photo.Revision}}
			user, want := "editor", 200
			switch scenario {
			case "reader":
				user = "reader"
				want = 403
			case "csrf":
				want = 403
			case "stale":
				form.Set("revision", strings.Repeat("0", 64))
				want = 409
			case "private":
				if err := os.WriteFile(filepath.Join(s.photos.Root(), filepath.Dir(photo.Path), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				want = 403
			case "deleted":
				if err := os.Remove(filepath.Join(s.photos.Root(), photo.Path)); err != nil {
					t.Fatal(err)
				}
				want = 404
			case "invalid":
				form.Set("path", "../one.jpg")
				want = 400
			}
			request := httptest.NewRequest("POST", "/photos/faces/unignore", strings.NewReader(form.Encode()))
			request.SetBasicAuth(user, "secret")
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Accept", "application/json")
			if scenario == "csrf" {
				request.Header.Set("Origin", "https://other.test")
			}
			response := httptest.NewRecorder()
			s.Handler().ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("%d: %s", response.Code, response.Body.String())
			}
			if scenario == "editor" {
				if !strings.Contains(response.Body.String(), `"restored":2`) || response.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatalf("response: %s %s", response.Header(), response.Body.String())
				}
				face, _ := s.photos.Face(ctx, photo.Faces[0].ID)
				if face.Ignored || face.Name != "Anna" || face.PersonID != photo.Faces[0].PersonID {
					t.Fatalf("assignment changed: %+v", face)
				}
			} else if scenario != "private" && scenario != "deleted" {
				face, _ := s.photos.Face(ctx, photo.Faces[0].ID)
				if !face.Ignored {
					t.Fatal("failed request changed face")
				}
			}
		})
	}
}
