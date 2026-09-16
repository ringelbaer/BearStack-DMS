package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPersonTagsHTTPAndGallery(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	id := group.Faces[0].PersonID
	endpoint := fmt.Sprintf("/photos/people/%d/tags", id)
	for _, tc := range []struct {
		user string
		code int
	}{{"", 401}, {"reader", 403}, {"editor", 200}} {
		w := faceRequest(s, "POST", endpoint, tc.user, url.Values{"tags": {"Family", "travel"}})
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.user, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", endpoint, strings.NewReader("tags=bad"))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.invalid")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-origin write: %d", w.Code)
	}
	person, err := s.photos.People(context.Background(), id, 1, "", false, false)
	if err != nil || strings.Join(person.Tags, ",") != "family,travel" {
		t.Fatalf("tags changed: %+v %v", person, err)
	}
	if err := s.photos.RenamePerson(context.Background(), id, "Zoe"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/photos", "/photos?path=.people", "/photos?path=" + url.QueryEscape(photos.PersonFolderPath(id))} {
		r := httptest.NewRequest("GET", path, nil)
		r.SetBasicAuth("reader", "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if path == "/photos" && (!strings.Contains(w.Body.String(), "photo-people-preview") || !strings.Contains(w.Body.String(), "/photos/faces/")) {
			t.Fatal("missing face folder")
		}
		if strings.Contains(path, "all") && (!strings.Contains(w.Body.String(), "photo-date-group") || !strings.Contains(w.Body.String(), "one.jpg")) {
			t.Fatal("missing normal dated gallery")
		}
	}
	for _, user := range []string{"reader", "editor"} {
		for _, path := range []string{"", ".people", ".people/all", photos.PersonFolderPath(id), ".people/t-ZmFtaWx5/" + fmt.Sprint(id), photos.DirectoryPeoplePath("") + "/" + fmt.Sprint(id)} {
			w := labelRequest(s, "GET", "/photos?path="+url.QueryEscape(path), user, "")
			want := user == "editor" && strings.Count(path, "/") == 2
			if w.Code != 200 || strings.Contains(w.Body.String(), `aria-label="Person bearbeiten"`) != want {
				t.Fatalf("edit link %s %s: %d", user, path, w.Code)
			}
			if want && !strings.Contains(w.Body.String(), fmt.Sprintf(`href="/photos/people/%d" aria-label="Person bearbeiten"`, id)) {
				t.Fatal("incorrect person edit target")
			}
		}
	}

	for _, user := range []string{"reader", "editor"} {
		r := httptest.NewRequest("GET", fmt.Sprintf("/photos/people/%d", id), nil)
		r.SetBasicAuth(user, "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), `aria-label="Personen-Tags bearbeiten"`) != (user == "editor") {
			t.Fatalf("tag control %s: %d", user, w.Code)
		}
	}
}

func TestPeopleFolderCatalogOptInAndFaceReadAccess(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	settings, err := s.photoSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.FolderPreviewCount = 2
	if err := s.savePhotoSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?people=1", "?people=1&recursive=1", "?people=1&section=media"} {
		w := labelRequest(s, "GET", "/api/photos/v1/browse"+query, "reader", "")
		var page photoCatalogPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
		var virtual *photoCatalogFolder
		for i := range page.Folders {
			if page.Folders[i].Virtual {
				virtual = &page.Folders[i]
			}
		}
		if (virtual != nil) != (query == "?people=1") {
			t.Fatalf("unexpected virtual folder: %s %+v", query, page.Folders)
		}
		if virtual != nil && (virtual.Path != photos.PeopleFolderPath || len(virtual.Previews) != 4 || virtual.Previews[0].FaceID == 0) {
			t.Fatalf("previews: %+v", virtual)
		}
	}
	w := labelRequest(s, "GET", "/api/photos/v1/browse?people=1&path=.people%2Fall", "reader", "")
	var page photoCatalogPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Folders) != 0 || page.Name != "Alle" || page.Parent != ".people" {
		t.Fatalf("directory: %d %s", w.Code, w.Body.String())
	}
	if err := s.photos.RenamePerson(context.Background(), group.Faces[0].PersonID, "Zoe"); err != nil {
		t.Fatal(err)
	}
	w = labelRequest(s, "GET", "/api/photos/v1/browse?people=1&path=.people%2Fall", "reader", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Folders) != 1 || page.Folders[0].Name != "Zoe" {
		t.Fatalf("named directory: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", "/photos?path=.people%2Fall", "reader", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Zoe") || strings.Contains(w.Body.String(), "Unbenannt") {
		t.Fatalf("named web directory: %d %s", w.Code, w.Body.String())
	}
	for _, user := range []string{"", "reader", "editor"} {
		w := labelRequest(s, "GET", fmt.Sprintf("/api/photos/v1/faces/%d/thumbnail", group.Faces[0].ID), user, "")
		want := 200
		if user == "" {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("thumbnail %s: %d %s", user, w.Code, w.Body.String())
		}
	}
}

func TestDirectoryPeopleCatalogAndMenu(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	if err := s.photos.RenamePerson(context.Background(), group.Faces[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	w := labelRequest(s, "GET", "/api/photos/v1/browse?people=1", "reader", "")
	var root photoCatalogPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &root) != nil || root.PeoplePath != photos.DirectoryPeoplePath("") {
		t.Fatalf("root: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", "/api/photos/v1/browse?path="+url.QueryEscape(root.PeoplePath), "reader", "")
	var people photoCatalogPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &people) != nil || len(people.Folders) != 1 || people.FolderTotal != 1 || people.Folders[0].Name != "Ada" || people.Parent != "" || people.PeoplePath != "" {
		t.Fatalf("people: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/photos", nil)
	r.SetBasicAuth("reader", "secret")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Personen im Ordner") {
		t.Fatalf("menu: %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/photos?path="+url.QueryEscape(people.Folders[0].Path), nil)
	r.SetBasicAuth("reader", "secret")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "photo-date-group") {
		t.Fatalf("gallery: %d %s", w.Code, w.Body.String())
	}
}

func TestPeopleSearchResetKeepsModeAndSort(t *testing.T) {
	s, _ := groupPhotoServerFixture(t)
	r := httptest.NewRequest("GET", "/photos/people?known=1&q=missing&page=3&sort=count_desc", nil)
	r.SetBasicAuth("reader", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `aria-label="Personensuche zurücksetzen" href="/photos/people?filter=known&amp;sort=count_desc&amp;q="`) {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
}
