package photos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestGPXInventoryIsLazyAndCursorDoesNotExposeNewlyPrivatePaths(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"a-private", "b-public"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 40; i++ {
			// Deliberately invalid XML: an inventory scan must never parse it.
			if err := os.WriteFile(filepath.Join(root, directory, fmt.Sprintf("%02d.gpx", i)), []byte("metadata only"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	l := newTestLibrary(t, root)
	defer l.Close()
	ctx := context.Background()
	stats, err := l.RebuildIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 0 || stats.Blogs != 0 || len(l.gpxCache) != 0 {
		t.Fatalf("GPX polluted media or parsed eagerly: %+v", stats)
	}
	if err := os.WriteFile(filepath.Join(root, "a-private", AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	seen := map[string]bool{}
	for batch := 0; batch < 4; batch++ {
		result, err := l.GPXFiles(ctx, "", cursor, false)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Ready || len(result.Files) > 32 {
			t.Fatalf("inventory: %+v", result)
		}
		if batch == 0 && (len(result.Files) != 0 || !result.HasNext) {
			t.Fatal("first bounded batch should skip newly private tracks")
		}
		if _, err := strconv.ParseInt(result.Cursor, 10, 64); err != nil {
			t.Fatalf("cursor reveals path: %q", result.Cursor)
		}
		for _, file := range result.Files {
			if parentPath(file.Path) != "b-public" || seen[file.Path] {
				t.Fatalf("hidden or repeated path: %s", file.Path)
			}
			seen[file.Path] = true
		}
		if !result.HasNext {
			break
		}
		if cursor == result.Cursor {
			t.Fatal("cursor did not advance over hidden tracks")
		}
		cursor = result.Cursor
	}
	if len(seen) != 40 {
		t.Fatalf("public tracks: %d", len(seen))
	}
	if _, err := l.GPXFiles(ctx, "a-private", "", false); !errors.Is(err, errAdminOnly) {
		t.Fatalf("private directory: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.GPXFiles(cancelled, "", "", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := l.GPXFiles(ctx, "", "../secret", false); !errors.Is(err, ErrMapCursor) {
		t.Fatalf("cursor validation: %v", err)
	}
}

func TestGPXInventoryRefreshesContentChangesAndBackfillsWithoutInvalidatingMaps(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "trip"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "trip", "track.gpx")
	if err := os.WriteFile(file, []byte("<gpx/>"), 0600); err != nil {
		t.Fatal(err)
	}
	l := newTestLibrary(t, root)
	defer l.Close()
	ctx := context.Background()
	rebuild := func() {
		t.Helper()
		if _, err := l.RebuildIndex(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rebuild()
	first, err := l.GPXFiles(ctx, "trip", "", false)
	if err != nil || len(first.Files) != 1 || !first.Ready {
		t.Fatalf("first inventory: %+v %v", first, err)
	}
	directoryInfo, err := os.Stat(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("<gpx><trk/></gpx>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Dir(file), directoryInfo.ModTime(), directoryInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	rebuild()
	updated, err := l.GPXFiles(ctx, "trip", "", false)
	if err != nil || len(updated.Files) != 1 || updated.Files[0].Bytes != int64(len("<gpx><trk/></gpx>")) || updated.Cursor != first.Cursor {
		t.Fatalf("content update with stable cursor: %+v %v", updated, err)
	}
	// Model the automatic upgrade: existing scan coverage survives; only the
	// optional GPX inventory awaits the next normal scan.
	if _, err := l.index.db.Exec(`DELETE FROM gpx_index; UPDATE photo_folder_scan SET gpx_scanned=0`); err != nil {
		t.Fatal(err)
	}
	pending, err := l.GPXFiles(ctx, "trip", "", false)
	if err != nil || pending.Ready {
		t.Fatalf("pending backfill: %+v %v", pending, err)
	}
	if _, err := l.Map(ctx, ListOptions{}, MapBounds{-90, -180, 90, 180}); err != nil {
		t.Fatalf("existing map coverage invalidated: %v", err)
	}
	rebuild()
	ready, err := l.GPXFiles(ctx, "trip", "", false)
	if err != nil || !ready.Ready || len(ready.Files) != 1 {
		t.Fatalf("backfill: %+v %v", ready, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	rebuild()
	empty, err := l.GPXFiles(ctx, "trip", "", false)
	if err != nil || len(empty.Files) != 0 {
		t.Fatalf("deleted track retained: %+v %v", empty, err)
	}
}

func TestGPXInventoryCursorsRoundTripWithoutOffsetPaging(t *testing.T) {
	root := t.TempDir()
	l := newTestLibrary(t, root)
	defer l.Close()
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	files := make([]GPXFile, 100)
	for i := range files {
		files[i] = GPXFile{Path: fmt.Sprintf("%03d.gpx", i), Name: fmt.Sprintf("%03d.gpx", i)}
	}
	if err := l.index.replaceGPXDirectory(ctx, "", files); err != nil {
		t.Fatal(err)
	}
	first, err := l.GPXFiles(ctx, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.GPXFiles(ctx, "", first.Cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := l.GPXFilesBefore(ctx, "", second.PreviousCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != 32 || len(second.Files) != 32 || len(previous.Files) != 32 || previous.HasPrevious || !previous.HasNext {
		t.Fatalf("cursor flags: %+v", previous)
	}
	for i := range first.Files {
		if first.Files[i].Path != previous.Files[i].Path {
			t.Fatal("backward cursor lost file order")
		}
	}
}
