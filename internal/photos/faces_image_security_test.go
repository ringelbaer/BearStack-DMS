package photos

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"image/jpeg"
	"os"
	"testing"
)

func TestFaceImageRejectsWebPDimensionMismatch(t *testing.T) {
	// A white 2x2 image encoded with cwebp; no external encoder is needed by the test.
	valid, err := hex.DecodeString("52494646240000005745425056503820180000003001009d012a0200020002003425a400037000fefb940000")
	if err != nil {
		t.Fatal(err)
	}
	chunk := func(kind string, data []byte) []byte {
		b := binary.LittleEndian.AppendUint32([]byte(kind), uint32(len(data)))
		b = append(b, data...)
		if len(data)%2 != 0 {
			b = append(b, 0)
		}
		return b
	}
	mismatched := func(alpha bool) []byte {
		// VP8X declares a 1x1 canvas, while the VP8 frame still contains 2x2 pixels.
		canvas := make([]byte, 10)
		if alpha {
			canvas[0] = 0x10
		}
		body := append([]byte("WEBP"), chunk("VP8X", canvas)...)
		if alpha {
			body = append(body, chunk("ALPH", []byte{0, 255})...)
		}
		body = append(body, valid[12:]...)
		return append(binary.LittleEndian.AppendUint32([]byte("RIFF"), uint32(len(body))), body...)
	}
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"valid", valid, true},
		{"mismatched canvas", mismatched(false), false},
		{"undersized alpha plane", mismatched(true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := faceLibrary(t, "image.webp")
			path, err := l.Resolve("image.webp")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, tc.data, 0640); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("FaceImage panicked on malformed WebP: %v", p)
				}
			}()
			data, err := l.FaceImage(context.Background(), "image.webp")
			if !tc.valid {
				if err == nil {
					t.Fatal("FaceImage accepted inconsistent WebP dimensions")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || cfg.Width != 2 || cfg.Height != 2 {
				t.Fatalf("valid image: config=%+v err=%v", cfg, err)
			}
		})
	}
}
