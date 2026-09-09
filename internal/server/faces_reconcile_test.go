package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func mergeSuggestionServer(t *testing.T) (*Server, photos.FaceMergeSuggestion) {
	t.Helper()
	s := faceTestServer(t)
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join(s.photos.Root(), "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.photos.Root(), "two.jpg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		job, err := s.photos.NextFaceJob(ctx)
		if err != nil {
			t.Fatal(err)
		}
		v := make([]float32, 128)
		v[0] = 1
		if i == 1 {
			v[0] = .52
			v[1] = float32(math.Sqrt(1 - .52*.52))
		}
		d := facerec.Detection{X: .2, Y: .2, Width: .4, Height: .4, Confidence: .99, Embedding: v}
		if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{d}}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			fs, err := s.photos.AutomaticFaces(ctx, job.Path)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.photos.RenamePerson(ctx, fs[0].PersonID, "Ada"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = s.photos.ScheduleFaceReconciliation(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		progress, err := s.photos.ReconcileFacesBatch(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		if !progress.Pending {
			break
		}
	}
	suggestions, err := s.photos.FaceMergeSuggestions(ctx, 60)
	if err != nil || len(suggestions) != 1 {
		t.Fatalf("suggestions %+v %v", suggestions, err)
	}
	return s, suggestions[0]
}

func TestFaceMergeSuggestionsHTTP(t *testing.T) {
	for _, scenario := range []string{"accept", "reject", "stale", "html-stale", "csrf", "reader", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			s, suggestion := mergeSuggestionServer(t)
			for _, user := range []string{"reader", "editor"} {
				w := labelRequest(s, "GET", "/photos/people/merge-suggestions?format=json", user, "")
				want := 200
				if user == "reader" {
					want = 403
				}
				if w.Code != want {
					t.Fatalf("list %s: %d %s", user, w.Code, w.Body.String())
				}
				if want == 200 && (w.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(w.Body.String(), `"source_revision"`) || strings.Contains(w.Body.String(), "embedding")) {
					t.Fatalf("list contract %s", w.Body.String())
				}
			}
			r := httptest.NewRequest("GET", "/photos/people/merge-suggestions", nil)
			r.SetBasicAuth("editor", "secret")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "Getrennt lassen") || !strings.Contains(w.Body.String(), "Zusammenführen") {
				t.Fatalf("HTML %d %s", w.Code, w.Body.String())
			}
			form := url.Values{"source_revision": {fmt.Sprint(suggestion.SourceRevision)}, "target_revision": {fmt.Sprint(suggestion.TargetRevision)}}
			action, user, want := "accept", "editor", 200
			switch scenario {
			case "reject":
				action = "reject"
			case "stale", "html-stale":
				if err := s.photos.RenamePerson(context.Background(), suggestion.TargetID, "Grace"); err != nil {
					t.Fatal(err)
				}
				want = 409
			case "csrf":
				want = 403
			case "reader":
				user = "reader"
				want = 403
			case "invalid":
				form.Del("source_revision")
				want = 400
			}
			r = httptest.NewRequest("POST", fmt.Sprintf("/photos/people/merge-suggestions/%d/%s", suggestion.ID, action), strings.NewReader(form.Encode()))
			r.SetBasicAuth(user, "secret")
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Accept", "application/json")
			if scenario == "html-stale" {
				r.Header.Set("Accept", "text/html")
			}
			if scenario == "csrf" {
				r.Header.Set("Origin", "https://attacker.invalid")
			}
			w = httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("action %d want%d: %s", w.Code, want, w.Body.String())
			}
			if scenario == "html-stale" && (!strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), `href="/photos/people/merge-suggestions"`)) {
				t.Fatalf("HTML conflict lost recovery link: %s", w.Body.String())
			}
			if want == 200 {
				items, err := s.photos.FaceMergeSuggestions(context.Background(), 60)
				if err != nil || len(items) != 0 {
					t.Fatalf("handled suggestion remains: %+v %v", items, err)
				}
			}
		})
	}
}

func TestFaceReconciliationControlsWithoutInferenceService(t *testing.T) {
	s, _ := groupPhotoServerFixture(t)
	for _, action := range []string{"reconcile", "reconcile-pause", "reconcile-resume"} {
		if w := faceRequest(s, "POST", "/settings/photos/faces/"+action, "editor", nil); w.Code != 403 {
			t.Fatalf("editor control %d", w.Code)
		}
		if w := faceRequest(s, "POST", "/settings/photos/faces/"+action, "manager", nil); w.Code != 303 {
			t.Fatalf("control %s: %d %s", action, w.Code, w.Body.String())
		}
		s.faceWorker.reconcileRun.Lock()
		s.faceWorker.reconcileRun.Unlock()
		w := faceRequest(s, "GET", "/settings/photos/faces?format=json&progress=1", "manager", nil)
		var view FaceSettingsView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || view.Settings.ReconcileEnabled != (action != "reconcile-pause") || view.Configured || view.Settings.Enabled || view.ReconciliationError != "" {
			t.Fatalf("view %+v %v", view, err)
		}
	}
}

func TestFaceReconciliationSettingCompatibilityAndWake(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	// An installation with a pre-existing settings record receives the additive default.
	if err := s.repo.SaveSetting(ctx, faceSettingsKey, `{"enabled":false,"batch_size":100,"delay_millis":1000,"interval_minutes":15}`); err != nil {
		t.Fatal(err)
	}
	settings, err := s.faceSettings(ctx)
	if err != nil || !settings.ReconcileEnabled {
		t.Fatalf("migration default %+v %v", settings, err)
	}
	for _, tc := range []struct {
		form    url.Values
		enabled bool
	}{
		{url.Values{"reconcile_enabled": {"0"}}, false},
		{url.Values{"batch_size": {"20"}}, false},
		{url.Values{"reconcile_enabled": {"1"}}, true},
	} {
		select {
		case <-s.faceWorker.wake:
		default:
		}
		w := faceRequest(s, "POST", "/settings/photos/faces", "manager", tc.form)
		if w.Code != 303 {
			t.Fatalf("save %d %s", w.Code, w.Body.String())
		}
		settings, err = s.faceSettings(ctx)
		if err != nil || settings.ReconcileEnabled != tc.enabled {
			t.Fatalf("setting %+v %v", settings, err)
		}
		select {
		case <-s.faceWorker.wake:
		default:
			t.Fatal("settings did not wake worker")
		}
	}
}
