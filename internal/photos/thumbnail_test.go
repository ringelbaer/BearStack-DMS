package photos

import (
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeThumbnailSize(t *testing.T) {
	tests := []struct {
		name string
		size int
		want int
	}{
		{name: "default", size: 0, want: DefaultThumbnailSize},
		{name: "minimum", size: 1, want: MinThumbnailSize},
		{name: "maximum", size: MaxThumbnailSize + 1, want: MaxThumbnailSize},
		{name: "unchanged", size: 960, want: 960},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeThumbnailSize(tt.size); got != tt.want {
				t.Fatalf("NormalizeThumbnailSize(%d) = %d, want %d", tt.size, got, tt.want)
			}
		})
	}
}

func TestNormalizeThumbnailSizes(t *testing.T) {
	got := NormalizeThumbnailSizes([]int{0, 960, 1, 960, MaxThumbnailSize + 1})
	want := []int{MaxThumbnailSize, 960, MinThumbnailSize}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeThumbnailSizes = %#v, want %#v", got, want)
	}
}

func TestLibraryThumbnailReadyContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lib := &Library{}
	ready, err := lib.ThumbnailReadyContext(ctx, "photo.jpg", 120)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled", err)
	}
	if ready {
		t.Fatal("ready = true for canceled context")
	}
}

func TestThumbnailCachePathUsesStableHash(t *testing.T) {
	lib := &Library{cacheDir: filepath.Join(t.TempDir(), "cache")}
	rel := strings.Repeat("very-long-folder/", 40) + "photo.jpg"
	got := lib.thumbnailCachePath(rel, 420)
	key := thumbnailCacheKey(rel)
	want := filepath.Join(lib.cacheDir, "thumbnails", "v2", key[:2], key[2:4], key+"_420q80.webp")
	if got != want {
		t.Fatalf("thumbnail cache path = %q, want %q", got, want)
	}
	if strings.Contains(got, "very-long-folder") {
		t.Fatalf("thumbnail cache path includes original path: %q", got)
	}
}

func TestCachedThumbnailReadyAcceptsLegacyCachePath(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 200, G: 100, A: 255})
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	path := lib.legacyThumbnailCachePath("photo.jpg", 120)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("webp"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !lib.CachedThumbnailReady("photo.jpg", 120) {
		t.Fatal("legacy cached thumbnail was not accepted")
	}
}
