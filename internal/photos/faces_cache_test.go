package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFaceThumbnailCacheSharesRenderAndPersists(t *testing.T) {
	c := faceThumbnailCache{dir: filepath.Join(t.TempDir(), "faces")}
	defer c.close()
	var calls atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			b, err := c.get(context.Background(), "same", func() ([]byte, error) { calls.Add(1); return []byte("jpeg"), nil })
			if err != nil || string(b) != "jpeg" {
				t.Errorf("get: %q %v", b, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("rendered %d times", calls.Load())
	}
	files, err := filepath.Glob(filepath.Join(c.dir, "*", "*", "*.jpg"))
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files: %v %v", files, err)
	}
	info, err := os.Stat(files[0])
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v %v", info, err)
	}
	// Removing a cached file should simply trigger regeneration.
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	_, err = c.get(context.Background(), "same", func() ([]byte, error) { calls.Add(1); return []byte("jpeg"), nil })
	if err != nil || calls.Load() != 2 {
		t.Fatalf("missing file: %d %v", calls.Load(), err)
	}
	dir := c.dir
	c.close()
	reopened := faceThumbnailCache{dir: dir}
	defer reopened.close()
	if _, err := reopened.get(context.Background(), "same", func() ([]byte, error) { t.Fatal("regenerated after restart"); return nil, nil }); err != nil {
		t.Fatal(err)
	}

}

func TestFaceThumbnailCacheFailures(t *testing.T) {
	c := faceThumbnailCache{dir: filepath.Join(t.TempDir(), "faces")}
	defer c.close()
	ctx := context.Background()
	expected := errors.New("decode failure")
	if _, err := c.get(ctx, "bad", func() ([]byte, error) { return nil, expected }); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	if _, err := c.get(ctx, "bad", func() ([]byte, error) { return []byte("retry"), nil }); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.get(cancelled, "b", func() ([]byte, error) { t.Fatal("render after cancellation"); return nil, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFaceThumbnailCacheStillChecksVisibilityAndDeletion(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "album/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	id := faces[0].ID
	if _, err := l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := l.FaceThumbnailSize(ctx, id, 640); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(l.faceThumbnails.dir, "*", "*", "*.jpg"))
	if err != nil || len(files) != 2 {
		t.Fatal("preview sizes share cache key")
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "album/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.FaceThumbnail(ctx, id); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("cached private face served: %v", err)
	}
	if err := os.Remove(filepath.Join(l.Root(), "album/a.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.FaceThumbnail(ctx, id); err == nil {
		t.Fatal("cached deleted face served")
	}
}

func TestFaceThumbnailCacheCancelledWaiterDoesNotCancelRender(t *testing.T) {
	c := faceThumbnailCache{dir: filepath.Join(t.TempDir(), "faces")}
	defer c.close()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		_, err := c.get(context.Background(), "face", func() ([]byte, error) {
			close(started)
			<-release
			return []byte("jpeg"), nil
		})
		if err != nil {
			t.Errorf("render: %v", err)
		}
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.get(ctx, "face", func() ([]byte, error) { t.Error("duplicate render"); return nil, nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v", err)
	}
	close(release)
	<-finished
	if _, err := c.get(context.Background(), "face", func() ([]byte, error) { t.Error("completed render not cached"); return nil, nil }); err != nil {
		t.Fatal(err)
	}
}
