// Datei liest Fotoeinstellungen aus HTTP-Formularen.
package server

import "net/http"

func photoSettingsFromRequest(r *http.Request) PhotoSettings {
	defaults := defaultPhotoSettings()
	return normalizePhotoSettings(PhotoSettings{
		PageSize:                       parseIntOrDefault(r.FormValue("photo_page_size"), defaults.PageSize),
		FolderPreviewCount:             parseIntOrDefault(r.FormValue("folder_preview_count"), defaults.FolderPreviewCount),
		FolderThumbnailSize:            parseIntOrDefault(r.FormValue("folder_thumbnail_size"), defaults.FolderThumbnailSize),
		ThumbnailSize:                  parseIntOrDefault(r.FormValue("thumbnail_size"), defaults.ThumbnailSize),
		PreviewSize:                    parseIntOrDefault(r.FormValue("preview_size"), defaults.PreviewSize),
		LargePreviewSize:               parseIntOrDefault(r.FormValue("large_preview_size"), defaults.LargePreviewSize),
		SlideshowSeconds:               parseIntOrDefault(r.FormValue("slideshow_seconds"), defaults.SlideshowSeconds),
		FrameSeconds:                   parseIntOrDefault(r.FormValue("frame_seconds"), defaults.FrameSeconds),
		PreloadAdjacent:                r.FormValue("preload_adjacent") == "1",
		MapTrackResolutionMeters:       parseIntOrDefault(r.FormValue("photo_map_track_resolution_meters"), defaults.MapTrackResolutionMeters),
		IndexWorkerEnabled:             r.FormValue("index_worker_enabled") == "1",
		IndexWorkerIntervalMinutes:     parseIntOrDefault(r.FormValue("index_worker_interval_minutes"), defaults.IndexWorkerIntervalMinutes),
		IndexWorkerDelayMillis:         parseIntOrDefault(r.FormValue("index_worker_delay_millis"), defaults.IndexWorkerDelayMillis),
		ThumbnailWorkerEnabled:         r.FormValue("thumbnail_worker_enabled") == "1",
		ThumbnailWorkerIntervalMinutes: parseIntOrDefault(r.FormValue("thumbnail_worker_interval_minutes"), defaults.ThumbnailWorkerIntervalMinutes),
		ThumbnailWorkerBatchSize:       parseIntOrDefault(r.FormValue("thumbnail_worker_batch_size"), defaults.ThumbnailWorkerBatchSize),
		ThumbnailConcurrency:           parseIntOrDefault(r.FormValue("thumbnail_concurrency"), defaults.ThumbnailConcurrency),
	})
}
