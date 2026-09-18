package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestFamilyTreeHTTPSettingsNavigationAndPermissions(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	ctx := context.Background()
	id, other := group.Faces[0].PersonID, group.Faces[1].PersonID
	for _, pid := range []int64{id, other} {
		if err := s.photos.RenamePerson(ctx, pid, fmt.Sprintf("Person %d", pid)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.photos.SetPersonParents(ctx, id, other, 0); err != nil {
		t.Fatal(err)
	}
	html := func(path, user string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.SetBasicAuth(user, "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	w := html("/photos", "reader")
	if strings.Contains(w.Body.String(), `href="/photos/family-tree"`) {
		t.Fatal("unset menu visible")
	}
	if w := html("/photos/family-tree", "reader"); w.Code != 404 {
		t.Fatalf("unset page: %d", w.Code)
	}
	settings, err := s.photos.FamilyTreeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"revision": {fmt.Sprint(settings.Revision)}, "person_id": {fmt.Sprint(id), fmt.Sprint(other)}}
	for _, tc := range []struct {
		user      string
		get, post int
	}{{"", 401, 401}, {"reader", 403, 403}, {"editor", 403, 403}, {"manager", 200, 303}} {
		w := labelRequest(s, "GET", "/settings/photos/family-tree", tc.user, "")
		if w.Code != tc.get {
			t.Fatalf("GET %s: %d %s", tc.user, w.Code, w.Body.String())
		}
		w = faceRequest(s, "POST", "/settings/photos/family-tree", tc.user, form)
		if w.Code != tc.post {
			t.Fatalf("POST %s: %d %s", tc.user, w.Code, w.Body.String())
		}
	}
	if w := faceRequest(s, "POST", "/settings/photos/family-tree", "manager", form); w.Code != 409 {
		t.Fatalf("stale write %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/settings/photos/family-tree", strings.NewReader(form.Encode()))
	r.SetBasicAuth("manager", "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin settings accepted")
	}
	for _, user := range []string{"reader", "editor", "manager"} {
		w = html("/photos", user)
		if !strings.Contains(w.Body.String(), `href="/photos/family-tree"`) {
			t.Fatalf("missing %s menu", user)
		}
		w = html("/photos/family-tree", user)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "data-tree-viewport") {
			t.Fatalf("view %s: %d", user, w.Code)
		}
	}
	w = labelRequest(s, "GET", "/photos/family-tree?format=json", "reader", "")
	var page photos.FamilyTreePage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Trees) != 1 || len(page.Trees[0].People) != 2 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("graph: %d %s", w.Code, w.Body.String())
	}
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateResponse("/photos/family-tree", "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	settings, err = s.photos.FamilyTreeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if w := faceRequest(s, "POST", "/settings/photos/family-tree", "manager", url.Values{"revision": {fmt.Sprint(settings.Revision)}}); w.Code != 303 {
		t.Fatalf("clear: %d", w.Code)
	}
	if w := html("/photos", "reader"); strings.Contains(w.Body.String(), `href="/photos/family-tree"`) {
		t.Fatal("cleared tree navigation remains")
	}
	if w := html("/settings/photos", "manager"); !strings.Contains(w.Body.String(), `href="/settings/photos/family-tree"`) {
		t.Fatal("configuration no longer reachable")
	}
	if w := labelRequest(s, "GET", "/photos/family-tree?format=json", "reader", ""); w.Code != 404 {
		t.Fatalf("cleared tree API: %d", w.Code)
	}
	savedPhotos := s.photos
	s.photos = nil
	defer func() { s.photos = savedPhotos }()
	if w := html("/settings/photos/family-tree", "manager"); w.Code != 404 {
		t.Fatalf("disabled module settings: %d", w.Code)
	}
}
