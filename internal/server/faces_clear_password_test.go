package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func TestFaceClearRequiresPasswordBeforeChangingState(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, facerec.Dimensions)
	vector[0] = 1
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: vector}}}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, job.Path)
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	if err := s.photos.RenamePerson(ctx, faces[0].PersonID, "Petra"); err != nil {
		t.Fatal(err)
	}
	settings, err := s.faceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	if err := s.saveFaceSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		label, user, confirm, password string
		status                         int
	}{
		{"missing password", "manager", "delete", "", 403},
		{"wrong password", "manager", "delete", "wrong-confirmation-password", 403},
		{"missing checkbox", "manager", "", "secret", 400},
		{"read-only user", "reader", "delete", "secret", 403},
	} {
		t.Run(test.label, func(t *testing.T) {
			response := faceRequest(s, "POST", "/settings/photos/faces/clear", test.user, url.Values{"confirm": {test.confirm}, "password": {test.password}})
			if response.Code != test.status {
				t.Fatalf("status: %d %s", response.Code, response.Body.String())
			}
			if test.password != "" && strings.Contains(response.Body.String(), test.password) {
				t.Fatal("password reflected in response")
			}
			current, err := s.faceSettings(ctx)
			if err != nil || !current.Enabled {
				t.Fatalf("failed confirmation disabled processing: %+v %v", current, err)
			}
			face, err := s.photos.Face(ctx, faces[0].ID)
			if err != nil || face.Name != "Petra" {
				t.Fatalf("failed confirmation changed data: %+v %v", face, err)
			}
		})
	}
	request := httptest.NewRequest("GET", "/settings/photos/faces", nil)
	request.SetBasicAuth("manager", "secret")
	rendered := httptest.NewRecorder()
	s.Handler().ServeHTTP(rendered, request)
	if rendered.Code != 200 || !strings.Contains(rendered.Body.String(), `data-password-prompt=`) || !strings.Contains(rendered.Body.String(), `data-password-prompt-confirm="Gesichtsdaten löschen"`) {
		t.Fatalf("password dialog missing: %d", rendered.Code)
	}
	response := faceRequest(s, "POST", "/settings/photos/faces/clear", "manager", url.Values{"confirm": {"delete"}, "password": {"secret"}})
	if response.Code != 303 {
		t.Fatalf("confirmed deletion: %d %s", response.Code, response.Body.String())
	}
	current, err := s.faceSettings(ctx)
	if err != nil || current.Enabled {
		t.Fatalf("processing not disabled: %+v %v", current, err)
	}
	if _, err := s.photos.Face(ctx, faces[0].ID); err == nil {
		t.Fatal("confirmed deletion retained face")
	}
}
