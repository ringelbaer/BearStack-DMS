package facerec

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientProtocol(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Error("missing token")
		}
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(Health{Ready: true, Protocol: 1, Model: Model})
			return
		}
		if r.Header.Get("Content-Type") != "image/jpeg" {
			t.Error("content type")
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "jpeg" {
			t.Error("image body changed")
		}
		v := make([]float32, 128)
		v[0] = 2
		_ = json.NewEncoder(w).Encode(Result{Model: Model, Faces: []Detection{{X: .1, Y: .1, Width: .2, Height: .2, Confidence: .99, Embedding: v}}})
	}))
	defer srv.Close()
	c, err := New(srv.URL, strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := c.Analyze(context.Background(), []byte("jpeg"))
	if err != nil || len(r.Faces) != 1 || r.Faces[0].Embedding[0] != 1 {
		t.Fatalf("result %+v %v", r, err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestClientRejectsRedirectsAndMalformedResponses(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{{"redirect", "", 302}, {"wrong-model", `{"model":"other","faces":[]}`, 200}, {"bad-json", "not json", 200}, {"oversize", strings.Repeat("x", (2<<20)+1), 200}, {"unavailable", "", 503}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer srv.Close()
			c, _ := New(srv.URL, strings.Repeat("t", 32))
			if _, err := c.Analyze(context.Background(), []byte("jpeg")); err == nil {
				t.Fatal("invalid response accepted")
			}
			if calls != 1 {
				t.Fatal("redirect followed")
			}
		})
	}
}
func TestValidateRejectsInvalidVectorsAndBoxes(t *testing.T) {
	for _, change := range []func(*Detection){func(d *Detection) { d.X = -1 }, func(d *Detection) { d.Width = 2 }, func(d *Detection) { d.X = math.NaN() }, func(d *Detection) { d.Embedding[0] = float32(math.Inf(1)) }, func(d *Detection) { d.Embedding = nil }, func(d *Detection) { d.Embedding[0] = 0 }} {
		v := make([]float32, 128)
		v[0] = 1
		d := Detection{Width: .2, Height: .2, Confidence: .99, Embedding: v}
		change(&d)
		if Validate(&d) == nil {
			t.Fatal("invalid detection accepted")
		}
	}
}
func TestClientConfigAndCancellation(t *testing.T) {
	for _, u := range []string{"", "file:///tmp/x", "http://user:pass@localhost", "http://localhost/?token=secret"} {
		if _, err := New(u, "token"); err == nil {
			t.Fatal("invalid endpoint")
		}
	}
	c, _ := New("http://127.0.0.1:1", strings.Repeat("t", 32))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Health(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}
