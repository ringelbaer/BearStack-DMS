package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFaceThumbnailIncludesContext(t *testing.T) {
	l := faceLibrary(t, "source.png")
	src := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x / 2), G: uint8(y / 2), B: 100, A: 255})
		}
	}
	var original bytes.Buffer
	if err := png.Encode(&original, src); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(l.Root(), "source.png")
	if err := os.WriteFile(path, original.Bytes(), 0444); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		x, y, w, h float64
		want       image.Rectangle
	}{
		{"center", .375, .375, .25, .25, image.Rect(100, 100, 300, 300)},
		{"left", 0, .375, .25, .25, image.Rect(0, 100, 150, 300)},
		{"right", .75, .375, .25, .25, image.Rect(250, 100, 400, 300)},
		{"top", .375, 0, .25, .25, image.Rect(100, 0, 300, 150)},
		{"bottom", .375, .75, .25, .25, image.Rect(100, 250, 300, 400)},
		{"corner", 0, 0, .25, .25, image.Rect(0, 0, 150, 150)},
		{"whole image", 0, 0, 1, 1, src.Bounds()},
		{"portrait", .4375, .25, .125, .5, image.Rect(150, 0, 250, 400)},
		{"landscape", .25, .4375, .5, .125, image.Rect(0, 150, 400, 250)},
	} {
		for _, size := range []int{160, 640} {
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
					t.Fatalf("bounds: %v", img.Bounds())
				}
				scale := float64(size) / float64(max(tc.want.Dx(), tc.want.Dy()))
				cw, ch := int(math.Round(float64(tc.want.Dx())*scale)), int(math.Round(float64(tc.want.Dy())*scale))
				// Sample the center and all four outer regions. The source gradients encode
				// coordinates, proving that the full requested context reaches the output.
				for _, p := range []image.Point{{1, 1}, {1, 9}, {9, 1}, {9, 9}, {5, 5}} {
					x, y := (size-cw)/2+cw*p.X/10, (size-ch)/2+ch*p.Y/10
					got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					wantX, wantY := tc.want.Min.X+tc.want.Dx()*p.X/10, tc.want.Min.Y+tc.want.Dy()*p.Y/10
					if absInt(int(got.R)-wantX/2) > 4 || absInt(int(got.G)-wantY/2) > 4 || absInt(int(got.B)-100) > 4 {
						t.Fatalf("sample %v = %v, want source near (%d,%d)", p, got, wantX, wantY)
					}
				}
			})
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, original.Bytes()) {
		t.Fatal("original photo changed", err)
	}
}

func TestFaceThumbnailContextPreservesExistingCache(t *testing.T) {
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
			abs := filepath.Join(l.Root(), face.Path)
			info, err := os.Stat(abs)
			if err != nil {
				t.Fatal(err)
			}
			stamp := time.Unix(1700000000, 0)
			type preview struct {
				size int
				path string
				data []byte
			}
			var previews []preview
			for _, size := range []int{160, 640} {
				// Freeze the pre-change cache key, independent of the production helper.
				raw := fmt.Sprintf("aspect-fit-v2:%d:%d:%s:%d:%d:%g:%g:%g:%g", face.ID, size, abs, info.Size(), info.ModTime().UnixNano(), face.X, face.Y, face.Width, face.Height)
				key := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
				path := filepath.Join(l.CacheDir(), "faces", "v1", key[:2], key[2:4], key+".jpg")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				var data bytes.Buffer
				if err := jpeg.Encode(&data, image.NewNRGBA(image.Rect(0, 0, size, size)), nil); err != nil {
					t.Fatal(err)
				}
				if !l.faceThumbnails.register(ctx, key, face.ID) {
					t.Fatal("register preview")
				}
				if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				previews = append(previews, preview{size, path, data.Bytes()})
			}
			entries := faceCacheEntries(t, l, face.ID)
			check := func(l *Library) {
				t.Helper()
				for _, p := range previews {
					got, err := l.FaceThumbnailSize(ctx, face.ID, p.size)
					if err != nil || !bytes.Equal(got, p.data) {
						t.Fatalf("existing %d preview regenerated: %v", p.size, err)
					}
					assertLegacyFaceCacheFile(t, p.path, p.data, stamp)
				}
				after := faceCacheEntries(t, l, face.ID)
				if len(after) != len(entries) {
					t.Fatalf("cache entries changed: %v", after)
				}
				for key, expiry := range entries {
					if got, ok := after[key]; !ok || got != expiry {
						t.Fatalf("cache lifetime changed: %v", after)
					}
				}
			}
			check(l)
			root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
			if err := l.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(root, cache, db, 60)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			check(reopened)
		})
	}
}
