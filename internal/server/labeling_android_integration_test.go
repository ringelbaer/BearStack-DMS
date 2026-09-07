package server

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

// Opt-in local fixture for Android integration tests. Never opens a LAN listener or touches user data.
func TestAndroidLabelingFixture(t *testing.T) {
	stop := os.Getenv("BEARSTACK_ANDROID_FIXTURE_STOP")
	if stop == "" {
		t.Skip("optional Android HTTPS integration fixture")
	}
	s := faceTestServer(t)
	data, err := os.ReadFile(filepath.Join(s.photos.Root(), "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"} {
		if err = os.WriteFile(filepath.Join(s.photos.Root(), name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		job, e := s.photos.NextFaceJob(ctx)
		if e != nil {
			t.Fatal(e)
		}
		v := make([]float32, 128)
		if i < 5 {
			v[0] = 1
		} else {
			v[1] = 1
		}
		if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: v}}}); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:18787")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(s.Handler())
	server.Listener.Close()
	server.Listener = listener
	server.StartTLS()
	defer server.Close()
	t.Log("HTTPS fixture ready on https://127.0.0.1:18787; account manager, password secret")
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("Android fixture timed out")
		case <-tick.C:
			if _, err = os.Stat(stop); err == nil {
				return
			}
		}
	}
}
