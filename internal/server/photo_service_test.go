package server

import (
	"context"
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
