package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThumbnailCommandTimeoutReleasesSlotAndFlight(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	r := newThumbnailRuntime(1)
	run := func(script string) error {
		_, err := r.flight(context.Background(), "same-image", func() (string, error) {
			release, err := r.acquireSlot(context.Background())
			if err != nil {
				return "", err
			}
			defer release()
			return "", runThumbnailCommandTimeout(context.Background(), 100*time.Millisecond, "test", "/bin/sh", "", t.TempDir(), "-c", script)
		})
		return err
	}
	start := time.Now()
	if err := run("exec /bin/sleep 30"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("decoder did not stop promptly")
	}
	done := make(chan error, 1)
	go func() { done <- run("exit 0") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("slot or flight remained blocked")
	}
}

func TestThumbnailCancellationDoesNotStartFallback(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	bin := t.TempDir()
	for _, name := range []string{"vipsthumbnail", "ffmpeg"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for _, kind := range []string{MediaTypeImage, MediaTypeVideo} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			target := filepath.Join(t.TempDir(), "thumb.webp")
			err := writeMediaThumbnail(ctx, kind, "source.jpg", target, 420)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("partial thumbnail remains: %v", err)
			}
		})
	}
}

func TestVideoFallbackPreservesTimeout(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	bin := t.TempDir()
	script := "#!/bin/sh\nwhile [ $# -gt 1 ]; do\n if [ \"$1\" = -ss ]; then\n  [ \"$2\" = 1 ] && exit 1\n  exec /bin/sleep 30\n fi\n shift\ndone\nexit 2\n"
	if err := os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := writeVideoThumbnail(ctx, "source.mp4", filepath.Join(t.TempDir(), "thumb.webp"), 420); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fallback timeout lost: %v", err)
	}
}
