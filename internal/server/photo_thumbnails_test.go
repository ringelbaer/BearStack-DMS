package server

import (
	"context"
	"reflect"
	"testing"

	"bearstack/internal/photos"
)

type photoThumbnailReaderFunc func(context.Context, []photos.Media, int) map[string]bool

func (read photoThumbnailReaderFunc) CachedThumbnailsReadyForMediaContext(ctx context.Context, media []photos.Media, size int) map[string]bool {
	return read(ctx, media, size)
}

func TestPhotoThumbnailReadinessBatchesSizesAndPreservesPresentation(t *testing.T) {
	image := photos.Media{Path: "album/photo.jpg", Type: photos.MediaTypeImage}
	video := photos.Media{Path: "album/movie.mp4", Type: photos.MediaTypeVideo}
	listing := photos.Listing{
		Folders: []photos.Folder{{Path: "album", Previews: []photos.Media{
			image, {Path: "portrait.jpg", FaceID: 42}, {Path: "audio.mp3", Type: photos.MediaTypeAudio},
		}}},
		Media: []photos.Media{image, video, {Path: "missing.jpg", Type: photos.MediaTypeImage}, {Path: "audio.mp3", Type: photos.MediaTypeAudio}},
	}
	for _, sameSize := range []bool{false, true} {
		settings := PhotoSettings{FolderThumbnailSize: 180, ThumbnailSize: 420}
		if sameSize {
			settings.FolderThumbnailSize = settings.ThumbnailSize
		}
		calls := map[int][]string{}
		reader := photoThumbnailReaderFunc(func(ctx context.Context, media []photos.Media, size int) map[string]bool {
			if _, exists := calls[size]; exists {
				t.Fatalf("repeated status batch for size %d", size)
			}
			for _, item := range media {
				calls[size] = append(calls[size], item.Path)
			}
			return map[string]bool{image.Path: size == 180, video.Path: true}
		})
		ready := loadPhotoThumbnailReadiness(context.Background(), reader, listing, settings)
		want := map[int][]string{180: {image.Path}, 420: {image.Path, video.Path, "missing.jpg"}}
		if sameSize {
			want = map[int][]string{420: {image.Path, image.Path, video.Path, "missing.jpg"}}
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("status batches: got %v, want %v", calls, want)
		}
		view := newPhotoListingView(context.Background(), listing, settings, ready)
		if view.Folders[0].Previews[0].ThumbReady == sameSize || !view.Folders[0].Previews[1].ThumbReady || view.Folders[0].Previews[1].ThumbURL != "/photos/faces/42/thumbnail" {
			t.Fatalf("folder and face readiness: %+v", view.Folders[0].Previews)
		}
		if view.Media[0].ThumbReady || !view.Media[1].ThumbReady || view.Media[2].ThumbReady || view.Media[3].ThumbReady || view.Media[3].ThumbURL != "" {
			t.Fatalf("media readiness: %+v", view.Media)
		}
	}
}

func TestPhotoThumbnailReadinessDefaultsAndEmptyListing(t *testing.T) {
	defaults := defaultPhotoSettings()
	listing := photos.Listing{Media: []photos.Media{{Path: "photo.jpg", Type: photos.MediaTypeImage}}}
	count := 0
	reader := photoThumbnailReaderFunc(func(_ context.Context, _ []photos.Media, size int) map[string]bool {
		count++
		if size != defaults.ThumbnailSize {
			t.Fatalf("size = %d, want %d", size, defaults.ThumbnailSize)
		}
		return map[string]bool{"photo.jpg": true}
	})
	ready := loadPhotoThumbnailReadiness(context.Background(), reader, listing, PhotoSettings{})
	if !newPhotoListingView(context.Background(), listing, PhotoSettings{}, ready).Media[0].ThumbReady {
		t.Fatal("default sizes differ between loading and presentation")
	}
	loadPhotoThumbnailReadiness(context.Background(), reader, photos.Listing{}, PhotoSettings{})
	if count != 1 {
		t.Fatalf("empty listing queried thumbnails: %d calls", count)
	}
	ready = (photoApplicationService{}).ThumbnailReadiness(context.Background(), listing, PhotoSettings{})
	if newPhotoListingView(context.Background(), listing, PhotoSettings{}, ready).Media[0].ThumbReady {
		t.Fatal("missing library marked thumbnails as ready")
	}
}
