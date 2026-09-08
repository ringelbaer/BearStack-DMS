package photos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFaceThumbnailPreservesCropAspectRatio(t *testing.T) {
	l := faceLibrary(t, "source.png")
	src := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.NRGBA{R: 240, G: 96, B: 48, A: 255}), image.Point{}, draw.Src)
	f, err := os.Create(filepath.Join(l.Root(), "source.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{160, 640} {
		for _, tc := range []struct {
			name       string
			x, y, w, h float64
			want       image.Rectangle
		}{
			{"portrait", .25, 0, .25, 1, image.Rect(size/4, 0, size*3/4, size)},
			{"landscape", 0, .25, 1, .5, image.Rect(0, size*3/8, size, size*5/8)},
			// Normalized width != height, but the source crop is square in pixels.
			{"square", .25, .25, .25, .5, image.Rect(0, 0, size, size)},
			{"clipped edge", .875, 0, .25, .5, image.Rect(size/4, 0, size*3/4, size)},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, size), func(t *testing.T) {
				data, err := l.renderFaceThumbnail(context.Background(), RecognizedFace{Path: "source.png", X: tc.x, Y: tc.y, Width: tc.w, Height: tc.h}, size)
				if err != nil {
					t.Fatal(err)
				}
				img, err := jpeg.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds() != image.Rect(0, 0, size, size) {
					t.Fatalf("tile is not square: %v", img.Bounds())
				}
				for y := 0; y < size; y++ {
					for x := 0; x < size; x++ {
						// Leave two pixels at the JPEG boundary for chroma subsampling.
						point := image.Pt(x, y)
						want := color.NRGBA{R: 17, G: 24, B: 32, A: 255}
						if point.In(tc.want.Inset(2)) {
							want = color.NRGBA{R: 240, G: 96, B: 48, A: 255}
						} else if point.In(tc.want.Inset(-2)) {
							continue
						}
						got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
						if absInt(int(got.R)-int(want.R)) > 8 || absInt(int(got.G)-int(want.G)) > 8 || absInt(int(got.B)-int(want.B)) > 8 {
							t.Fatalf("pixel %v = %v, want %v; crop must fill %v", point, got, want, tc.want)
						}
					}
				}
			})
		}
	}
}

func TestFaceThumbnailReplacesStretchedCacheOnDemand(t *testing.T) {
	for _, ignored := range []bool{false, true} {
		t.Run(fmt.Sprintf("ignored=%v", ignored), func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg")
			stopFaceCacheCleanup(t, l)
			finishFace(t, l, 0)
			faces, err := l.AutomaticFaces(ctx, "a.jpg")
			if err != nil || len(faces) != 1 {
				t.Fatalf("faces: %v %v", faces, err)
			}
			face := faces[0]
			if ignored {
				if err := l.EditFaces(ctx, []int64{face.ID}, 0, true, ""); err != nil {
					t.Fatal(err)
				}
			}
			oldSmall, _, _ := legacyFaceCacheFile(t, l, face, 160)
			oldLarge, largeData, stamp := legacyFaceCacheFile(t, l, face, 640)
			l.faceThumbnails.mu.Lock()
			err = l.faceThumbnails.adoptLegacy(ctx, strings.TrimSuffix(filepath.Base(oldSmall), ".jpg"), face.ID)
			l.faceThumbnails.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			for _, size := range []int{160, 640} {
				data, err := l.FaceThumbnailSize(ctx, face.ID, size)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
				if err != nil || cfg.Width != size || cfg.Height != size {
					t.Fatalf("old stretched cache served: %+v %v", cfg, err)
				}
				old := oldSmall
				if size == 640 {
					old = oldLarge
				} else {
					assertLegacyFaceCacheFile(t, oldLarge, largeData, stamp)
				}
				if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("superseded preview retained: %v", err)
				}
				again, err := l.FaceThumbnailSize(ctx, face.ID, size)
				if err != nil || !bytes.Equal(data, again) {
					t.Fatal("replacement was not cached", err)
				}
			}
			entries := faceCacheEntries(t, l, face.ID)
			if len(entries) != 2 {
				t.Fatalf("obsolete cache entries remain: %v", entries)
			}
			for _, expiry := range entries {
				if ignored && (expiry <= time.Now().Unix() || expiry > time.Now().Add(ignoredFaceThumbnailTTL).Unix()) || !ignored && expiry != 0 {
					t.Fatalf("incorrect replacement lifetime: %d", expiry)
				}
			}
		})
	}
}

func TestFaceThumbnailReplacementRetriesFailedCleanup(t *testing.T) {
	c := faceThumbnailCache{dir: t.TempDir()}
	defer c.close()
	old := c.path(hashFaceThumbnailKey("old"))
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(old, "blocker")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	get := func() {
		t.Helper()
		data, err := c.get(context.Background(), 0, "new", func() ([]byte, error) {
			calls++
			return []byte("corrected preview"), nil
		}, "old")
		if err != nil || string(data) != "corrected preview" {
			t.Fatalf("cleanup failure blocked preview: %q %v", data, err)
		}
	}
	get()
	if _, err := os.Stat(c.path(hashFaceThumbnailKey("new"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cached before cleanup succeeded", err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	get()
	get()
	if calls != 2 {
		t.Fatalf("expected cleanup retry then cache hit, rendered %d times", calls)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("obsolete preview survived retry", err)
	}
}
