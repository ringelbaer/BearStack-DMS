package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestLabelFoldersHTTP(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	id := group.Faces[0].PersonID
	if err := s.photos.RenamePerson(context.Background(), id, "Ada"); err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf("/api/photos/labeling/v1/people/%d/folders", id)
	data, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user   string
		status int
	}{{"", 401}, {"reader", 403}, {"editor", 200}} {
		w := labelRequest(s, "GET", endpoint, tc.user, "")
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.user, w.Code, w.Body.String())
		}
		if err := contract.ValidateResponse("/api/photos/labeling/v1/people/{id}/folders", "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"?page=0", "?page=-1", "?page=1000001", "?page=invalid"} {
		if w := labelRequest(s, "GET", endpoint+query, "editor", ""); w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
	w := labelRequest(s, "GET", endpoint, "editor", "")
	var page photos.PersonFolderPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" || len(page.Folders) != 1 || page.Folders[0].DisplayPath != "Fotos" || len(page.Folders[0].Faces) != 1 {
		t.Fatalf("page: %s", w.Body.String())
	}
	session, _ := s.photos.LabelSession(context.Background())
	body := fmt.Sprintf(`{"operation_id":"native-folder-exclude","dataset":%q,"revision":%d,"directory":"","action":"folder_exclude"}`, session.Dataset, page.Revision)
	actionPath := fmt.Sprintf("/api/photos/labeling/v1/people/%d/actions", id)
	for _, tc := range []struct {
		user   string
		status int
	}{{"", 401}, {"reader", 403}, {"editor", 200}} {
		w := labelRequest(s, "POST", actionPath, tc.user, body)
		if w.Code != tc.status {
			t.Fatalf("write %s: %d %s", tc.user, w.Code, w.Body.String())
		}
		if err := contract.ValidateResponse("/api/photos/labeling/v1/people/{id}/actions", "POST", w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest("POST", actionPath, strings.NewReader(body))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://evil.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("CSRF %d", w.Code)
	}
	if w := labelRequest(s, "POST", actionPath, "editor", body); w.Code != 200 {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", endpoint, "editor", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"excluded":true`) {
		t.Fatalf("excluded-only: %d %s", w.Code, w.Body.String())
	}
	listPath := fmt.Sprintf("/api/photos/labeling/v1/people?upper=%d&include_excluded=1&q=Ada", session.UpperID)
	w = labelRequest(s, "GET", listPath, "editor", "")
	if w.Code != 200 {
		t.Fatalf("excluded people: %d %s", w.Code, w.Body.String())
	}
	if err := contract.ValidateResponse("/api/photos/labeling/v1/people", "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	var people photos.LabelCandidates
	if err := json.Unmarshal(w.Body.Bytes(), &people); err != nil || len(people.People) != 1 || people.People[0].ID != id || people.People[0].Count != 0 {
		t.Fatalf("excluded people: %s %v", w.Body.String(), err)
	}

}
