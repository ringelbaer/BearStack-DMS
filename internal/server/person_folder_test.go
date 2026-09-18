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

func TestPersonFoldersHTTPPermissionsRevisionAndForms(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	id := group.Faces[0].PersonID
	endpoint := fmt.Sprintf("/photos/people/%d/folder", id)
	for _, tc := range []struct {
		user string
		code int
	}{{"", 401}, {"reader", 200}, {"editor", 200}} {
		w := labelRequest(s, "GET", endpoint, tc.user, "")
		if w.Code != tc.code {
			t.Fatalf("%s %d %s", tc.user, w.Code, w.Body.String())
		}
		if w.Code == 200 && (strings.Contains(w.Body.String(), "data-person-folder-form") != (tc.user == "editor") || !strings.Contains(w.Body.String(), "<h2>Fotos</h2>")) {
			t.Fatalf("UI: %s", w.Body.String())
		}
	}
	w := labelRequest(s, "GET", endpoint+"?format=json", "reader", "")
	var p photos.PersonFolderPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil || len(p.Folders) != 1 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	form := url.Values{"revision": {fmt.Sprint(p.Revision)}, "directory": {""}, "action": {"exclude"}}
	for _, tc := range []struct {
		user string
		code int
	}{{"", 401}, {"reader", 403}, {"editor", 303}} {
		w := faceRequest(s, "POST", endpoint, tc.user, form)
		if w.Code != tc.code {
			t.Fatalf("%s %d %s", tc.user, w.Code, w.Body.String())
		}
	}
	if w := faceRequest(s, "POST", endpoint, "editor", form); w.Code != 409 {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", endpoint, "editor", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Pfad wieder freigeben") {
		t.Fatalf("exclusion: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("CSRF: %d", w.Code)
	}
	// Other people's faces in the same directory retain their assignments.
	other, err := s.photos.Face(context.Background(), group.Faces[1].ID)
	if err != nil || other.PersonID != group.Faces[1].PersonID {
		t.Fatalf("unrelated face: %+v %v", other, err)
	}
}

func TestPersonHeadingShowsOnlyRecordedUndivorcedDetails(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	ctx := context.Background()
	id, other, ex := group.Faces[0].PersonID, group.Faces[1].PersonID, group.Faces[2].PersonID
	for i, p := range []int64{id, other, ex} {
		if err := s.photos.RenamePerson(ctx, p, []string{"Ada", "Current", "Former"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.photos.PersonDetails(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	err = s.photos.SetPersonDetails(ctx, id, photos.PersonDetailsInput{Revision: d.Revision, BirthDate: "1980-05-06", Marriages: []photos.PersonMarriageInput{{SpouseID: other, WeddingDate: "2010-01-02"}, {SpouseID: ex, WeddingDate: "2000-01-01", DivorceDate: "2005-01-01"}}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/photos/people/%d", id), nil)
	r.SetBasicAuth("reader", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body := w.Body.String()
	start := strings.Index(body, "<p data-person-summary")
	if start < 0 {
		t.Fatalf("missing summary: %s", body)
	}
	end := strings.Index(body[start:], "</p>")
	summary := body[start : start+end]
	if !strings.Contains(summary, "06.05.1980") || !strings.Contains(summary, "Current seit 02.01.2010") || strings.Contains(summary, "Former") || strings.Contains(summary, "Gestorben") {
		t.Fatal(summary)
	}
	if !strings.Contains(body, fmt.Sprintf(`/photos/people/%d/folder`, id)) {
		t.Fatal("missing folder navigation")
	}
	for _, route := range []string{"/photos/people/merge-suggestions", "/photos/people"} {
		w = labelRequest(s, "GET", route, "editor", "")
		if strings.Contains(w.Body.String(), "/people/chains") || strings.Contains(w.Body.String(), "app-face-chains") {
			t.Fatalf("remaining link on %s", route)
		}
	}
}
