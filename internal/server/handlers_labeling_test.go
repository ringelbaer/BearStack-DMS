package server

import (
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"net/http/httptest"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func labelRequest(s *Server, method, path, user, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		r.SetBasicAuth(user, "secret")
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestLabelingHTTPContractPermissionsAndImages(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	const base = "/api/photos/labeling/v1"
	for _, test := range []struct {
		user   string
		status int
	}{{"", 401}, {"reader", 403}, {"editor", 403}, {"manager", 200}} {
		w := labelRequest(s, "GET", base+"/session", test.user, "")
		if w.Code != test.status {
			t.Fatalf("%s %d %s", test.user, w.Code, w.Body.String())
		}
	}
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
	session, _ := s.photos.LabelSession(ctx)
	w := labelRequest(s, "GET", fmt.Sprintf("%s/candidates?upper=%d", base, session.UpperID), "manager", "")
	var candidates photos.LabelCandidates
	if err = json.Unmarshal(w.Body.Bytes(), &candidates); err != nil || len(candidates.People) != 1 {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
	p := candidates.People[0]
	if strings.Contains(w.Body.String(), "embedding") {
		t.Fatal("embedding exposed")
	}
	for _, size := range []int{160, 640} {
		w = labelRequest(s, "GET", fmt.Sprintf("%s/faces/%d/thumbnail?size=%d", base, p.FaceID, size), "manager", "")
		img, e := jpeg.Decode(w.Body)
		if e != nil || img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Fatalf("thumbnail %d %v", w.Code, e)
		}
	}
	a := photos.LabelAction{OperationID: "http-operation-12345", Dataset: session.Dataset, Revision: p.Revision, Action: "name", Name: "Petra"}
	body, _ := json.Marshal(a)
	path := fmt.Sprintf("%s/people/%d/actions", base, p.ID)
	w = labelRequest(s, "POST", path, "reader", string(body))
	if w.Code != 403 {
		t.Fatalf("unauthorized write %d", w.Code)
	}
	w = labelRequest(s, "POST", path, "manager", string(body))
	if w.Code != 200 {
		t.Fatalf("write %d %s", w.Code, w.Body.String())
	}
	receipt := w.Body.String()
	w = labelRequest(s, "GET", base+"/actions/"+a.OperationID+"?dataset="+session.Dataset, "manager", "")
	if w.Code != 200 || w.Body.String() != receipt {
		t.Fatalf("receipt %d %s", w.Code, w.Body.String())
	}
	w = labelRequest(s, "POST", path, "manager", string(body))
	if w.Code != 200 || w.Body.String() != receipt {
		t.Fatalf("repeat %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/candidates?after=-1", "/people/1?offset=-1", "/faces/1/thumbnail?size=500"} {
		w = labelRequest(s, "GET", base+path, "manager", "")
		if w.Code != 400 {
			t.Fatalf("validation %s: %d", path, w.Code)
		}
	}
	w = labelRequest(s, "POST", path, "manager", string(body)+" {}")
	if w.Code != 400 {
		t.Fatalf("trailing JSON %d", w.Code)
	}
	audit, err := s.repo.ListAuditLogs(ctx, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range audit {
		if a.Action == "Fotoperson über Benennungs-API bearbeiten" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing labeling audit")
	}
}
