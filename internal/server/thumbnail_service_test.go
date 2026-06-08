package server

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

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
	script := filepath.Join(dir, "pdftoppm")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
prefix=""
while [ "$#" -gt 0 ]; do
	prefix="$1"
	shift
done
printf '%s' 'jpg' > "$prefix.jpg"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
