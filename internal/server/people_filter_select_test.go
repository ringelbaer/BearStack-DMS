package server

import (
	"context"
	"encoding/json"
	"testing"

	"bearstack/internal/photos"
)

func TestPeopleFilterSelectionOverridesLegacyFlags(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	if err := s.photos.RenamePerson(context.Background(), photo.Faces[0].PersonID, "Known"); err != nil {
		t.Fatal(err)
	}
	if err := s.photos.EditFaces(context.Background(), []int64{photo.Faces[1].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode          string
		people, faces int
	}{{"all", 5, 0}, {"known", 1, 0}, {"unknown", 4, 0}, {"ignored", 0, 1}} {
		w := labelRequest(s, "GET", "/photos/people?format=json&filter="+tc.mode+"&unknown=1&known=1&ignored=1", "reader", "")
		var page photos.PeoplePage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.People) != tc.people || len(page.Faces) != tc.faces || page.KnownOnly != (tc.mode == "known") || page.UnknownOnly != (tc.mode == "unknown") || page.IgnoredOnly != (tc.mode == "ignored") {
			t.Fatalf("%s: %d %s", tc.mode, w.Code, w.Body.String())
		}
	}
	for _, mode := range []string{"", "invalid"} {
		if w := labelRequest(s, "GET", "/photos/people?filter="+mode, "reader", ""); w.Code != 400 {
			t.Fatalf("invalid mode: %d", w.Code)
		}
	}
}
