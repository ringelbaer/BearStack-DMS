package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func refinementFixture(t *testing.T, count int) (*Library, facerec.Result) {
	t.Helper()
	l := faceLibrary(t, "a.jpg")
	writeSizedJPEG(t, filepath.Join(l.Root(), "a.jpg"), 3200, 1600, color.White)
	r := facerec.Result{Model: facerec.Model, Faces: make([]facerec.Detection, count)}
	for i := range r.Faces {
		d := faceDetection(0)
		d.X, d.Y, d.Width, d.Height = .02+float64(i)*.08, .4, .04, .08
		d.Quality = &facerec.Quality{FacePixels: 64, Sharpness: 40, ReferenceEligible: true}
		r.Faces[i] = d
	}
	return l, r
}

func refinedFixture() facerec.Result {
	d := faceDetection(1)
	d.X, d.Y, d.Width, d.Height = .22, .22, .56, .56
	d.Quality = &facerec.Quality{FacePixels: 128, Sharpness: 100, ReferenceEligible: true}
	return facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{d}}
}

func TestFaceRefinementRecoversDetailAndPreservesBoxes(t *testing.T) {
	l, original := refinementFixture(t, 1)
	calls := 0
	result, err := l.RefineFaceResult(context.Background(), "a.jpg", original, func(_ context.Context, b []byte) (facerec.Result, error) {
		calls++
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width < 200 || cfg.Width > 1600 || cfg.Height > 1600 || len(b) > facerec.MaxImageBytes {
			t.Fatalf("invalid original crop: %+v, %v", cfg, err)
		}
		return refinedFixture(), nil
	})
	if err != nil || calls != 1 || result.Faces[0].Embedding[1] != 1 || result.Faces[0].Quality.FacePixels != 128 {
		t.Fatalf("refinement failed: calls=%d result=%+v err=%v", calls, result, err)
	}
	a, b := original.Faces[0], result.Faces[0]
	if a.X != b.X || a.Y != b.Y || a.Width != b.Width || a.Height != b.Height || a.Confidence != b.Confidence {
		t.Fatal("refinement changed the original detection box")
	}
	if a.Embedding[0] != 1 || a.Quality.FacePixels != 64 {
		t.Fatal("input was mutated")
	}
	if l.faceImages.lru.Len() != 0 {
		t.Fatal("original source was added to the gallery raster cache")
	}
}

func TestFaceRefinementBudgetAndLegacyResponse(t *testing.T) {
	l, original := refinementFixture(t, 10)
	for i := range original.Faces {
		original.Faces[i].Quality = nil
	}
	calls := 0
	_, err := l.RefineFaceResult(context.Background(), "a.jpg", original, func(context.Context, []byte) (facerec.Result, error) {
		calls++
		r := refinedFixture()
		r.Faces[0].Quality = nil
		return r, nil
	})
	if err != nil || calls != maxFaceRefinements {
		t.Fatalf("legacy/budget: calls=%d err=%v", calls, err)
	}
}

func TestFaceRefinementRetainsSafeOriginalOnOptionalFailure(t *testing.T) {
	for _, mode := range []string{"service", "ambiguous", "other face", "other model", "bad quality", "missing quality", "weak", "no pixel gain"} {
		t.Run(mode, func(t *testing.T) {
			l, original := refinementFixture(t, 1)
			result, err := l.RefineFaceResult(context.Background(), "a.jpg", original, func(context.Context, []byte) (facerec.Result, error) {
				r := refinedFixture()
				switch mode {
				case "service":
					return facerec.Result{}, errors.New("unavailable")
				case "ambiguous":
					r.Faces = append(r.Faces, r.Faces[0])
				case "other face":
					r.Faces[0].X, r.Faces[0].Width = .01, .1
				case "other model":
					r.Model = "incompatible"
				case "bad quality":
					r.Faces[0].Quality.Sharpness = math.NaN()
				case "missing quality":
					r.Faces[0].Quality = nil
				case "weak":
					r.Faces[0].Quality.ReferenceEligible = false
				case "no pixel gain":
					// The overlap still exceeds .5, but fewer than 96 pixels remain.
					r.Faces[0].X, r.Faces[0].Y = .3, .3
					r.Faces[0].Width, r.Faces[0].Height = .4, .4
				}
				return r, nil
			})
			if err != nil || result.Faces[0].Embedding[0] != 1 || result.Faces[0].Quality.FacePixels != 64 {
				t.Fatalf("unsafe replacement: %+v %v", result, err)
			}
		})
	}
}

func TestFaceRefinementAbortsSourcePrivacyAndCancellationChanges(t *testing.T) {
	for _, mode := range []string{"source", "privacy", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			l, original := refinementFixture(t, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := l.RefineFaceResult(ctx, "a.jpg", original, func(context.Context, []byte) (facerec.Result, error) {
				switch mode {
				case "source":
					if err := os.WriteFile(filepath.Join(l.Root(), "a.jpg"), []byte("replaced"), 0600); err != nil {
						t.Fatal(err)
					}
				case "privacy":
					if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				case "cancel":
					cancel()
				}
				return refinedFixture(), nil
			})
			if err == nil {
				t.Fatal("unsafe source change was ignored")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}

func TestFaceRefinementSkipsUnhelpfulOriginalAndLargeFaces(t *testing.T) {
	for _, mode := range []string{"small source", "large face", "no faces"} {
		t.Run(mode, func(t *testing.T) {
			l, original := refinementFixture(t, 1)
			switch mode {
			case "small source":
				writeSizedJPEG(t, filepath.Join(l.Root(), "a.jpg"), 1600, 800, color.White)
			case "large face":
				original.Faces[0].Quality.FacePixels = 100
			case "no faces":
				original.Faces = nil
			}
			_, err := l.RefineFaceResult(context.Background(), "a.jpg", original, func(context.Context, []byte) (facerec.Result, error) {
				t.Fatal("unhelpful refinement sent to service")
				return facerec.Result{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOrientedFaceSourceMatchesAllEXIFPixelTransforms(t *testing.T) {
	src := image.NewNRGBA(image.Rect(2, 3, 5, 5))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetNRGBA(x+2, y+3, color.NRGBA{R: uint8(1 + y*3 + x), A: 255})
		}
	}
	wants := [][]uint8{{1, 2, 3, 4, 5, 6}, {3, 2, 1, 6, 5, 4}, {6, 5, 4, 3, 2, 1}, {4, 5, 6, 1, 2, 3}, {1, 4, 2, 5, 3, 6}, {4, 1, 5, 2, 6, 3}, {6, 3, 5, 2, 4, 1}, {3, 6, 2, 5, 1, 4}}
	for orientation := 1; orientation <= 8; orientation++ {
		w, h := 3, 2
		if orientation >= 5 {
			w, h = h, w
		}
		s := orientedFaceSource{source: src, orientation: orientation, width: w, height: h}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				got := color.NRGBAModel.Convert(s.At(x, y)).(color.NRGBA).R
				if got != wants[orientation-1][y*w+x] {
					t.Fatalf("orientation=%d x=%d y=%d got=%d", orientation, x, y, got)
				}
			}
		}
	}
}

func TestFaceRefinementRealModel(t *testing.T) {
	models := os.Getenv("BEARSTACK_TEST_FACE_MODELS_DIR")
	if models == "" {
		t.Skip("Set BEARSTACK_TEST_FACE_MODELS_DIR for real-model refinement")
	}
	python := os.Getenv("BEARSTACK_TEST_FACE_PYTHON")
	if python == "" {
		python = "python3"
	}
	serviceDir, err := filepath.Abs("../../services/faces")
	if err != nil {
		t.Fatal(err)
	}
	// Using stdin/stdout keeps this opt-in test independent of service sockets,
	// tokens and a running deployment while exercising the actual Python engine.
	analyze := func(ctx context.Context, b []byte) (facerec.Result, error) {
		code := "import json,sys; from pathlib import Path; sys.path.insert(0,sys.argv[1]); from server import Engine; print(json.dumps(Engine(Path(sys.argv[2])).analyze(sys.stdin.buffer.read())))"
		cmd := exec.CommandContext(ctx, python, "-c", code, serviceDir, models)
		cmd.Stdin = bytes.NewReader(b)
		out, err := cmd.Output()
		if err != nil {
			return facerec.Result{}, err
		}
		var r facerec.Result
		err = json.Unmarshal(out, &r)
		return r, err
	}
	fixture, err := os.Open(filepath.Join(serviceDir, "tests/fixtures/astronaut.png"))
	if err != nil {
		t.Fatal(err)
	}
	portrait, _, err := image.Decode(fixture)
	fixture.Close()
	if err != nil {
		t.Fatal(err)
	}
	var native bytes.Buffer
	if err := jpeg.Encode(&native, portrait, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	baseline, err := analyze(ctx, native.Bytes())
	if err != nil || len(baseline.Faces) != 1 {
		t.Fatalf("native model fixture: %+v %v", baseline, err)
	}
	l := faceLibrary(t, "a.jpg")
	source := image.NewNRGBA(image.Rect(0, 0, 4800, 2400))
	draw.Draw(source, image.Rect(700, 200, 1212, 712), portrait, portrait.Bounds().Min, draw.Src)
	f, err := os.Create(filepath.Join(l.Root(), "a.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	err = jpeg.Encode(f, source, &jpeg.Options{Quality: 90})
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := l.FaceImage(ctx, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := analyze(ctx, preview)
	if err != nil || len(initial.Faces) != 1 || initial.Faces[0].Quality.ReferenceEligible {
		t.Fatalf("expected one detected small face: %+v %v", initial, err)
	}
	refined, err := l.RefineFaceResult(ctx, "a.jpg", initial, analyze)
	if err != nil || !refined.Faces[0].Quality.ReferenceEligible {
		t.Fatalf("original crop did not recover useful pixels: %+v %v", refined, err)
	}
	before := cosine(baseline.Faces[0].Embedding, initial.Faces[0].Embedding)
	after := cosine(baseline.Faces[0].Embedding, refined.Faces[0].Embedding)
	if after < .9 || after < before+.1 {
		t.Fatalf("refinement lost detail: before=%f after=%f", before, after)
	}
	t.Logf("face pixels %.1f -> %.1f; same-fixture cosine %.3f -> %.3f", initial.Faces[0].Quality.FacePixels, refined.Faces[0].Quality.FacePixels, before, after)
}
