package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPeopleSortHTTPAndValidation(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	for i, f := range photo.Faces {
		if err := s.photos.RenamePerson(context.Background(), f.PersonID, fmt.Sprintf("Person %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"name", "count", "folder", "date"} {
		for _, direction := range []string{"asc", "desc"} {
			sorting := key + "_" + direction
			w := labelRequest(s, "GET", "/photos/people?format=json&sort="+sorting, "reader", "")
			var page photos.PeoplePage
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || page.Sort != sorting || len(page.People) != 6 {
				t.Fatalf("%s: %d %+v %v", sorting, w.Code, page, err)
			}
			if direction == "desc" && page.People[0].ID != photo.Faces[5].PersonID {
				t.Fatalf("descending %s: %+v", sorting, page.People)
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("missing cache protection")
			}
		}
	}
	for _, sort := range []string{"name; DROP TABLE photo_people", "folder", "DATE_DESC"} {
		for _, filter := range []string{"all", "ignored"} {
			if w := labelRequest(s, "GET", "/photos/people?filter="+filter+"&sort="+url.QueryEscape(sort), "reader", ""); w.Code != 400 {
				t.Fatalf("invalid sort accepted: %d", w.Code)
			}
		}
	}
	// Person details and autocomplete retain their own fixed ordering.
	for _, path := range []string{fmt.Sprintf("/photos/people/%d?format=json&sort=invalid", photo.Faces[0].PersonID), "/photos/people?format=suggestions&sort=invalid&q=Person"} {
		if w := labelRequest(s, "GET", path, "reader", ""); w.Code != 200 {
			t.Fatalf("unrelated mode rejected sort: %d", w.Code)
		}
	}
	if w := labelRequest(s, "GET", "/photos/people?sort=count_desc", "", ""); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestPeopleSortHTMLPaginationAndIgnoredRestore(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "people.html", PageData{People: photos.PeoplePage{Page: 2, TotalPages: 3, HasPrev: true, HasNext: true, UnknownOnly: true, Sort: "date_desc"}}); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Count(html, "&amp;sort=date_desc") != 8 || strings.Count(html, "&amp;unknown=1") != 4 || !strings.Contains(html, `value="date_desc" selected`) {
		t.Fatalf("sort control/links lost: %s", html)
	}
	if strings.Contains(html, `Alle Filter aufheben`) {
		t.Fatal("obsolete reset link must not be shown")
	}
	s, photo := groupPhotoServerFixture(t)
	face := photo.Faces[0]
	if err := s.photos.EditFaces(context.Background(), []int64{face.ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/photos/people?filter=ignored&sort=folder_desc", nil)
	request.SetBasicAuth("editor", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `name="sort" value="folder_desc"`) {
		t.Fatalf("restore form lost sort: %d", w.Code)
	}
	form := url.Values{"face_id": {fmt.Sprint(face.ID)}, "target": {"0"}, "action": {"restore"}, "ignored": {"1"}, "sort": {"folder_desc"}, "q": {""}, "page": {"2"}}
	request = httptest.NewRequest("POST", "/photos/faces/edit", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("editor", "secret")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || w.Code != 303 || location.Query().Get("sort") != "folder_desc" {
		t.Fatalf("restore lost sorting: %d %s %v", w.Code, w.Header().Get("Location"), err)
	}
}
