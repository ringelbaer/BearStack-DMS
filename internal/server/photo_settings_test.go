package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type photoSettingReader map[string]string

func (values photoSettingReader) GetSetting(_ context.Context, key string) (string, bool, error) {
	value, ok := values[key]
	return value, ok, nil
}

func TestPhotoSettingsDatabaseAndFormUseSameBounds(t *testing.T) {
	// The database keys and form names are separate public inputs.
	keys := map[string]string{
		photoPageSizeSettingKey:                "photo_page_size",
		photoFolderPreviewCountSettingKey:      "folder_preview_count",
		photoFolderThumbnailSizeSettingKey:     "folder_thumbnail_size",
		photoThumbnailSizeSettingKey:           "thumbnail_size",
		photoPreviewSizeSettingKey:             "preview_size",
		photoLargePreviewSizeSettingKey:        "large_preview_size",
		photoSlideshowSecondsSettingKey:        "slideshow_seconds",
		photoFrameSecondsSettingKey:            "frame_seconds",
		photoMapTrackResolutionSettingKey:      "photo_map_track_resolution_meters",
		photoIndexWorkerIntervalSettingKey:     "index_worker_interval_minutes",
		photoIndexWorkerDelaySettingKey:        "index_worker_delay_millis",
		photoThumbnailWorkerIntervalSettingKey: "thumbnail_worker_interval_minutes",
		photoThumbnailWorkerBatchSettingKey:    "thumbnail_worker_batch_size",
		photoThumbnailConcurrencySettingKey:    "thumbnail_concurrency",
	}
	for _, value := range []string{"", "invalid", "999999999999999999999999", "-1", "0", " 200 ", "99999"} {
		t.Run(value, func(t *testing.T) {
			stored := photoSettingReader{}
			form := url.Values{"preload_adjacent": {"1"}}
			for key, name := range keys {
				stored[key] = value
				form.Set(name, value)
			}
			fromDB, err := photoSettings(context.Background(), stored)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/settings/photos", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if fromForm := photoSettingsFromRequest(r); fromDB != fromForm {
				t.Fatalf("database = %+v; form = %+v", fromDB, fromForm)
			}
		})
	}
}

func TestPhotoSettingsPreserveSourceDefaultsAndCheckboxSemantics(t *testing.T) {
	fromDB, err := photoSettings(context.Background(), photoSettingReader{})
	if err != nil || fromDB != defaultPhotoSettings() {
		t.Fatalf("missing database settings = %+v, err = %v", fromDB, err)
	}
	for _, value := range []string{"", "true", "on", "invalid", "1"} {
		form := url.Values{"preload_adjacent": {value}, "index_worker_enabled": {value}, "thumbnail_worker_enabled": {value}}
		r := httptest.NewRequest(http.MethodPost, "/settings/photos", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		want := defaultPhotoSettings()
		want.PreloadAdjacent = value == "1"
		want.IndexWorkerEnabled = value == "1"
		want.ThumbnailWorkerEnabled = value == "1"
		if got := photoSettingsFromRequest(r); got != want {
			t.Fatalf("checkbox %q: got %+v, want %+v", value, got, want)
		}
	}
	stored := photoSettingReader{photoPreloadAdjacentSettingKey: "invalid", photoIndexWorkerEnabledSettingKey: "true", photoThumbnailWorkerEnabledSettingKey: "on"}
	fromDB, err = photoSettings(context.Background(), stored)
	if err != nil || !fromDB.PreloadAdjacent || !fromDB.IndexWorkerEnabled || !fromDB.ThumbnailWorkerEnabled {
		t.Fatalf("stored booleans = %+v, err = %v", fromDB, err)
	}
}

type failingPhotoSettingReader struct{ err error }

func (r failingPhotoSettingReader) GetSetting(context.Context, string) (string, bool, error) {
	return "", false, r.err
}

func TestPhotoSettingsPropagateReadError(t *testing.T) {
	want := errors.New("settings read failed")
	if _, err := photoSettings(context.Background(), failingPhotoSettingReader{want}); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
