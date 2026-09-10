package server

import (
	"bearstack/internal/photos"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestManualGroupMergeHTTP(t *testing.T) {
	s, _ := mergeSuggestionServer(t, true)
	session, err := s.photos.LabelSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/photos/labeling/v1/groups?upper=%d&include_named=1", session.UpperID)
	for _, test := range []struct {
		user   string
		status int
	}{{"", 401}, {"reader", 403}, {"editor", 200}, {"manager", 200}} {
		if w := labelRequest(s, "GET", path, test.user, ""); w.Code != test.status {
			t.Fatalf("%s: %d %s", test.user, w.Code, w.Body)
		}
	}
	for _, query := range []string{"", "?upper=abc", "?upper=-1", "?upper=10&include_named=yes", "?upper=10&after=-1"} {
		if w := labelRequest(s, "GET", "/api/photos/labeling/v1/groups"+query, "editor", ""); w.Code != 400 {
			t.Fatalf("query %q: %d", query, w.Code)
		}
	}
	w := labelRequest(s, "GET", path, "editor", "")
	var page photos.LabelCandidates
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.People) < 2 || w.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(w.Body.String(), "embedding") {
		t.Fatalf("page: %s", w.Body)
	}
	a := photos.LabelAction{OperationID: "manual-merge-http-0001", Dataset: session.Dataset, Action: "merge_groups", Revision: page.People[0].Revision}
	for _, p := range page.People {
		a.Groups = append(a.Groups, photos.LabelGroupRef{ID: p.ID, Revision: p.Revision})
	}
	body, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	actionPath := fmt.Sprintf("/api/photos/labeling/v1/people/%d/actions", page.People[0].ID)
	if w := labelRequest(s, "POST", actionPath, "reader", string(body)); w.Code != 403 {
		t.Fatalf("reader mutation: %d", w.Code)
	}
	first := labelRequest(s, "POST", actionPath, "editor", string(body))
	again := labelRequest(s, "POST", actionPath, "editor", string(body))
	if first.Code != 200 || again.Code != 200 || first.Body.String() != again.Body.String() {
		t.Fatalf("receipted merge: %d %s / %d %s", first.Code, first.Body, again.Code, again.Body)
	}
}
