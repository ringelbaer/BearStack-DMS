package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPersonDetailsHTTPPermissionsConflictAndValidation(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	id, other := group.Faces[0].PersonID, group.Faces[1].PersonID
	for _, pid := range []int64{id, other} {
		if err := s.photos.RenamePerson(context.Background(), pid, fmt.Sprintf("Person %d", pid)); err != nil {
			t.Fatal(err)
		}
	}
	endpoint := fmt.Sprintf("/photos/people/%d/details", id)
	w := labelRequest(s, "GET", endpoint, "reader", "")
	var details photos.PersonDetails
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &details) != nil || len(details.Revision) != 64 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("read: %d %s", w.Code, w.Body.String())
	}
	input := photos.PersonDetailsInput{Revision: details.Revision, BirthDate: "1970-01-01", SiblingIDs: []int64{other}, Marriages: []photos.PersonMarriageInput{{SpouseID: other, WeddingDate: "2000-01-01"}}}
	body, _ := json.Marshal(input)
	for _, tc := range []struct {
		user string
		code int
	}{{"", 401}, {"reader", 403}, {"editor", 200}} {
		w = labelRequest(s, "PUT", endpoint, tc.user, string(body))
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.user, w.Code, w.Body.String())
		}
	}
	w = labelRequest(s, "PUT", endpoint, "editor", string(body))
	if w.Code != 409 {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{`{`, string(body) + `{}`, `{"unexpected":true}`} {
		w = labelRequest(s, "PUT", endpoint, "editor", bad)
		if w.Code != 400 {
			t.Fatalf("invalid: %d %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("PUT", endpoint, strings.NewReader(string(body)))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://evil.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-origin: %d", w.Code)
	}
	for _, user := range []string{"reader", "editor"} {
		r := httptest.NewRequest("GET", fmt.Sprintf("/photos/people/%d", id), nil)
		r.SetBasicAuth(user, "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "data-person-details-open") || strings.Contains(w.Body.String(), "data-person-details-save") != (user == "editor") {
			t.Fatalf("UI %s: %d", user, w.Code)
		}
	}
}
