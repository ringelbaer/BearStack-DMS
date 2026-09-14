package photos

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestXMPSidecarSizeBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg.xmp")
	for _, size := range []int{0, maxXMPSidecarBytes, maxXMPSidecarBytes + 1} {
		if err := os.WriteFile(path, bytes.Repeat([]byte{' '}, size), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := readXMPSidecar(path)
		if size <= maxXMPSidecarBytes {
			if err != nil || len(data) != size {
				t.Fatalf("size %d: got %d bytes, %v", size, len(data), err)
			}
		} else if err == nil || len(data) != 0 {
			t.Fatalf("oversized sidecar accepted: %d bytes, %v", len(data), err)
		}
	}
}

func TestXMPSidecarRejectsLinksDirectoriesAndOversizedMetadata(t *testing.T) {
	root := t.TempDir()
	photo := filepath.Join(root, "photo.jpg")
	outside := filepath.Join(t.TempDir(), "secret.xmp")
	if err := os.WriteFile(outside, []byte(`<rdf:Description xmp:Rating="5"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"symlink", "broken-symlink", "directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			path := photo + ".xmp"
			defer os.Remove(path)
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(outside, path)
			case "broken-symlink":
				err = os.Symlink(outside+"-missing", path)
			case "directory":
				err = os.Mkdir(path, 0700)
			default:
				data := append([]byte(`<rdf:Description xmp:Rating="5"/>`), bytes.Repeat([]byte{' '}, maxXMPSidecarBytes)...)
				err = os.WriteFile(path, data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readXMPSidecar(path); err == nil {
				t.Fatal("unsafe sidecar accepted")
			}
			meta, err := readMetadata(photo)
			if err != nil || meta.Rating != nil {
				t.Fatalf("unsafe optional metadata imported: %+v, %v", meta, err)
			}
			if fingerprint := xmpSidecarFingerprint(photo); fingerprint != "" {
				t.Fatalf("unsafe sidecar fingerprint: %s", fingerprint)
			}
		})
	}
	// Changing an unsafe sidecar to a supported regular file invalidates caches.
	if err := os.WriteFile(photo+".xmp", []byte(`<rdf:Description xmp:Rating="4"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := readMetadata(photo)
	if err != nil || meta.Rating == nil || *meta.Rating != 4 || xmpSidecarFingerprint(photo) == "" {
		t.Fatalf("regular sidecar: %+v %v", meta, err)
	}
}
