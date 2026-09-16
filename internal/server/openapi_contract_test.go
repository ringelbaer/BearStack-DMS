package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestOpenAPIHTTPResponses(t *testing.T) {
	data, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	s := faceTestServer(t)
	const base = "/api/photos/labeling/v1"
	check := func(method, path, canonical, user, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		var w *httptest.ResponseRecorder
		if canonical == "/photos/people/{id}/tags" || canonical == "/photos/people/{id}/parents" {
			form, err := url.ParseQuery(body)
			if err != nil {
				t.Fatal(err)
			}
			w = faceRequest(s, method, path, user, form)
		} else {
			w = labelRequest(s, method, path, user, body)
		}
		if w.Code != status {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		if err := contract.ValidateResponse(canonical, method, w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatalf("%s %s: %v\n%s", method, path, err, w.Body.String())
		}
		return w
	}
	for _, tc := range []struct {
		path, canonical, user string
		status                int
	}{
		{"/healthz", "/healthz", "reader", 200},
		{"/api/photos/v1/session", "/api/photos/v1/session", "reader", 200},
		{"/api/photos/v1/browse", "/api/photos/v1/browse", "reader", 200},
		{"/api/photos/v1/browse?page=-1", "/api/photos/v1/browse", "reader", 400},
		{base + "/session", base + "/session", "editor", 200},
		{base + "/session", base + "/session", "reader", 403},
		{base + "/session", base + "/session", "", 401},
		{base + "/candidates?upper=0", base + "/candidates", "editor", 200},
		{base + "/candidates", base + "/candidates", "editor", 400},
		{base + "/people?upper=0", base + "/people", "editor", 200},
		{base + "/people/999999", base + "/people/{id}", "editor", 404},
		{base + "/people/1?offset=-1", base + "/people/{id}", "editor", 400},
		{base + "/suggestions?q=Nobody", base + "/suggestions", "editor", 200},
		{base + "/merge-suggestions/next", base + "/merge-suggestions/next", "editor", 200},
	} {
		check("GET", tc.path, tc.canonical, tc.user, "", tc.status)
	}
	// Use real indexed detections and a committed action so item/detail/receipt
	// schemas are exercised with data, not only vacuous empty arrays.
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	v := make([]float32, 128)
	v[0] = 1
	if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: v}}}); err != nil {
		t.Fatal(err)
	}
	session, err := s.photos.LabelSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := check("GET", fmt.Sprintf("%s/candidates?upper=%d", base, session.UpperID), base+"/candidates", "editor", "", 200)
	var candidates photos.LabelCandidates
	if err = json.Unmarshal(w.Body.Bytes(), &candidates); err != nil || len(candidates.People) != 1 {
		t.Fatalf("candidates: %s %v", w.Body.String(), err)
	}
	person := candidates.People[0]
	check("POST", fmt.Sprintf("/photos/people/%d/tags", person.ID), "/photos/people/{id}/tags", "editor", "tags=family", 200)
	check("GET", "/api/photos/v1/browse?people=1", "/api/photos/v1/browse", "reader", "", 200)
	check("GET", "/api/photos/v1/browse?people=1&path=.people", "/api/photos/v1/browse", "reader", "", 200)
	check("GET", "/api/photos/v1/browse?people=1&path=.people/all", "/api/photos/v1/browse", "reader", "", 200)
	check("GET", "/api/photos/v1/browse?path=.people/f-", "/api/photos/v1/browse", "reader", "", 200)
	check("GET", fmt.Sprintf("/api/photos/v1/browse?path=.people/f-/%d", person.ID), "/api/photos/v1/browse", "reader", "", 200)
	check("GET", fmt.Sprintf("/api/photos/v1/browse?people=1&path=.people/all/%d", person.ID), "/api/photos/v1/browse", "reader", "", 200)
	faces, err := s.photos.AutomaticFaces(ctx, job.Path)
	if err != nil {
		t.Fatal(err)
	}
	check("GET", fmt.Sprintf("/api/photos/v1/faces/%d/thumbnail", faces[0].ID), "/api/photos/v1/faces/{id}/thumbnail", "reader", "", 200)

	path := fmt.Sprintf("%s/people/%d", base, person.ID)
	check("GET", path, base+"/people/{id}", "editor", "", 200)
	check("GET", fmt.Sprintf("%s/groups?upper=%d", base, session.UpperID), base+"/groups", "editor", "", 200)
	body, _ := json.Marshal(photos.LabelAction{OperationID: "contract-operation-12345", Dataset: session.Dataset, Revision: person.Revision, Action: "name", Name: "Contract Person"})
	check("POST", path+"/actions", base+"/people/{id}/actions", "editor", string(body), 200)
	check("POST", fmt.Sprintf("/photos/people/%d/parents", person.ID), "/photos/people/{id}/parents", "editor", "mother_id=0&father_id=0", 200)
	check("GET", fmt.Sprintf("/photos/people/%d/parents", person.ID), "/photos/people/{id}/parents", "reader", "", 200)
	check("GET", base+"/actions/contract-operation-12345?dataset="+session.Dataset, base+"/actions/{operation}", "editor", "", 200)
	check("POST", path+"/actions", base+"/people/{id}/actions", "editor", "{}", 400)
	check("GET", fmt.Sprintf("%s/people?upper=%d", base, session.UpperID), base+"/people", "editor", "", 200)
}
