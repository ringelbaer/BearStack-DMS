package server

import (
	"context"

	"bearstack/internal/photos"
)

// Readiness is prepared once per thumbnail size, before building the view.
type photoThumbnailReadiness map[int]map[string]bool

type photoThumbnailReader interface {
	CachedThumbnailsReadyForMediaContext(context.Context, []photos.Media, int) map[string]bool
}

func (svc photoApplicationService) ThumbnailReadiness(ctx context.Context, listing photos.Listing, settings PhotoSettings) photoThumbnailReadiness {
	if svc.library == nil {
		return nil
	}
	return loadPhotoThumbnailReadiness(ctx, svc.library, listing, settings)
}

func loadPhotoThumbnailReadiness(ctx context.Context, reader photoThumbnailReader, listing photos.Listing, settings PhotoSettings) photoThumbnailReadiness {
	settings = normalizePhotoPresentationSettings(settings)
	groups := make(map[int][]photos.Media, 2)
	for _, folder := range listing.Folders {
		for _, item := range folder.Previews {
			// Face portraits have their own cache and are always presented as ready.
			if item.FaceID <= 0 && photos.CanThumbnail(item.Path) {
				groups[settings.FolderThumbnailSize] = append(groups[settings.FolderThumbnailSize], item)
			}
		}
	}
	for _, item := range listing.Media {
		if (item.Type == photos.MediaTypeImage || item.Type == photos.MediaTypeVideo) && photos.CanThumbnail(item.Path) {
			groups[settings.ThumbnailSize] = append(groups[settings.ThumbnailSize], item)
		}
	}
	ready := make(photoThumbnailReadiness, len(groups))
	for size, media := range groups {
		finish := photos.StartListTraceStep(ctx, "photos.service.thumbnail_ready", photos.ListTraceInt("size", size), photos.ListTraceInt("items", len(media)))
		ready[size] = reader.CachedThumbnailsReadyForMediaContext(ctx, media, size)
		finish()
	}
	return ready
}
