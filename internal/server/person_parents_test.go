package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPersonParentsHTTP(t *testing.T) {
	s, group := groupPhotoServerFixture(t)
	child, mother := group.Faces[0].PersonID, group.Faces[1].PersonID
	for _, v := range []struct {
		id   int64
		name string
	}{{child, "Kind"}, {mother, "Mutter"}} {
		if err := s.photos.RenamePerson(context.Background(), v.id, v.name); err != nil {
			t.Fatal(err)
		}
	}
	endpoint := fmt.Sprintf("/photos/people/%d/parents", child)
	for _, tc := range []struct {
		user string
		code int
	}{{"", 401}, {"reader", 403}, {"editor", 200}} {
		w := faceRequest(s, "POST", endpoint, tc.user, url.Values{"mother_id": {fmt.Sprint(mother)}, "father_id": {"0"}})
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.user, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/photos/people/%d", child), nil)
	r.SetBasicAuth("reader", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`href="/photos/people/%d">Mutter`, mother)) || strings.Contains(w.Body.String(), "Eltern speichern") {
		t.Fatalf("read-only detail: %d %s", w.Code, w.Body.String())
	}
	w = faceRequest(s, "POST", endpoint, "editor", url.Values{"mother_id": {fmt.Sprint(child)}, "father_id": {"0"}})
	if w.Code != 400 {
		t.Fatalf("self-parent: %d", w.Code)
	}
}
