package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/testutil/apicontract"
)

func TestImageGroupCreationReturnsToGallery(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	primary := "20260922_Trip & Family/nested_album/two.jpg"
	original, err := os.ReadFile(filepath.Join(s.photos.Root(), "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(filepath.Join(s.photos.Root(), primary)), 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.photos.Root(), primary), original, 0444); err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, back, primary, want string }{
		{"gallery context", "/photos?path=20260922_Trip+%26+Family&sort=ascending_name&type=image&page=2", primary, "/photos?path=20260922_Trip+%26+Family&sort=ascending_name&type=image&page=2"},
		{"root gallery", "/photos?recursive=1&q=family", primary, "/photos?recursive=1&q=family"},
		{"no return", "", primary, imageGalleryURL(primary)},
		{"root primary", "", "one.jpg", imageGalleryURL("one.jpg")},
		{"external", "https://unrelated.example/photos", primary, imageGalleryURL(primary)},
		{"network path", "//unrelated.example/photos", primary, imageGalleryURL(primary)},
		{"encoded network path", "/%2funrelated.example/photos", primary, imageGalleryURL(primary)},
		{"backslash", "/%5cunrelated.example/photos", primary, imageGalleryURL(primary)},
		{"group page", "/photos/image-groups/1", primary, imageGalleryURL(primary)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{"ids": {"one.jpg", primary}, "primary": {tc.primary}, "return": {tc.back}}
			r := httptest.NewRequest("POST", "/photos/image-groups", strings.NewReader(form.Encode()))
			r.SetBasicAuth("editor", "secret")
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 303 || w.Header().Get("Location") != withNotice(tc.want, "Bildgruppe erstellt.") {
				t.Fatalf("redirect: %d %s", w.Code, w.Header().Get("Location"))
			}
			media, err := s.photos.Media(tc.primary)
			if err != nil {
				t.Fatal(err)
			}
			group, err := s.photos.ImageGroup(ctx, media.ImageGroupID, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.photos.ApplyImageGroupAction(ctx, group.ID, group.Revision, 0, "dissolve", false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestImageGroupDissolutionReturnsToPrimaryFolder(t *testing.T) {
	for _, action := range []string{"dissolve", "remove"} {
		t.Run(action, func(t *testing.T) {
			s := faceTestServer(t)
			ctx := context.Background()
			primary := "20260922_Trip & Family/nested_album/two.jpg"
			original, err := os.ReadFile(filepath.Join(s.photos.Root(), "one.jpg"))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.MkdirAll(filepath.Dir(filepath.Join(s.photos.Root(), primary)), 0750); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(s.photos.Root(), primary), original, 0444); err != nil {
				t.Fatal(err)
			}
			if _, err = s.photos.RebuildIndex(ctx); err != nil {
				t.Fatal(err)
			}
			id, err := s.photos.CreateImageGroup(ctx, []string{"one.jpg", primary}, primary, false)
			if err != nil {
				t.Fatal(err)
			}
			group, err := s.photos.ImageGroup(ctx, id, false)
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{"action": {action}, "revision": {strconv.FormatInt(group.Revision, 10)}, "entity_id": {strconv.FormatInt(group.PrimaryID, 10)}}
			r := httptest.NewRequest("POST", imageGroupURL(id), strings.NewReader(form.Encode()))
			r.SetBasicAuth("editor", "secret")
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			location, err := url.Parse(w.Header().Get("Location"))
			if w.Code != 303 || err != nil || location.Path != "/photos" || location.Query().Get("path") != "20260922_Trip & Family/nested_album" {
				t.Fatalf("redirect: %d %s %v", w.Code, w.Header().Get("Location"), err)
			}
			if stale := faceRequest(s, "POST", imageGroupURL(id), "editor", form); stale.Code != 409 {
				t.Fatalf("already dissolved: %d %s", stale.Code, stale.Body)
			}
		})
	}
}

func TestImageGroupHTTPPermissionsValidationAndRevision(t *testing.T) {
	s := faceTestServer(t)
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(spec)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, user string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		w := faceRequest(s, method, path, user, form)
		canonical := "/photos/image-groups"
		if strings.HasPrefix(path, canonical+"/") {
			canonical += "/{id}"
		}
		if err := contract.ValidateResponse(canonical, method, w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatalf("%s %s: %v\n%s", method, path, err, w.Body)
		}
		return w
	}
	paths := []string{"one.jpg", "20240102_Family_Trip/nested_album/two.jpg", "three.jpg"}
	original, err := os.ReadFile(filepath.Join(s.photos.Root(), paths[0]))
	if err != nil {
		t.Fatal(err)
	}
	additional := "20250703_Another_Trip/four.jpg"
	for _, p := range append(paths[1:], additional) {
		if err = os.MkdirAll(filepath.Dir(filepath.Join(s.photos.Root(), p)), 0750); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(s.photos.Root(), p), original, 0444); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	create := url.Values{"ids": paths, "primary": {paths[1]}}
	for _, user := range []string{"reader", "unknown"} {
		w := request("POST", "/photos/image-groups", user, create)
		if w.Code != 403 && w.Code != 401 {
			t.Fatalf("%s: %d %s", user, w.Code, w.Body)
		}
	}
	for _, form := range []url.Values{
		{"ids": {paths[0]}, "primary": {paths[0]}},
		{"ids": {paths[0], paths[0]}, "primary": {paths[0]}},
		{"ids": paths, "primary": {"outside.jpg"}},
		{"ids": {paths[0], "../outside.jpg"}, "primary": {paths[0]}},
	} {
		w := request("POST", "/photos/image-groups", "editor", form)
		if w.Code != 400 {
			t.Fatalf("invalid %v: %d %s", form, w.Code, w.Body)
		}
	}
	r := httptest.NewRequest("POST", "/photos/image-groups", strings.NewReader(create.Encode()))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://unrelated.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-origin: %d", w.Code)
	}
	w = request("POST", "/photos/image-groups", "editor", create)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w = request("GET", created.URL, "reader", nil)
	var group imageGroupResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &group) != nil || len(group.Members) != 3 {
		t.Fatalf("read: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal(w.Header())
	}
	for _, m := range group.Members {
		if m.Path == paths[1] && (!m.Primary || m.DisplayPath != "Fotos / 02.01.2024 · Family Trip / nested album / two.jpg") {
			t.Fatalf("display: %+v", m)
		}
		if m.Path == paths[0] && m.DisplayPath != "Fotos / one.jpg" {
			t.Fatal(m.DisplayPath)
		}
	}
	for _, user := range []string{"reader", "editor"} {
		r = httptest.NewRequest("GET", created.URL, nil)
		r.SetBasicAuth(user, "secret")
		w = httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatalf("render: %d %s", w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "Als Hauptbild verwenden") != (user == "editor") {
			t.Fatal("edit controls exposed to reader")
		}
	}
	update := url.Values{"action": {"primary"}, "revision": {strconv.FormatInt(group.Revision, 10)}, "entity_id": {strconv.FormatInt(group.Members[0].EntityID, 10)}}
	w = request("POST", created.URL, "reader", update)
	if w.Code != 403 {
		t.Fatalf("reader edit: %d", w.Code)
	}
	w = request("POST", created.URL, "editor", update)
	if w.Code != 200 {
		t.Fatalf("primary: %d %s", w.Code, w.Body)
	}
	w = request("POST", created.URL, "editor", update)
	if w.Code != 409 {
		t.Fatalf("stale: %d %s", w.Code, w.Body)
	}
	w = request("GET", created.URL, "reader", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &group); err != nil {
		t.Fatal(err)
	}
	add := url.Values{"action": {"add"}, "revision": {strconv.FormatInt(group.Revision, 10)}, "ids": {additional}}
	if w = request("POST", created.URL, "reader", add); w.Code != 403 {
		t.Fatalf("reader add: %d", w.Code)
	}
	if w = request("POST", created.URL, "editor", add); w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	if w = request("POST", created.URL, "editor", add); w.Code != 409 {
		t.Fatalf("stale add: %d %s", w.Code, w.Body)
	}
	w = request("GET", created.URL, "reader", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &group); err != nil || len(group.Members) != 4 {
		t.Fatalf("members: %+v %v", group, err)
	}
	for _, m := range group.Members {
		if m.Primary && m.EntityID != group.Members[0].EntityID {
			t.Fatal("addition replaced primary")
		}
	}
	for _, p := range []string{"/photos/image-groups/no", "/photos/image-groups/0", "/photos/image-groups/999999"} {
		if w := request("GET", p, "reader", nil); w.Code != 404 {
			t.Fatalf("missing: %d", w.Code)
		}
	}
}
