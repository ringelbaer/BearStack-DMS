// Datei buendelt serverseitige Foto-Operationen zwischen Handlern und Foto-Library.
package server

import (
	"context"
	"errors"
	"strings"

	"bearstack/internal/photos"
	"bearstack/internal/tagutil"
)

var (
	errNoPhotosFound       = errors.New("keine Fotos gefunden")
	errPhotoModuleMissing  = errors.New("Fotomodul ist nicht konfiguriert")
	errInvalidPhotoTagKind = errors.New("ungültiger Foto-Tag-Typ")
)

type photoApplicationService struct {
	library  *photos.Library
	settings func(context.Context) (PhotoSettings, error)
}

type photoListingRequest struct {
	Options          photos.ListOptions
	IncludeAdminOnly bool
	Frame            bool
	CanEdit          bool
	MapRequested     bool
}

func (s *Server) photoService() photoApplicationService {
	return photoApplicationService{
		library:  s.photos,
		settings: s.photoSettings,
	}
}

func (svc photoApplicationService) Listing(ctx context.Context, request photoListingRequest) (photos.Listing, PhotoSettings, error) {
	if svc.library == nil {
		return photos.Listing{}, PhotoSettings{}, errPhotoModuleMissing
	}
	finishSettings := photos.StartListTraceStep(ctx, "photos.service.settings")
	settings, err := svc.settings(ctx)
	if err != nil {
		finishSettings(photos.ListTraceString("error", err.Error()))
		return photos.Listing{}, PhotoSettings{}, err
	}
	finishSettings(
		photos.ListTraceInt("page_size", settings.PageSize),
		photos.ListTraceInt("folder_preview_count", settings.FolderPreviewCount),
		photos.ListTraceInt("folder_thumbnail_size", settings.FolderThumbnailSize),
	)
	finishOptions := photos.StartListTraceStep(ctx, "photos.service.options")
	opts := request.Options
	cleanPath, err := photos.CleanPath(opts.Path)
	if err != nil {
		finishOptions(photos.ListTraceString("error", err.Error()))
		return photos.Listing{}, PhotoSettings{}, err
	}
	opts.Path = cleanPath
	opts.IncludeAdminOnly = request.IncludeAdminOnly
	opts.PageSize = settings.PageSize
	opts.FolderPreviewSize = settings.FolderPreviewCount
	opts.RouteClusterRadiusMeters = settings.MapTrackResolutionMeters
	mapRequested := request.MapRequested && photoMapAvailable(opts.Path)
	opts.IncludeMapData = mapRequested
	if request.Frame {
		opts.Recursive = true
		opts.Page = 1
		opts.PageSize = 10000
		opts.LeanMetadata = true
		opts.IncludeMapData = false
	}
	if mapRequested {
		opts.GPSOnly = true
		opts.Recursive = true
		opts.Page = 1
		opts.PageSize = 10000
		opts.LeanMetadata = true
	}
	if !request.Frame && !mapRequested && !request.CanEdit {
		opts.LeanMetadata = true
	}
	finishOptions(
		photos.ListTraceString("path", opts.Path),
		photos.ListTraceString("query", opts.Query),
		photos.ListTraceString("media_type", opts.MediaType),
		photos.ListTraceString("sort", opts.Sort),
		photos.ListTraceBool("frame", request.Frame),
		photos.ListTraceBool("map", mapRequested),
		photos.ListTraceBool("recursive", opts.Recursive),
		photos.ListTraceBool("lean_metadata", opts.LeanMetadata),
		photos.ListTraceBool("include_admin_only", opts.IncludeAdminOnly),
	)
	finishList := photos.StartListTraceStep(ctx, "photos.service.library_list", photos.ListTraceString("path", opts.Path))
	listing, err := svc.library.List(ctx, opts)
	if err != nil {
		finishList(photos.ListTraceString("error", err.Error()))
		return photos.Listing{}, PhotoSettings{}, err
	}
	finishList(
		photos.ListTraceInt("folders", len(listing.Folders)),
		photos.ListTraceInt("blogs", len(listing.Blogs)),
		photos.ListTraceInt("media", len(listing.Media)),
		photos.ListTraceInt("total", listing.Total),
	)
	if err := svc.library.AddAutomaticFaces(ctx, listing.Media); err != nil {
		return photos.Listing{}, PhotoSettings{}, err
	}
	return listing, settings, nil
}

func (svc photoApplicationService) SetTags(ctx context.Context, admin bool, kind, path string, tags []string) ([]string, error) {
	if svc.library == nil {
		return nil, errPhotoModuleMissing
	}
	access := photoAccessPolicy{library: svc.library, allowAdminOnly: admin}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "folder":
		if err := access.RequireFolder(path); err != nil {
			return nil, err
		}
		return svc.library.SetFolderTagsContext(ctx, path, tags)
	case "media", "image":
		if err := access.RequireMedia(path); err != nil {
			return nil, err
		}
		return svc.library.SetMediaTagsContext(ctx, path, tags)
	default:
		return nil, errInvalidPhotoTagKind
	}
}

func (svc photoApplicationService) UpdateMediaTags(ctx context.Context, admin bool, paths, tags []string, add bool) (int, error) {
	if svc.library == nil {
		return 0, errPhotoModuleMissing
	}
	tags = normalizeTagValues(tags, "")
	access := photoAccessPolicy{library: svc.library, allowAdminOnly: admin}
	mediaItems := make([]photos.Media, 0, len(paths))
	for _, path := range paths {
		if err := access.RequireMedia(path); err != nil {
			return 0, err
		}
		media, err := svc.library.MediaContext(ctx, path)
		if err != nil {
			return 0, err
		}
		mediaItems = append(mediaItems, media)
	}

	updated := 0
	for _, media := range mediaItems {
		next := tagutil.Merge(media.Tags, tags)
		if !add {
			next = tagutil.Remove(media.Tags, tags)
		}
		if tagutil.EqualNormalized(media.Tags, next) {
			continue
		}
		if _, err := svc.library.SetMediaTagsContext(ctx, media.Path, next); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func (svc photoApplicationService) RandomMediaPath(ctx context.Context, opts photos.ListOptions, admin bool) (string, error) {
	if svc.library == nil {
		return "", errPhotoModuleMissing
	}
	opts.IncludeAdminOnly = admin
	opts.Recursive = true
	opts.Page = 1
	opts.PageSize = 1
	opts.LeanMetadata = true
	opts.IncludeMapData = false
	listing, err := svc.library.List(ctx, opts)
	if err != nil {
		return "", err
	}
	if listing.Total <= 0 || len(listing.Media) == 0 {
		return "", errNoPhotosFound
	}
	index, err := randomIndex(listing.Total)
	if err != nil {
		return "", err
	}
	if index == 0 {
		return listing.Media[0].Path, nil
	}
	opts.Page = index + 1
	listing, err = svc.library.List(ctx, opts)
	if err != nil {
		return "", err
	}
	if len(listing.Media) == 0 {
		return "", errNoPhotosFound
	}
	return listing.Media[0].Path, nil
}
