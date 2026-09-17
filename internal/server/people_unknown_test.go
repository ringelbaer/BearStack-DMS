package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestUnknownPeopleHTTPFilterAndReset(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	ctx := context.Background()
	if err := s.photos.RenamePerson(ctx, photo.Faces[0].PersonID, "Named"); err != nil {
		t.Fatal(err)
	}
	if err := s.photos.EditFaces(ctx, []int64{photo.Faces[1].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"", "reader", "editor", "manager"} {
		for _, extra := range []string{"", "&known=1&ignored=1"} {
			w := labelRequest(s, "GET", "/photos/people?format=json&unknown=1"+extra, user, "")
			if user == "" {
				if w.Code != 401 {
					t.Fatalf("anonymous: %d", w.Code)
				}
				continue
			}
			var page photos.PeoplePage
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || !page.UnknownOnly || page.KnownOnly || page.IgnoredOnly || len(page.People) != 4 || len(page.Faces) != 0 {
				t.Fatalf("%s %s: %d %+v %v", user, extra, w.Code, page, err)
			}
			for _, person := range page.People {
				if person.Name != "" || person.FaceID == photo.Faces[1].ID {
					t.Fatalf("excluded person suggested: %+v", person)
				}
			}
		}
	}
	request := httptest.NewRequest("GET", "/photos/people?unknown=1&known=1&ignored=1", nil)
	request.SetBasicAuth("reader", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	html := w.Body.String()
	if w.Code != 200 || !strings.Contains(html, `name="filter" value="unknown"`) || strings.Contains(html, `type="search" name="q"`) || !strings.Contains(html, `aria-current="page">Unbenannt</a>`) || strings.Contains(html, `name="filter" value="known"`) || strings.Contains(html, `name="filter" value="ignored"`) || strings.Contains(html, `Alle Filter aufheben`) {
		t.Fatalf("normalized filters: %d %s", w.Code, html)
	}
	request = httptest.NewRequest("GET", "/photos/people?page=1&q=", nil)
	request.SetBasicAuth("reader", "secret")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if w.Code != 200 || strings.Count(w.Body.String(), `class="person-overview-card"`) != 5 || strings.Contains(w.Body.String(), `name="filter" value="unknown"`) {
		t.Fatalf("reset did not restore unfiltered active groups: %d %s", w.Code, w.Body.String())
	}
}

func TestUnknownPeoplePaginationLinks(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "people.html", PageData{People: photos.PeoplePage{Page: 2, TotalPages: 3, HasPrev: true, HasNext: true, UnknownOnly: true}}); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Count(html, "&amp;unknown=1") != 4 || strings.Contains(html, "&amp;known=1") || strings.Contains(html, "&amp;ignored=1") {
		t.Fatalf("pagination lost unknown filter: %s", html)
	}
}

func TestPeopleSourceFolderHeadingsAndSelectionPermissions(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"all", "known", "unknown", "ignored"} {
		for _, canEdit := range []bool{false, true} {
			for _, directory := range []string{"Fotos", `2026/2026_07_15_Urlaub_&_ <Meer>`} {
				page := photos.PeoplePage{Page: 1, TotalPages: 1, KnownOnly: mode == "known", UnknownOnly: mode == "unknown", IgnoredOnly: mode == "ignored"}
				page.People = []photos.Person{{ID: 1, FaceID: 2, Directory: directory, Count: 1}}
				photoPath := "photo.jpg"
				if directory != "Fotos" {
					photoPath = directory + "/photo.jpg"
				}
				page.People[0].FolderName = photos.MediaFolderName(photoPath)
				page.People[0].DisplayPath = photos.MediaDisplayPath(photoPath)
				page.Faces = []photos.RecognizedFace{{ID: 2, PersonID: 1, Path: photoPath}}
				var out bytes.Buffer
				if err := templates.ExecuteTemplate(&out, "people.html", PageData{People: page, Auth: AuthPermissions{CanPhotosEdit: canEdit}}); err != nil {
					t.Fatal(err)
				}
				html := out.String()
				if got, want := strings.Contains(html, "data-people-selection-mode"), canEdit && mode == "unknown"; got != want {
					t.Fatalf("mode=%s edit=%v: selection mode=%v, want %v", mode, canEdit, got, want)
				}
				folder, image := strings.Index(html, "<span data-person-folder"), strings.Index(html, `<img loading="lazy"`)
				if folder < 0 || image < 0 || (folder < image) != (mode == "unknown" || mode == "ignored") {
					t.Fatalf("mode=%s: wrong folder/image order", mode)
				}
				folderEnd := folder + strings.Index(html[folder:], "</span>") + len("</span>")
				name := "Fotos"
				if directory != "Fotos" {
					name = "Urlaub &amp; &lt;Meer&gt;"
				}
				if !strings.Contains(html[folder:folderEnd], ">"+name+"</span>") {
					t.Fatalf("mode=%s: missing escaped folder basename: %s", mode, html[folder:folderEnd])
				}
			}
		}
	}
}
