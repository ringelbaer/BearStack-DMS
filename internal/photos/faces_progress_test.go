package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFaceProgressNeedsNoFilesystemAndExpires(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/one.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`UPDATE photo_face_jobs SET status='failed',error='private/path.jpg'`); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(l.Root(), filepath.Join(t.TempDir(), "offline")); err != nil {
		t.Fatal(err)
	}
	want := FaceStatus{Failed: 1, Faces: 1, People: 1}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			s, err := l.FaceProgress(ctx)
			if err != nil || s.Failed != want.Failed || s.Faces != 1 || s.People != 1 || len(s.Errors) != 0 {
				t.Errorf("progress %+v %v", s, err)
			}
		})
	}
	wg.Wait()
	if _, err := l.FaceStatus(ctx); err == nil {
		t.Fatal("full view must still validate directories")
	}
	if _, err := l.index.db.Exec(`UPDATE photo_face_jobs SET status='done',error=''`); err != nil {
		t.Fatal(err)
	}
	if s, err := l.FaceProgress(ctx); err != nil || s.Failed != 1 {
		t.Fatal("snapshot not shared", s, err)
	}
	l.faceProgress.mu.Lock()
	l.faceProgress.expires = time.Time{}
	l.faceProgress.mu.Unlock()
	if s, err := l.FaceProgress(ctx); err != nil || s.Done != 1 || s.Failed != 0 {
		t.Fatal("expired counts not refreshed", s, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.FaceProgress(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
