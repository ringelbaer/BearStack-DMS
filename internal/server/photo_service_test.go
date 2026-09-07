package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/repository"
)

func TestPhotoListingUsesConfiguredPageSize(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for i := 1; i <= 3; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("photo-%d.jpg", i)), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		repo:   repo,
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	settings := defaultPhotoSettings()
	settings.PageSize = 2
	if err := server.savePhotoSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	listing, loadedSettings, err := server.photoService().Listing(ctx, photoListingRequest{
		Options: photos.ListOptions{Sort: "ascending_date", Page: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if loadedSettings.PageSize != 2 || listing.PageSize != 2 || len(listing.Media) != 2 || !listing.HasNext {
		t.Fatalf("settings=%#v listing pageSize=%d media=%d hasNext=%v", loadedSettings, listing.PageSize, len(listing.Media), listing.HasNext)
	}
}

func TestPhotoBulkTagsValidateWholeSelectionAndNormalizePaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "private"), 0750); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"public.jpg", "private/secret.jpg", "private/.adminonly"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("photo"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	library, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	svc := photoApplicationService{library: library}
	if n, err := svc.UpdateMediaTags(ctx, false, []string{"public.jpg", "./public.jpg"}, []string{" First "}, true); err != nil || n != 1 {
		t.Fatalf("normalized selection count=%d, error=%v", n, err)
	}
	if n, err := svc.UpdateMediaTags(ctx, false, []string{"public.jpg", "./private/secret.jpg"}, []string{"forbidden"}, true); !errors.Is(err, photos.ErrAdminOnly()) || n != 0 {
		t.Fatalf("private selection count=%d, error=%v", n, err)
	}
	if n, err := svc.UpdateMediaTags(ctx, false, []string{"public.jpg", "missing.jpg"}, []string{"missing"}, true); err == nil || n != 0 {
		t.Fatalf("missing selection count=%d, error=%v", n, err)
	}
	m, err := library.MediaContext(ctx, "public.jpg")
	if err != nil || len(m.Tags) != 1 || m.Tags[0] != "first" {
		t.Fatalf("partial changes: tags=%v error=%v", m.Tags, err)
	}
	if n, err := svc.UpdateMediaTags(ctx, true, []string{"private/secret.jpg"}, []string{"allowed"}, true); err != nil || n != 1 {
		t.Fatalf("admin selection count=%d, error=%v", n, err)
	}
}
