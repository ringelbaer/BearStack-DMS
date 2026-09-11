package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestFaceMergeNamedConfirmationTemplate(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source, target string
		warning        bool
	}{
		{"", "", false}, {"Ada", "", false}, {"", "Grace", false},
		{"Ada", "Grace", true}, {"Alex", "Alex", true},
	} {
		t.Run(tc.source+"/"+tc.target, func(t *testing.T) {
			var out bytes.Buffer
			err := templates.ExecuteTemplate(&out, "face_merges.html", PageData{
				FaceMergeSuggestions: []photos.FaceMergeSuggestion{{ID: 1, SourceID: 2, TargetID: 3,
					SourceName: tc.source, TargetName: tc.target, SourceRevision: 4, TargetRevision: 5}},
			})
			if err != nil {
				t.Fatal(err)
			}
			html := out.String()
			if strings.Contains(html, "data-merge-warning=") != tc.warning || strings.Contains(html, "<noscript><label><input type=\"checkbox\" required>") != tc.warning {
				t.Fatal("confirmation must be required exactly when both groups are named")
			}
			if !strings.Contains(html, `formaction="/photos/people/merge-suggestions/1/reject" formnovalidate`) {
				t.Fatal("rejection must not require merge confirmation")
			}
		})
	}
}

func TestLabelingMergeHTTP(t *testing.T) {
	for _, action := range []string{"accept_merge", "reject_merge", "name_merge"} {
		t.Run(action, func(t *testing.T) {
			s, expected := mergeSuggestionServer(t, action != "name_merge")
			request := httptest.NewRequest("GET", "/photos/people/merge-suggestions", nil)
			request.SetBasicAuth("editor", "secret")
			html := httptest.NewRecorder()
			s.Handler().ServeHTTP(html, request)
			if strings.Contains(html.Body.String(), "data-merge-name") != (action == "name_merge") {
				t.Fatal("wrong pencil visibility")
			}
			sides := 1
			if action == "name_merge" {
				sides = 2
			}
			if strings.Count(html.Body.String(), "data-merge-side-name") != sides || strings.Count(html.Body.String(), "data-merge-ignore") != sides {
				t.Fatal("only unnamed groups may have individual controls")
			}
			if !strings.Contains(html.Body.String(), fmt.Sprintf("Ähnlichkeit: %.2f", expected.Score)) {
				t.Fatal("missing similarity in WebUI")
			}
			const path = "/api/photos/labeling/v1/merge-suggestions/next"
			for _, user := range []string{"reader", "editor", "manager"} {
				w := labelRequest(s, "GET", path, user, "")
				if user == "reader" {
					if w.Code != 403 {
						t.Fatalf("reader: %d", w.Code)
					}
					continue
				}
				var result struct {
					Suggestion photos.LabelMergeSuggestion `json:"suggestion"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Suggestion.ID != expected.ID || result.Suggestion.Score != expected.Score || len(result.Suggestion.Source.Faces) != 1 || len(result.Suggestion.Target.Faces) != 1 || w.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(w.Body.String(), "embedding") {
					t.Fatalf("%s: %d %s", user, w.Code, w.Body.String())
				}
			}
			session, err := s.photos.LabelSession(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			name := ""
			if action == "name_merge" {
				name = "Ada"
			}
			body, err := json.Marshal(photos.LabelAction{Action: action, Name: name, OperationID: "android-merge-0001", Dataset: session.Dataset,
				Revision: expected.SourceRevision, TargetID: expected.TargetID, TargetRevision: expected.TargetRevision, SuggestionID: expected.ID})
			if err != nil {
				t.Fatal(err)
			}
			actionPath := fmt.Sprintf("/api/photos/labeling/v1/people/%d/actions", expected.SourceID)
			if w := labelRequest(s, "POST", actionPath, "reader", string(body)); w.Code != 403 {
				t.Fatalf("reader action: %d", w.Code)
			}
			first := labelRequest(s, "POST", actionPath, "editor", string(body))
			if first.Code != 200 {
				t.Fatalf("action: %d %s", first.Code, first.Body.String())
			}
			again := labelRequest(s, "POST", actionPath, "editor", string(body))
			if again.Code != 200 || again.Body.String() != first.Body.String() {
				t.Fatalf("replay: %d %s", again.Code, again.Body.String())
			}
			if w := labelRequest(s, "GET", path, "editor", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"suggestion":null`) {
				t.Fatalf("next: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLabelingMergeSkipPairHTTP(t *testing.T) {
	s, pair := mergeSuggestionServer(t, false)
	for _, query := range []struct {
		value string
		code  int
	}{
		{fmt.Sprintf("?exclude_source=%d&exclude_target=%d", pair.SourceID, pair.TargetID), 200},
		{fmt.Sprintf("?exclude_source=%d&exclude_target=%d", pair.TargetID, pair.SourceID), 200},
		{"?exclude_source=1", 400}, {"?exclude_target=1", 400},
		{"?exclude_source=-1&exclude_target=2", 400}, {"?exclude_source=1&exclude_target=1", 400},
		{"?exclude_source=abc&exclude_target=2", 400},
	} {
		w := labelRequest(s, "GET", "/api/photos/labeling/v1/merge-suggestions/next"+query.value, "editor", "")
		if w.Code != query.code || (w.Code == 200 && !strings.Contains(w.Body.String(), `"suggestion":null`)) {
			t.Fatalf("%s: %d %s", query.value, w.Code, w.Body.String())
		}
	}
	if w := labelRequest(s, "GET", "/api/photos/labeling/v1/merge-suggestions/next", "editor", ""); w.Code != 200 || strings.Contains(w.Body.String(), `"suggestion":null`) {
		t.Fatalf("skip must not reject or delete pair: %d %s", w.Code, w.Body.String())
	}
}
