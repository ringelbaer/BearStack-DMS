package server

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bearstack/internal/repository"
)

func TestPhotoSettingsFailedSavePreservesCacheAndRuntime(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings.db")
	repo, err := repository.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	var applied PhotoSettings
	svc := settingsService{store: repo, photo: &photoSettingsState{}, applyPhoto: func(s PhotoSettings) { applied = s }}
	old := defaultPhotoSettings()
	if err := svc.SavePhotoSettings(ctx, old); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_setting BEFORE INSERT ON settings WHEN new.key='photo_thumbnail_size' BEGIN SELECT RAISE(ABORT,'write failure'); END`); err != nil {
		t.Fatal(err)
	}
	next := old
	next.PageSize, next.ThumbnailSize = 200, 640
	if err := svc.SavePhotoSettings(ctx, next); err == nil {
		t.Fatal("expected failure")
	}
	got, err := svc.PhotoSettings(ctx)
	if err != nil || got != old || applied != old {
		t.Fatalf("failed save published state: %+v %+v %v", got, applied, err)
	}
	values, err := repo.GetSettings(ctx, photoSettingKeys...)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := photoSettings(ctx, settingsValues(values))
	if err != nil || persisted != old {
		t.Fatalf("partial database save: %+v %v", persisted, err)
	}
}

func TestPhotoSettingsLoadAppliesPersistedRuntimeSettings(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	want := defaultPhotoSettings()
	want.ThumbnailConcurrency = 4
	if err := repo.SaveSettings(ctx, photoSettingValues(want)); err != nil {
		t.Fatal(err)
	}
	var applied PhotoSettings
	calls := 0
	svc := settingsService{store: repo, photo: &photoSettingsState{}, applyPhoto: func(s PhotoSettings) {
		applied = s
		calls++
	}}
	for range 2 {
		got, err := svc.PhotoSettings(ctx)
		if err != nil || got != want || applied != want || calls != 1 {
			t.Fatalf("persisted settings: cache=%+v runtime=%+v calls=%d %v", got, applied, calls, err)
		}
	}
}

type pausedSettingsRead struct {
	settingsStore
	read, resume chan struct{}
	once         sync.Once
}

func (s *pausedSettingsRead) GetSettings(ctx context.Context, keys ...string) (map[string]string, error) {
	values, err := s.settingsStore.GetSettings(ctx, keys...)
	s.once.Do(func() { close(s.read); <-s.resume })
	return values, err
}

func TestSettingsSlowReadCannotOverwriteConcurrentSave(t *testing.T) {
	for _, kind := range []string{"photo", "render"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			store := &pausedSettingsRead{settingsStore: repo, read: make(chan struct{}), resume: make(chan struct{})}
			var applied PhotoSettings
			svc := settingsService{store: store, photo: &photoSettingsState{}, app: &appSettingsState{}, applyPhoto: func(s PhotoSettings) { applied = s }}
			readDone := make(chan error, 1)
			go func() {
				if kind == "photo" {
					_, err := svc.PhotoSettings(ctx)
					readDone <- err
				} else {
					_, err := svc.RenderSettings(ctx)
					readDone <- err
				}
			}()
			<-store.read
			saveDone := make(chan error, 1)
			next := defaultPhotoSettings()
			next.PageSize = 200
			go func() {
				if kind == "photo" {
					saveDone <- svc.SavePhotoSettings(ctx, next)
				} else {
					saveDone <- svc.SaveSettings(ctx, map[string]string{themeModeSettingKey: themeModeDesign2, appNameSettingKey: "New"})
				}
			}()
			// Let a concurrent save finish before releasing the old snapshot if
			// the service permits it. A serialized writer waits for the reader;
			// either strategy must leave the newest value cached and applied.
			saved := false
			var saveErr error
			select {
			case saveErr = <-saveDone:
				saved = true
			case <-time.After(200 * time.Millisecond):
			}
			close(store.resume)
			if err := <-readDone; err != nil {
				t.Fatal(err)
			}
			if !saved {
				saveErr = <-saveDone
			}
			if saveErr != nil {
				t.Fatal(saveErr)
			}
			if kind == "photo" {
				got, err := svc.PhotoSettings(ctx)
				if err != nil || got != next || applied != next {
					t.Fatalf("stale photo settings: cache=%+v runtime=%+v %v", got, applied, err)
				}
			} else {
				got, err := svc.RenderSettings(ctx)
				if err != nil || got.ThemeMode != themeModeDesign2 {
					t.Fatalf("stale render settings: %+v %v", got, err)
				}
			}
		})
	}
}

type failingSettingsSave struct{ settingsStore }

func (s failingSettingsSave) SaveSettings(context.Context, map[string]string) error {
	return errors.New("failed save")
}

func TestGeneralSettingsFailedSavePreservesCache(t *testing.T) {
	svc := settingsService{store: failingSettingsSave{}, app: &appSettingsState{}}
	svc.CacheAppName("Old")
	svc.CacheRenderSettings(renderSettingsSnapshot{ThemeMode: themeModeDesign2})
	if err := svc.SaveSettings(context.Background(), map[string]string{appNameSettingKey: "New"}); err == nil {
		t.Fatal("expected failure")
	}
	if name, err := svc.AppName(context.Background()); err != nil || name != "Old" {
		t.Fatalf("name: %q %v", name, err)
	}
	if got, err := svc.RenderSettings(context.Background()); err != nil || got.ThemeMode != themeModeDesign2 {
		t.Fatalf("render: %+v %v", got, err)
	}
}
