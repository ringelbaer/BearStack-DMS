package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func TestIgnoredFacesFilterAndNamingHTTP(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	embedding := make([]float32, facerec.Dimensions)
	embedding[0] = 1
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: embedding}}}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, job.Path)
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces %v %v", faces, err)
	}
	id := faces[0].ID
	if err := s.photos.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"reader", "editor", "manager"} {
		request := httptest.NewRequest("GET", "/photos/people?ignored=1", nil)
		request.SetBasicAuth(user, "secret")
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		html := response.Body.String()
		if response.Code != 200 || !strings.Contains(html, `aria-current="page">Ignoriert</a>`) || strings.Contains(html, `type="search" name="q"`) || !strings.Contains(html, fmt.Sprintf("/photos/faces/%d/thumbnail", id)) {
			t.Fatalf("%s ignored view: %d %s", user, response.Code, html)
		}
		if strings.Contains(html, ">Wiederherstellen</button>") != (user != "reader") || strings.Contains(html, "data-ignored-edit") != (user != "reader") || strings.Contains(html, "data-person-dialog aria") != (user != "reader") {
			t.Fatalf("%s restore permissions", user)
		}
		if !strings.Contains(html, "Seite 1 von 1") {
			t.Fatal("missing single-page counter")
		}
		for _, label := range []string{"Erste Seite", "Letzte Seite"} {
			if strings.Contains(html, label) {
				t.Fatalf("unnecessary single-page navigation %q", label)
			}
		}
		if strings.Contains(html, "data-people-overview") {
			t.Fatal("ignored faces must not use the active-group merge UI")
		}
	}
	response := faceRequest(s, "GET", "/photos/people?ignored=1&format=json", "manager", nil)
	var page photos.PeoplePage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || !page.IgnoredOnly || len(page.Faces) != 1 || !page.Faces[0].Ignored {
		t.Fatalf("JSON: %s %v", response.Body.String(), err)
	}
	form := url.Values{"face_id": {fmt.Sprint(id)}, "action": {"move"}, "target": {"0"}, "name": {"Petra"}, "ignored": {"1"}, "q": {"Test & Name"}, "known": {"1"}, "page": {"2"}}
	denied := faceRequest(s, "POST", "/photos/faces/edit", "reader", form)
	if denied.Code != 403 {
		t.Fatalf("reader restored face: %d", denied.Code)
	}
	blank := url.Values{"face_id": {fmt.Sprint(id)}, "action": {"move"}, "ignored": {"1"}, "name": {"  "}}
	if response := faceRequest(s, "POST", "/photos/faces/edit", "manager", blank); response.Code != 400 {
		t.Fatalf("empty restore name accepted: %d", response.Code)
	}
	stillIgnored, err := s.photos.Face(ctx, id)
	if err != nil || !stillIgnored.Ignored {
		t.Fatalf("invalid restore changed face: %+v %v", stillIgnored, err)
	}
	request := httptest.NewRequest("POST", "/photos/faces/edit", strings.NewReader(form.Encode()))
	request.SetBasicAuth("manager", "secret")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil || response.Code != 303 || location.Path != "/photos/people" || location.Query().Get("ignored") != "1" || location.Query().Get("q") != "Test & Name" || location.Query().Get("known") != "1" || location.Query().Get("page") != "2" {
		t.Fatalf("redirect: %d %s %v", response.Code, response.Header(), err)
	}
	restored, err := s.photos.Face(ctx, id)
	if err != nil || restored.Ignored || restored.Name != "Petra" {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if err := s.photos.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	restore := url.Values{"face_id": {fmt.Sprint(id)}, "action": {"restore"}, "ignored": {"1"}, "target": {"0"}}
	if denied := faceRequest(s, "POST", "/photos/faces/edit", "reader", restore); denied.Code != 403 {
		t.Fatalf("reader restore: %d", denied.Code)
	}
	response = faceRequest(s, "POST", "/photos/faces/edit", "editor", restore)
	restored, err = s.photos.Face(ctx, id)
	if err != nil || response.Code != 200 || restored.Ignored || restored.Name != "" {
		t.Fatalf("unnamed restore: %d %+v %v", response.Code, restored, err)
	}
	replay := faceRequest(s, "POST", "/photos/faces/edit", "editor", restore)
	after, _ := s.photos.Face(ctx, id)
	if replay.Code != 409 || after.PersonID != restored.PersonID {
		t.Fatalf("stale restore moved active face: %d %+v", replay.Code, after)
	}

}
