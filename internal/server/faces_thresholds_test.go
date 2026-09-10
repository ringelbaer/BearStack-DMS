package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestFaceThresholdSettings(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	fields := []string{"assignment_similarity", "assignment_margin", "reconcile_similarity", "reconcile_margin", "suggestion_similarity", "suggestion_margin"}
	values := []string{"0.4", "0", "0.7", "0.2", "0.6", "0.1"}
	form := url.Values{}
	for i, key := range fields {
		form.Set(key, values[i])
	}
	w := faceRequest(s, "POST", "/settings/photos/faces", "manager", form)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	expected := photos.FaceThresholds{AssignmentSimilarity: .4, AssignmentMargin: 0, ReconcileSimilarity: .7, ReconcileMargin: .2, SuggestionSimilarity: .6, SuggestionMargin: .1}
	assertSaved := func() {
		t.Helper()
		got, err := s.photos.FaceThresholds(ctx)
		if err != nil || got != expected {
			t.Fatalf("saved: %+v %v", got, err)
		}
	}
	assertSaved()
	for _, key := range fields {
		invalid := []string{"", "NaN", "Inf", "-0.01", "abc"}
		if strings.HasSuffix(key, "similarity") {
			invalid = append(invalid, "0.39", "0.71")
		} else {
			invalid = append(invalid, "0.21")
		}
		for _, value := range invalid {
			w = faceRequest(s, "POST", "/settings/photos/faces", "manager", url.Values{key: {value}, "reference_limit": {"50"}})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("%s=%s: %d", key, value, w.Code)
			}
			assertSaved()
		}
	}
	if limit, err := s.photos.FaceReferenceLimit(ctx); err != nil || limit != 30 {
		t.Fatalf("invalid form mutated references: %d %v", limit, err)
	}
	w = faceRequest(s, "POST", "/settings/photos/faces", "manager", url.Values{"suggestion_margin": {"0", "0.1"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate: %d", w.Code)
	}
	w = faceRequest(s, "POST", "/settings/photos/faces", "reader", form)
	if w.Code != http.StatusForbidden {
		t.Fatalf("permissions: %d", w.Code)
	}
	w = faceRequest(s, "POST", "/settings/photos/faces", "manager", url.Values{"batch_size": {"20"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("legacy: %d", w.Code)
	}
	assertSaved()
	w = faceRequest(s, "GET", "/settings/photos/faces?format=json", "manager", nil)
	var view FaceSettingsView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Settings.Thresholds == nil || *view.Settings.Thresholds != expected {
		t.Fatalf("JSON: %+v %v", view, err)
	}
	w = faceRequest(s, "GET", "/settings/photos/faces", "manager", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<details class="face-settings-expert">`) {
		t.Fatalf("expert UI: %d", w.Code)
	}
	for _, key := range fields {
		if !strings.Contains(w.Body.String(), `name="`+key+`"`) {
			t.Fatalf("missing input %s", key)
		}
	}
}
