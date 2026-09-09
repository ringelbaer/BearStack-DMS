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
	"sort"

	"bearstack/internal/facerec"
	"golang.org/x/image/draw"
)

const maxFaceRefinements = 8
const refinementFacePixels = 80
const maxRefinementBytes = 16 << 20

type faceRefinement struct {
	index  int
	region Face // Crop bounds in upright original-image coordinates, normalized.
	jpeg   []byte
	width  int
	height int
}

// RefineFaceResult uses original-resolution crops for small detected faces. It
// retains detection boxes so manual overrides remain stable. Extra inference is
// bounded and optional: service errors or ambiguous crops retain the first pass;
// cancellation, source replacement and privacy changes abort the whole result.
func (l *Library) RefineFaceResult(ctx context.Context, path string, result facerec.Result, analyze func(context.Context, []byte) (facerec.Result, error)) (facerec.Result, error) {
	if err := ctx.Err(); err != nil {
		return facerec.Result{}, err
	}
	if len(result.Faces) == 0 || analyze == nil {
		return result, nil
	}
	if len(result.Faces) > facerec.MaxFaces {
		return facerec.Result{}, errors.New("zu viele Gesichter")
	}
	// A failed refinement never mutates the caller's detections or vectors.
	out := result
	out.Faces = append([]facerec.Detection(nil), result.Faces...)
	for i := range out.Faces {
		out.Faces[i].Embedding = append([]float32(nil), out.Faces[i].Embedding...)
		if err := facerec.Validate(&out.Faces[i]); err != nil {
			return facerec.Result{}, err
		}
	}
	crops, key, err := l.prepareFaceRefinements(ctx, path, out.Faces)
	if err != nil {
		return facerec.Result{}, err
	}
	for _, crop := range crops {
		if err := l.checkFaceRefinementSource(ctx, path, key); err != nil {
			return facerec.Result{}, err
		}
		refined, inferenceErr := analyze(ctx, crop.jpeg)
		if err := l.checkFaceRefinementSource(ctx, path, key); err != nil {
			return facerec.Result{}, err
		}
		if inferenceErr != nil {
			// Avoid repeating optional requests to an unavailable or busy service.
			break
		}
		if refined.Model != result.Model || len(refined.Faces) > facerec.MaxFaces {
			continue
		}
		match := -1
		ambiguous := false
		for i := range refined.Faces {
			d := &refined.Faces[i]
			if err := facerec.Validate(d); err != nil {
				ambiguous = true
				break
			}
			box := Face{X: crop.region.X + d.X*crop.region.Width, Y: crop.region.Y + d.Y*crop.region.Height, Width: d.Width * crop.region.Width, Height: d.Height * crop.region.Height}
			target := result.Faces[crop.index]
			if overlap(box, Face{X: target.X, Y: target.Y, Width: target.Width, Height: target.Height}) < .5 {
				continue
			}
			if match != -1 {
				ambiguous = true
				break
			}
			// A detection spanning two original faces cannot safely replace either.
			for j, original := range result.Faces {
				if j != crop.index && overlap(box, Face{X: original.X, Y: original.Y, Width: original.Width, Height: original.Height}) >= .5 {
					ambiguous = true
				}
			}
			match = i
		}
		if match < 0 || ambiguous {
			continue
		}
		d := refined.Faces[match]
		old := out.Faces[crop.index]
		if old.Quality != nil && d.Quality == nil {
			// A mixed-version service pool must not erase known weak quality.
			continue
		}
		if d.Quality != nil && !d.Quality.ReferenceEligible {
			continue
		}
		// Redetection must actually recover more face pixels, not another tiny
		// detection within a larger crop. Legacy services omit quality metadata.
		pixels := math.Min(d.Width*float64(crop.width), d.Height*float64(crop.height))
		if old.Quality != nil && pixels < old.Quality.FacePixels*1.5 {
			continue
		}
		out.Faces[crop.index].Embedding = d.Embedding
		out.Faces[crop.index].Quality = d.Quality
	}
	return out, nil
}

func (l *Library) checkFaceRefinementSource(ctx context.Context, path string, key faceImageKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	private, err := l.MediaAdminOnly(path)
	if err != nil {
		return err
	}
	if private {
		return ErrAdminOnly()
	}
	return l.checkFaceImageSource(key)
}

// The original is decoded once for all crops and is never retained by the
// gallery cache. The existing decode gate bounds simultaneous large allocations.
func (l *Library) prepareFaceRefinements(ctx context.Context, path string, faces []facerec.Detection) ([]faceRefinement, faceImageKey, error) {
	var key faceImageKey
	abs, err := l.Resolve(path)
	if err != nil {
		return nil, key, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, key, err
	}
	key = imageSourceKey(abs, info)
	if err := l.checkFaceRefinementSource(ctx, path, key); err != nil {
		return nil, key, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, key, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, key, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > 40_000_000/int64(cfg.Height) {
		return nil, key, errors.New("Bild überschreitet 40 Megapixel")
	}
	orientation := 1
	if meta, e := readMetadata(abs); e == nil && meta.Orientation >= 1 && meta.Orientation <= 8 {
		orientation = meta.Orientation
	}
	w, h := cfg.Width, cfg.Height
	if orientation >= 5 {
		w, h = h, w
	}
	scale := math.Min(1, 1600/float64(max(w, h)))
	if scale > 1/1.5 {
		return nil, key, l.checkFaceRefinementSource(ctx, path, key)
	}
	indices := make([]int, 0, min(len(faces), maxFaceRefinements))
	for i, d := range faces {
		originalPixels := math.Min(d.Width*float64(w), d.Height*float64(h))
		previewPixels := originalPixels * scale
		if d.Quality != nil {
			previewPixels = d.Quality.FacePixels
		}
		if previewPixels < refinementFacePixels && originalPixels >= 48 && originalPixels >= previewPixels*1.5 {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return nil, key, l.checkFaceRefinementSource(ctx, path, key)
	}
	// Prefer faces with the most recoverable detail; ties retain detector order.
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := faces[indices[i]], faces[indices[j]]
		return math.Min(a.Width*float64(w), a.Height*float64(h)) > math.Min(b.Width*float64(w), b.Height*float64(h))
	})
	indices = indices[:min(len(indices), maxFaceRefinements)]
	if l.faceImageGate != nil {
		select {
		case l.faceImageGate <- struct{}{}:
			defer func() { <-l.faceImageGate }()
		case <-ctx.Done():
			return nil, key, ctx.Err()
		}
	}
	if err := l.checkFaceRefinementSource(ctx, path, key); err != nil {
		return nil, key, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, key, err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, key, err
	}
	if src.Bounds().Dx() != cfg.Width || src.Bounds().Dy() != cfg.Height {
		return nil, key, errFaceSourceChanged
	}
	// View rotation/mirroring lazily while copying each small crop. Allocating a
	// second fully oriented 40 MP raster would double peak source memory.
	upright := orientedFaceSource{source: src, orientation: orientation, width: w, height: h}
	crops := make([]faceRefinement, 0, len(indices))
	totalBytes := 0
	for _, i := range indices {
		if err := l.checkFaceRefinementSource(ctx, path, key); err != nil {
			return nil, key, err
		}
		d := faces[i]
		cx, cy := (d.X+d.Width/2)*float64(w), (d.Y+d.Height/2)*float64(h)
		half := .9 * math.Max(d.Width*float64(w), d.Height*float64(h))
		r := image.Rect(int(math.Floor(cx-half)), int(math.Floor(cy-half)), int(math.Ceil(cx+half)), int(math.Ceil(cy+half))).Intersect(upright.Bounds())
		if r.Empty() {
			continue
		}
		s := math.Min(1, 1600/float64(max(r.Dx(), r.Dy())))
		cw, ch := max(1, int(float64(r.Dx())*s)), max(1, int(float64(r.Dy())*s))
		crop := image.NewNRGBA(image.Rect(0, 0, cw, ch))
		draw.ApproxBiLinear.Scale(crop, crop.Bounds(), upright, r, draw.Src, nil)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, crop, &jpeg.Options{Quality: 90}); err != nil {
			return nil, key, err
		}
		if encoded.Len() > facerec.MaxImageBytes || totalBytes+encoded.Len() > maxRefinementBytes {
			continue
		}
		totalBytes += encoded.Len()
		crops = append(crops, faceRefinement{index: i, region: Face{X: float64(r.Min.X) / float64(w), Y: float64(r.Min.Y) / float64(h), Width: float64(r.Dx()) / float64(w), Height: float64(r.Dy()) / float64(h)}, jpeg: encoded.Bytes(), width: cw, height: ch})
	}
	return crops, key, l.checkFaceRefinementSource(ctx, path, key)
}

type orientedFaceSource struct {
	source        image.Image
	orientation   int
	width, height int
}

func (s orientedFaceSource) ColorModel() color.Model { return s.source.ColorModel() }
func (s orientedFaceSource) Bounds() image.Rectangle { return image.Rect(0, 0, s.width, s.height) }
func (s orientedFaceSource) At(x, y int) color.Color {
	if !(image.Point{X: x, Y: y}).In(s.Bounds()) {
		return color.NRGBA{}
	}
	w, h := s.source.Bounds().Dx(), s.source.Bounds().Dy()
	switch s.orientation {
	case 2:
		x = w - 1 - x
	case 3:
		x, y = w-1-x, h-1-y
	case 4:
		y = h - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, h-1-x
	case 7:
		x, y = w-1-y, h-1-x
	case 8:
		x, y = w-1-y, x
	}
	return s.source.At(x+s.source.Bounds().Min.X, y+s.source.Bounds().Min.Y)
}
