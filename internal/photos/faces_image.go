package photos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// FaceImage has its own fixed quality/size, independent of gallery preferences.
// DecodeConfig bounds allocations before decoding untrusted image dimensions.
func (l *Library) FaceImage(ctx context.Context, path string) ([]byte, error) {
	img, err := l.faceImage(ctx, path)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	return b.Bytes(), err
}
func (l *Library) faceImage(ctx context.Context, path string) (image.Image, error) {
	if l.faceImageGate != nil {
		select {
		case l.faceImageGate <- struct{}{}:
			defer func() { <-l.faceImageGate }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	private, err := l.MediaAdminOnly(path)
	if err != nil {
		return nil, err
	}
	if private {
		return nil, ErrAdminOnly()
	}
	abs, err := l.Resolve(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > 40_000_000/int64(cfg.Height) {
		return nil, errors.New("Bild überschreitet 40 Megapixel")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	orientation := 1
	if meta, e := readMetadata(abs); e == nil && meta.Orientation >= 1 && meta.Orientation <= 8 {
		orientation = meta.Orientation
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	scale := math.Min(1, 1600/float64(max(w, h)))
	w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	small := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), src, src.Bounds(), draw.Src, nil)
	if orientation == 1 {
		return small, nil
	}
	ow, oh := w, h
	if orientation >= 5 {
		ow, oh = h, w
	}
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			out.SetNRGBA(dx, dy, small.NRGBAAt(x, y))
		}
	}
	return out, nil
}
func (l *Library) FaceThumbnail(ctx context.Context, id int64) ([]byte, error) {
	return l.FaceThumbnailSize(ctx, id, 160)
}

func (l *Library) FaceThumbnailSize(ctx context.Context, id int64, size int) ([]byte, error) {
	if size != 160 && size != 640 {
		return nil, errors.New("ungültige Vorschaugröße")
	}
	f, err := l.Face(ctx, id)
	if err != nil {
		return nil, err
	}
	abs, err := l.Resolve(f.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	legacyKey := faceThumbnailKey(f, size, abs, info.Size(), info.ModTime().UnixNano())
	// Old previews stretched the crop. Replace them on demand without a full
	// cache scan or rebuilding the library at startup.
	key := "aspect-fit-v2:" + legacyKey
	b, err := l.faceThumbnails.get(ctx, id, key, func() ([]byte, error) {
		return l.renderFaceThumbnail(ctx, f, size)
	}, legacyKey)
	if err != nil {
		return nil, err
	}
	// Revalidate even on cache hits, including deletion or replacement during rendering.
	if _, err = l.Face(ctx, id); err != nil {
		return nil, err
	}
	return b, nil
}

func (l *Library) renderFaceThumbnail(ctx context.Context, f RecognizedFace, size int) ([]byte, error) {
	img, err := l.faceImage(ctx, f.Path)
	if err != nil {
		return nil, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	r := image.Rect(int(f.X*float64(w)), int(f.Y*float64(h)), int(math.Ceil((f.X+f.Width)*float64(w))), int(math.Ceil((f.Y+f.Height)*float64(h)))).Intersect(img.Bounds())
	if r.Empty() {
		return nil, errors.New("leere Gesichtsregion")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	// Keep the complete crop and its proportions inside the square tile. The
	// opaque neutral background also handles transparent source images.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.NRGBA{R: 17, G: 24, B: 32, A: 255}), image.Point{}, draw.Src)
	scale := float64(size) / float64(max(r.Dx(), r.Dy()))
	cw, ch := max(1, int(math.Round(float64(r.Dx())*scale))), max(1, int(math.Round(float64(r.Dy())*scale)))
	x, y := (size-cw)/2, (size-ch)/2
	draw.ApproxBiLinear.Scale(dst, image.Rect(x, y, x+cw, y+ch), img, r, draw.Over, nil)
	// Check again after decoding, in case the folder became private meanwhile.
	private, err := l.MediaAdminOnly(f.Path)
	if err != nil {
		return nil, err
	}
	if private {
		return nil, ErrAdminOnly()
	}
	var b bytes.Buffer
	err = jpeg.Encode(&b, dst, &jpeg.Options{Quality: 85})
	return b.Bytes(), err
}
