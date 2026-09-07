package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func TestDocumentThumbnailRejectsOversizedHeaderBeforeDecode(t *testing.T) {
	for _, dimensions := range [][2]uint32{{20_000, 20_000}, {40_000_001, 1}, {1, 40_000_001}} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			var raw bytes.Buffer
			if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}
			// Keep only a valid PNG signature and IHDR. Decoding pixel data would
			// fail with EOF; the dimension limit must reject it first.
			header := raw.Bytes()[:33]
			binary.BigEndian.PutUint32(header[16:20], dimensions[0])
			binary.BigEndian.PutUint32(header[20:24], dimensions[1])
			binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
			dir := t.TempDir()
			source, target := filepath.Join(dir, "source.png"), filepath.Join(dir, "target.jpg")
			if err := os.WriteFile(source, header, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := writeDocumentImageThumbnail(source, target, 120); !errors.Is(err, errDocumentThumbnailDimensions) {
				t.Fatalf("error = %v", err)
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "existing" {
				t.Fatalf("target changed: %q %v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatalf("temporary file leaked: %v %v", entries, err)
			}
		})
	}
}

func TestDocumentThumbnailAcceptsPixelLimitHeader(t *testing.T) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	header := raw.Bytes()[:33]
	binary.BigEndian.PutUint32(header[16:20], 8_000)
	binary.BigEndian.PutUint32(header[20:24], 5_000)
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	dir := t.TempDir()
	source := filepath.Join(dir, "source.png")
	if err := os.WriteFile(source, header, 0600); err != nil {
		t.Fatal(err)
	}
	err := writeDocumentImageThumbnail(source, filepath.Join(dir, "target.jpg"), 120)
	if err == nil || errors.Is(err, errDocumentThumbnailDimensions) {
		t.Fatalf("expected missing pixel data error at permitted limit, got %v", err)
	}
}

func TestWriteDocumentImageThumbnailReplacesTargetAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "target.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 80, A: 255})
		}
	}
	sourceFile, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(sourceFile, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = sourceFile.Close()
		t.Fatal(err)
	}
	if err := sourceFile.Close(); err != nil {
		t.Fatal(err)
	}

	oldContent := []byte("old-thumbnail")
	if err := os.WriteFile(target, oldContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeDocumentImageThumbnail(source, target, 12); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(updated, oldContent) || len(updated) == 0 {
		t.Fatalf("target was not replaced with a thumbnail, size=%d", len(updated))
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".target.jpg.*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestThumbnailServiceCreatesOfficeThumbnailFromPreviewPDF(t *testing.T) {
	installServerFakeSoffice(t)
	installFakePDFToPPM(t)
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	storedPath := writeStoredTestFile(t, store, "2026/05/note.rtf", []byte(`{\rtf1\ansi thumbnail text}`))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "note.rtf",
		StoredPath:   storedPath,
		Title:        "Note",
		MIMEType:     "application/rtf",
		SizeBytes:    26,
		SHA256:       "thumbnail-office",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.GetDocumentFile(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	service := newThumbnailService(repo, store, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	if err := service.Ensure(ctx, doc); err != nil {
		t.Fatal(err)
	}

	refreshed, err := repo.GetDocumentFile(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ThumbnailPath == "" {
		t.Fatal("thumbnail path was not stored")
	}
	thumbnailAbs, err := store.Resolve(refreshed.ThumbnailPath)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(thumbnailAbs); err != nil || info.Size() == 0 {
		t.Fatalf("thumbnail stat = %v size=%d", err, fileSize(info))
	}
	previewAbs, err := store.Resolve(documentOfficePreviewPath(docID))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(previewAbs); err != nil || info.Size() == 0 {
		t.Fatalf("preview stat = %v size=%d", err, fileSize(info))
	}
}

func TestThumbnailServiceCreatesPlainTextThumbnailWithoutSoffice(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	installFakePDFToPPM(t)
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	storedPath := writeStoredTestFile(t, store, "2026/05/note.md", []byte("# Heading\n\nPlain markdown text"))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "note.md",
		StoredPath:   storedPath,
		Title:        "Note",
		MIMEType:     "text/markdown; charset=utf-8",
		SizeBytes:    29,
		SHA256:       "thumbnail-plain-text",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.GetDocumentFile(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	service := newThumbnailService(repo, store, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	if err := service.Ensure(ctx, doc); err != nil {
		t.Fatal(err)
	}

	refreshed, err := repo.GetDocumentFile(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ThumbnailPath == "" {
		t.Fatal("thumbnail path was not stored")
	}
}

func installFakePDFToPPM(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.jpg")
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEARSTACK_TEST_PDF_THUMBNAIL", fixture)
	script := filepath.Join(dir, "pdftoppm")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
prefix=""
while [ "$#" -gt 0 ]; do
	prefix="$1"
	shift
done
/bin/cp "$BEARSTACK_TEST_PDF_THUMBNAIL" "$prefix.jpg"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPDFThumbnailPublicationPreservesTargetOnRendererFailure(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		cancel       bool
	}{
		{"partial failure", "printf broken > \"$prefix.jpg\"\nexit 1\n", false},
		{"invalid successful output", "printf broken > \"$prefix.jpg\"\n", false},
		{"cancelled renderer", "printf broken > \"$prefix.jpg\"\nexec /bin/sleep 30\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			script := "#!/bin/sh\nprefix=\"\"\nfor arg in \"$@\"; do prefix=\"$arg\"; done\n" + tc.script
			if err := os.WriteFile(filepath.Join(bin, "pdftoppm"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			dir := t.TempDir()
			target := filepath.Join(dir, "existing.jpg")
			old := []byte("previous complete thumbnail")
			if err := os.WriteFile(target, old, 0640); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			if err := writePDFThumbnail(ctx, "source.pdf", target); err == nil {
				t.Fatal("expected render failure")
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, old) {
				t.Fatalf("target changed: %q, %v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files remain: %#v, %v", entries, err)
			}
		})
	}
}

func TestPDFThumbnailPublicationReplacesTargetWithValidJPEG(t *testing.T) {
	installFakePDFToPPM(t)
	target := filepath.Join(t.TempDir(), "thumbnail.jpg")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writePDFThumbnail(context.Background(), "source.pdf", target); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := jpeg.Decode(file); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("permissions: %v, %v", info, err)
	}
}

func TestThumbnailWarmupContinuesPastEntireFailedBatch(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "documents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := 0; i < 11; i++ {
		content := []byte("broken image")
		if i == 10 {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
				t.Fatal(err)
			}
			content = buf.Bytes()
		}
		name := fmt.Sprintf("image-%02d.jpg", i)
		path := writeStoredTestFile(t, store, name, content)
		id, err := repo.CreateDocument(ctx, document.Document{OriginalName: name, StoredPath: path, MIMEType: "image/jpeg", SHA256: name, Title: name})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	service := newThumbnailService(repo, store, nil, make(chan struct{}, 1))
	if err := service.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		doc, err := repo.GetDocumentFile(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if (doc.ThumbnailPath != "") != (i == 10) {
			t.Fatalf("document %d thumbnail: %q", id, doc.ThumbnailPath)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := service.EnsureAll(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
