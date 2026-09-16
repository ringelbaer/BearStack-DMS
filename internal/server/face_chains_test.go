package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestFaceChainHTTPReviewAssignmentAndReceipt(t *testing.T) {
	s, _ := mergeSuggestionServer(t, false)
	const base = "/photos/people/chains"
	data, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	check := func(method, path string, w *httptest.ResponseRecorder) {
		t.Helper()
		if err := contract.ValidateResponse(path, method, w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest("GET", base, nil)
	r.SetBasicAuth("editor", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	check("GET", base, w)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "data-face-chains") || !strings.Contains(w.Body.String(), "app-face-chains.js") {
		t.Fatalf("HTML: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "POST", base+"/search", "editor", `{"hops":2}`)
	check("POST", base+"/search", w)
	var chain photos.FaceChain
	if err := json.Unmarshal(w.Body.Bytes(), &chain); err != nil || w.Code != 200 || len(chain.Groups) != 2 || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("search: %d %s %v", w.Code, w.Body.String(), err)
	}
	selection := photos.FaceChainSelection{Dataset: chain.Dataset}
	for _, g := range chain.Groups {
		selection.Groups = append(selection.Groups, g.LabelGroupRef)
	}
	body, _ := json.Marshal(photos.FaceChainPageRequest{FaceChainSelection: selection, Page: 1})
	w = labelRequest(s, "POST", base+"/faces", "editor", string(body))
	check("POST", base+"/faces", w)
	var page photos.FaceChainPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || page.Total != 2 || len(page.Faces) != 2 || page.Faces[0].FolderName == "" || strings.Contains(w.Body.String(), "embedding") {
		t.Fatalf("faces: %d %s %v", w.Code, w.Body.String(), err)
	}
	a := photos.FaceChainAssignment{FaceChainSelection: selection, OperationID: "chain-http-operation-0001", ExcludedFaces: []int64{page.Faces[0].ID}, Name: "Ada"}
	body, _ = json.Marshal(a)
	// The shared person dialog submits a form; recovery submits the same JSON.
	r = httptest.NewRequest("POST", base+"/assign", strings.NewReader(url.Values{"payload": {string(body)}}.Encode()))
	r.SetBasicAuth("editor", "secret")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	check("POST", base+"/assign", w)
	var saved struct {
		Receipt photos.LabelReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || w.Code != 200 || saved.Receipt.Action != "assign_chain" || saved.Receipt.Faces != 1 {
		t.Fatalf("assign: %d %s %v", w.Code, w.Body.String(), err)
	}
	first := w.Body.String()
	w = labelRequest(s, "POST", base+"/assign", "editor", string(body))
	if w.Code != 200 || w.Body.String() != first {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "GET", "/api/photos/labeling/v1/actions/"+a.OperationID+"?dataset="+a.Dataset, "editor", "")
	check("GET", "/api/photos/labeling/v1/actions/{operation}", w)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"action":"assign_chain"`) {
		t.Fatalf("receipt: %d %s", w.Code, w.Body.String())
	}
	if w := labelRequest(s, "POST", base+"/faces", "editor", string(mustChainJSON(t, photos.FaceChainPageRequest{FaceChainSelection: selection, Page: 1}))); w.Code != 409 {
		t.Fatalf("stale selection: %d %s", w.Code, w.Body.String())
	}
}

func mustChainJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestFaceChainHTTPPermissionsAndValidation(t *testing.T) {
	s := faceTestServer(t)
	const base = "/photos/people/chains"
	for _, path := range []string{"", "/search", "/faces", "/assign"} {
		method := "POST"
		if path == "" {
			method = "GET"
		}
		for _, user := range []string{"", "reader"} {
			want := 403
			if user == "" {
				want = 401
			}
			if w := labelRequest(s, method, base+path, user, `{}`); w.Code != want {
				t.Fatalf("permission %s %q: %d", path, user, w.Code)
			}
		}
		if method == "POST" {
			r := httptest.NewRequest(method, base+path, strings.NewReader(`{}`))
			r.SetBasicAuth("editor", "secret")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "https://attacker.invalid")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("cross-origin %s: %d", path, w.Code)
			}
		}
	}
	for _, tc := range []struct{ path, body string }{
		{"/search", `{"hops":0}`}, {"/search", `{"hops":6}`}, {"/search", `{"hops":2,"unknown":true}`},
		{"/search", `{"hops":2} {}`}, {"/search", strings.Repeat(" ", 512<<10) + `{"hops":2}`},
		{"/faces", `{}`}, {"/assign", `{}`},
	} {
		w := labelRequest(s, "POST", base+tc.path, "editor", tc.body)
		if w.Code != 400 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("validation %s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
