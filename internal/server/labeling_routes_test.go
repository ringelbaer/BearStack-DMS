package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLabelingRoutesPreserveValidationPermissionsAndCacheHeaders(t *testing.T) {
	s := faceTestServer(t)
	session, err := s.photos.LabelSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const base = "/api/photos/labeling/v1"
	for _, test := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/session", "", 200, ""},
		{"GET", "/candidates?upper=0", "", 200, ""},
		{"GET", "/people?upper=0", "", 200, ""},
		{"GET", "/people", "", 400, "invalid"},
		{"GET", "/people?upper=1&after=-1", "", 400, "invalid"},
		{"GET", "/people?upper=bad", "", 400, "invalid"},
		{"GET", "/people?upper=1&q=" + strings.Repeat("x", 801), "", 400, "invalid"},
		{"GET", "/people/1?limit=41", "", 400, "invalid"},
		{"GET", "/people/1?limit=0", "", 400, "invalid"},
		{"GET", "/people/1?after_face=-1", "", 400, "invalid"},
		{"GET", "/suggestions?q=Nobody", "", 200, ""},
		{"GET", "/people/999999", "", 404, "not_found"},
		{"GET", "/actions/missing-operation?dataset=" + session.Dataset, "", 404, "not_found"},
		{"GET", "/faces/999999/thumbnail", "", 404, "not_found"},
		{"GET", "/faces/999999/original", "", 404, "not_found"},
		{"POST", "/people/1/actions", `{"unknown":true}`, 400, "invalid"},
		{"POST", "/people/1/actions", `{} {}`, 400, "invalid"},
		{"POST", "/people/1/actions", strings.Repeat(" ", 16<<10) + `{}`, 400, "invalid"},
		{"GET", "/candidates", "", 400, "invalid"},
		{"GET", "/candidates?upper=1&after=-1", "", 400, "invalid"},
		{"GET", "/suggestions?q=" + strings.Repeat("x", 801), "", 400, "invalid"},
		{"GET", "/people/invalid", "", 400, "invalid"},
		{"GET", "/faces/invalid/thumbnail", "", 400, "invalid"},
		{"GET", "/faces/invalid/original", "", 400, "invalid"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			w := labelRequest(s, test.method, base+test.path, "editor", test.body)
			if w.Code != test.status || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("response: %d, headers %v, %s", w.Code, w.Header(), w.Body.String())
			}
			if test.code != "" {
				var result struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != test.code {
					t.Fatalf("error body: %s (%v)", w.Body.String(), err)
				}
			}
			if w := labelRequest(s, test.method, base+test.path, "reader", test.body); w.Code != 403 {
				t.Fatalf("reader admitted: %d", w.Code)
			}
		})
	}
}
