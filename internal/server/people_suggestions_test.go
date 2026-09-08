package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPeopleSuggestionsHTTPContractAndVisibility(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	if err := s.photos.RenamePerson(context.Background(), photo.Faces[0].PersonID, "Jürgen"); err != nil {
		t.Fatal(err)
	}
	const path = "/photos/people?format=suggestions&q=Juergen"
	for _, user := range []string{"", "reader", "editor", "manager"} {
		w := labelRequest(s, "GET", path, user, "")
		if user == "" {
			if w.Code != 401 {
				t.Fatalf("anonymous: %d", w.Code)
			}
			continue
		}
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: %d %v %s", user, w.Code, w.Header(), w.Body.String())
		}
		var result photos.PeopleSuggestions
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.People) != 1 || result.People[0].Name != "Jürgen" || result.People[0].Count != 1 || result.People[0].FaceID != photo.Faces[0].ID || result.HasNext {
			t.Fatalf("suggestions: %+v %v", result, err)
		}
		thumbnail := labelRequest(s, "GET", fmt.Sprintf("/photos/faces/%d/thumbnail", result.People[0].FaceID), user, "")
		if thumbnail.Code != 200 || !strings.HasPrefix(thumbnail.Header().Get("Content-Type"), "image/") {
			t.Fatalf("%s suggestion thumbnail: %d %s", user, thumbnail.Code, thumbnail.Body.String())
		}
		for _, unused := range []string{`"total_pages"`, `"page"`} {
			if strings.Contains(w.Body.String(), unused) {
				t.Fatalf("unused metadata %s: %s", unused, w.Body.String())
			}
		}
	}
	if w := labelRequest(s, "GET", "/photos/people?format=json&known=1", "reader", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"total_pages":1`) || !strings.Contains(w.Body.String(), `"face_id":`) {
		t.Fatalf("overview contract changed: %d %s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(filepath.Join(s.photos.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if w := labelRequest(s, "GET", path, "reader", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"people":[]`) {
		t.Fatalf("new marker ignored: %d %s", w.Code, w.Body.String())
	}
	if w := labelRequest(s, "GET", fmt.Sprintf("/photos/faces/%d/thumbnail", photo.Faces[0].ID), "reader", ""); w.Code == 200 {
		t.Fatal("previously suggested thumbnail remained accessible after protection")
	}
}
