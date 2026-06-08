package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	benchmarkGalleryCount       = 5_000
	benchmarkPhotoCount         = 300_000
	benchmarkChangedFolderCount = 100
	benchmarkTaggedFilesPerDir  = 10
	benchmarkBlogFilesPerDir    = 10
	benchmarkTagsPerTaggedFile  = 5
)

func BenchmarkMillionPhotoIndex(b *testing.B) {
	runPhotoIndexBenchmark(b, benchmarkGalleryCount, benchmarkPhotoCount)
}

func BenchmarkPhotoIndex300k(b *testing.B) {
	runPhotoIndexBenchmark(b, 29, 300_000)
}

func runPhotoIndexBenchmark(b *testing.B, galleries, photos int) {
	lib := newBenchmarkIndexedLibrary(b, galleries, photos)
	defer lib.Close()
	ctx := context.Background()
	photosPerGallery := photos / galleries
	if photosPerGallery < 1 {
		photosPerGallery = 1
	}
	targetGallery := galleries / 2
	targetDir := benchmarkGalleryName(targetGallery)
	targetIndex := targetGallery*photosPerGallery + photosPerGallery/2
	targetName := fmt.Sprintf("IMG_%07d.jpg", targetIndex)
	targetQuery := "directory:" + targetDir + " " + targetName

	b.Run("root-folders", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(ctx, ListOptions{PageSize: 120})
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.Folders) != galleries {
				b.Fatalf("folders = %d", len(listing.Folders))
			}
		}
	})

	b.Run("gallery-first-page", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(ctx, ListOptions{Path: targetDir, PageSize: 120})
			if err != nil {
				b.Fatal(err)
			}
			expectedPageSize := photosPerGallery
			if expectedPageSize > 120 {
				expectedPageSize = 120
			}
			if len(listing.Media) != expectedPageSize || listing.Total != photosPerGallery {
				b.Fatalf("media = %d total = %d", len(listing.Media), listing.Total)
			}
		}
	})

	b.Run("search-directory-and-name", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(ctx, ListOptions{Query: targetQuery, PageSize: 120})
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.Media) != 1 || listing.Total != 1 {
				b.Fatalf("media = %d total = %d", len(listing.Media), listing.Total)
			}
		}
	})

	b.Run("gps-filter-first-page", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(ctx, ListOptions{Query: "gps:true", PageSize: 120})
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.Media) != 120 || listing.Total != photos/10 {
				b.Fatalf("media = %d total = %d", len(listing.Media), listing.Total)
			}
		}
	})

	b.Run("search-media-tag-first-page", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(ctx, ListOptions{Query: "tag:benchmark", PageSize: 120})
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.Media) != 120 || listing.Total != photos/1000 {
				b.Fatalf("media = %d total = %d", len(listing.Media), listing.Total)
			}
		}
	})
}

func BenchmarkPhotoPostFilterSearch(b *testing.B) {
	ctx := context.Background()
	for _, size := range []struct {
		galleries int
		photos    int
	}{
		{galleries: 200, photos: 20_000},
		{galleries: 500, photos: 50_000},
	} {
		b.Run(fmt.Sprintf("%d-photos", size.photos), func(b *testing.B) {
			lib := newBenchmarkIndexedLibrary(b, size.galleries, size.photos)
			defer lib.Close()
			cases := []struct {
				name  string
				query string
			}{
				{name: "fts-exact", query: "IMG_0004242"},
				{name: "sql-tag", query: "tag:benchmark"},
				{name: "postfilter-short-term", query: "IM"},
				{name: "postfilter-negated-tag", query: "-tag:benchmark"},
				{name: "postfilter-two-of", query: "2-of:(camera:BearCam,lens:PrimeLens)"},
				{name: "postfilter-or-text", query: "IMG_0004242 or IMG_0004243"},
			}
			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						listing, err := lib.List(ctx, ListOptions{Query: tc.query, PageSize: 120})
						if err != nil {
							b.Fatal(err)
						}
						if listing.Total == 0 {
							b.Fatalf("query %q returned no results", tc.query)
						}
					}
				})
			}
		})
	}
}

func BenchmarkPhotoListIndexedGPXTracksCache(b *testing.B) {
	root := filepath.Join(b.TempDir(), "photos")
	trip := filepath.Join(root, "trip")
	if err := os.MkdirAll(trip, 0o750); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		path := filepath.Join(trip, fmt.Sprintf("track-%03d.gpx", i))
		if err := os.WriteFile(path, []byte(benchmarkGPXXML(300)), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(trip, "photo.jpg"), nil, 0o600); err != nil {
		b.Fatal(err)
	}
	lib, err := New(root, filepath.Join(b.TempDir(), "thumbs"), filepath.Join(b.TempDir(), "photos.db"), 120)
	if err != nil {
		b.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		b.Fatal(err)
	}
	opts := ListOptions{
		Path:           "trip",
		Recursive:      true,
		GPSOnly:        true,
		PageSize:       120,
		IncludeMapData: true,
	}
	clearGPXCache := func() {
		lib.gpxMu.Lock()
		clear(lib.gpxCache)
		lib.gpxMu.Unlock()
	}

	b.Run("warm-cache", func(b *testing.B) {
		if _, err := lib.List(context.Background(), opts); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			listing, err := lib.List(context.Background(), opts)
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.GPXTracks) != 60 {
				b.Fatalf("gpx tracks = %d", len(listing.GPXTracks))
			}
		}
	})

	b.Run("forced-reparse", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			clearGPXCache()
			listing, err := lib.List(context.Background(), opts)
			if err != nil {
				b.Fatal(err)
			}
			if len(listing.GPXTracks) != 60 {
				b.Fatalf("gpx tracks = %d", len(listing.GPXTracks))
			}
		}
	})
}

func BenchmarkPhotoListLeanMetadata(b *testing.B) {
	lib := newBenchmarkIndexedLibrary(b, 500, 50_000)
	defer lib.Close()
	ctx := context.Background()

	cases := []struct {
		name string
		opts ListOptions
	}{
		{
			name: "full-metadata",
			opts: ListOptions{
				Recursive: true,
				GPSOnly:   true,
				PageSize:  5_000,
			},
		},
		{
			name: "lean-metadata",
			opts: ListOptions{
				Recursive:    true,
				GPSOnly:      true,
				PageSize:     5_000,
				LeanMetadata: true,
			},
		},
		{
			name: "full-metadata-search",
			opts: ListOptions{
				Recursive: true,
				Query:     "bearcam",
				PageSize:  5_000,
			},
		},
		{
			name: "lean-metadata-search",
			opts: ListOptions{
				Recursive:    true,
				Query:        "bearcam",
				PageSize:     5_000,
				LeanMetadata: true,
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				listing, err := lib.List(ctx, tc.opts)
				if err != nil {
					b.Fatal(err)
				}
				if len(listing.Media) == 0 || listing.Total == 0 {
					b.Fatalf("listing = %#v", listing)
				}
			}
		})
	}
}

func BenchmarkCachedThumbnailsReadyFromIndexMetadata(b *testing.B) {
	root := b.TempDir()
	cacheDir := filepath.Join(b.TempDir(), "cache")
	lib, err := New(root, cacheDir, filepath.Join(b.TempDir(), "photos.db"), 120)
	if err != nil {
		b.Fatal(err)
	}
	defer lib.Close()

	const count = 2000
	const size = 420
	mod := time.Now().Add(-time.Hour).UTC()
	items := make([]Media, 0, count)
	for i := 0; i < count; i++ {
		media := Media{
			Name:      fmt.Sprintf("img-%04d.jpg", i),
			Path:      fmt.Sprintf("album/img-%04d.jpg", i),
			Directory: "album",
			Type:      MediaTypeImage,
			SizeBytes: 1024 + int64(i),
			ModTime:   mod,
		}
		if err := lib.markThumbnailGenerated(context.Background(), media, size); err != nil {
			b.Fatal(err)
		}
		thumb := lib.thumbnailCachePath(media.Path, size)
		if err := os.MkdirAll(filepath.Dir(thumb), 0o750); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(thumb, []byte("webp"), 0o600); err != nil {
			b.Fatal(err)
		}
		items = append(items, media)
	}

	b.Run("generated-index", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ready := lib.CachedThumbnailsReadyForMediaContext(context.Background(), items, size)
			if len(ready) != len(items) {
				b.Fatalf("ready = %d, want %d", len(ready), len(items))
			}
		}
	})

	if _, err := lib.index.db.Exec(`UPDATE photo_thumbnail_index SET status = ? WHERE size = ?`, thumbnailStatusQueued, size); err != nil {
		b.Fatal(err)
	}
	b.Run("queued-fallback-stat", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ready := lib.CachedThumbnailsReadyForMediaContext(context.Background(), items, size)
			if len(ready) != len(items) {
				b.Fatalf("ready = %d, want %d", len(ready), len(items))
			}
		}
	})
}

func benchmarkGPXXML(points int) string {
	if points < 1 {
		points = 1
	}
	var b strings.Builder
	b.Grow(64 + points*48)
	b.WriteString(`<?xml version="1.0"?><gpx><trk><trkseg>`)
	lat := 52.52
	lon := 13.405
	for i := 0; i < points; i++ {
		fmt.Fprintf(&b, `<trkpt lat="%.6f" lon="%.6f"></trkpt>`, lat, lon)
		lat += 0.0002
		lon += 0.0002
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	return b.String()
}

func BenchmarkMediaAdminOnlyBatch(b *testing.B) {
	root := b.TempDir()
	for _, dir := range []string{"public", "secret", "secret/nested"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			b.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "secret", AdminOnlyMarkerName), nil, 0o600); err != nil {
		b.Fatal(err)
	}
	lib, err := New(root, filepath.Join(b.TempDir(), "thumbs"), filepath.Join(b.TempDir(), "photos.db"), 120)
	if err != nil {
		b.Fatal(err)
	}
	defer lib.Close()
	paths := make([]string, 0, 200)
	for i := 0; i < 100; i++ {
		paths = append(paths, fmt.Sprintf("public/photo-%03d.jpg", i))
		paths = append(paths, fmt.Sprintf("secret/nested/private-%03d.jpg", i))
	}

	b.Run("single-loop", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, path := range paths {
				if _, err := lib.MediaAdminOnly(path); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := lib.MediaAdminOnlyBatch(paths); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMillionPhotoRebuildIndex(b *testing.B) {
	lib := newBenchmarkFilesystemLibrary(b, benchmarkGalleryCount, benchmarkPhotoCount)
	defer lib.Close()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats, err := lib.RebuildIndex(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if stats.Media != benchmarkPhotoCount || stats.Folders != benchmarkGalleryCount {
			b.Fatalf("stats = %#v", stats)
		}
	}
}

func BenchmarkMillionPhotoRebuildIndexWarm(b *testing.B) {
	lib := newBenchmarkFilesystemLibrary(b, benchmarkGalleryCount, benchmarkPhotoCount)
	defer lib.Close()
	ctx := context.Background()
	if _, err := lib.RebuildIndex(ctx); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats, err := lib.RebuildIndex(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if stats.Media != benchmarkPhotoCount || stats.Folders != benchmarkGalleryCount {
			b.Fatalf("stats = %#v", stats)
		}
	}
}

func BenchmarkMillionPhotoRebuildIndexScenarios(b *testing.B) {
	b.Run("empty-index", func(b *testing.B) {
		benchmarkRebuildScenario(b, false, nil, IndexStats{
			Media:   benchmarkPhotoCount,
			Folders: benchmarkGalleryCount,
		})
	})
	b.Run("warm-index", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, nil, IndexStats{
			Media:   benchmarkPhotoCount,
			Folders: benchmarkGalleryCount,
		})
	})
	b.Run("one-changed-folder", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, func(b *testing.B, lib *Library) IndexStats {
			addBenchmarkPhotos(b, lib, 0, 1)
			return IndexStats{Media: benchmarkPhotoCount + 1, Folders: benchmarkGalleryCount}
		}, IndexStats{})
	})
	b.Run("hundred-changed-folders", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, func(b *testing.B, lib *Library) IndexStats {
			addBenchmarkPhotos(b, lib, benchmarkChangedFolderCount, 1)
			return IndexStats{Media: benchmarkPhotoCount + benchmarkChangedFolderCount, Folders: benchmarkGalleryCount}
		}, IndexStats{})
	})
	b.Run("deleted-subtree", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, func(b *testing.B, lib *Library) IndexStats {
			gallery := benchmarkGalleryName(0)
			if err := os.RemoveAll(filepath.Join(lib.Root(), gallery)); err != nil {
				b.Fatal(err)
			}
			forceBenchmarkFolderScanStale(b, lib, "")
			return IndexStats{
				Media:   benchmarkPhotoCount - benchmarkPhotoCount/benchmarkGalleryCount,
				Folders: benchmarkGalleryCount - 1,
			}
		}, IndexStats{})
	})
	b.Run("many-tags", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, func(b *testing.B, lib *Library) IndexStats {
			paths := addBenchmarkPhotos(b, lib, benchmarkChangedFolderCount, benchmarkTaggedFilesPerDir)
			seedBenchmarkTagsForMedia(b, lib, paths, benchmarkTagsPerTaggedFile)
			return IndexStats{
				Media:   benchmarkPhotoCount + len(paths),
				Folders: benchmarkGalleryCount,
			}
		}, IndexStats{})
	})
	b.Run("many-blogs", func(b *testing.B) {
		benchmarkRebuildScenario(b, true, func(b *testing.B, lib *Library) IndexStats {
			blogs := addBenchmarkBlogs(b, lib, benchmarkChangedFolderCount, benchmarkBlogFilesPerDir)
			return IndexStats{
				Media:   benchmarkPhotoCount,
				Folders: benchmarkGalleryCount,
				Blogs:   blogs,
			}
		}, IndexStats{})
	})
}

func benchmarkRebuildScenario(b *testing.B, preindex bool, mutate func(*testing.B, *Library) IndexStats, fallback IndexStats) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		lib := newBenchmarkFilesystemLibrary(b, benchmarkGalleryCount, benchmarkPhotoCount)
		if preindex {
			if _, err := lib.RebuildIndex(ctx); err != nil {
				_ = lib.Close()
				b.Fatal(err)
			}
		}
		expected := fallback
		if mutate != nil {
			expected = mutate(b, lib)
		}
		b.StartTimer()
		stats, err := lib.RebuildIndex(ctx)
		b.StopTimer()
		if err != nil {
			_ = lib.Close()
			b.Fatal(err)
		}
		if stats.Media != expected.Media || stats.Folders != expected.Folders || stats.Blogs != expected.Blogs {
			_ = lib.Close()
			b.Fatalf("stats = %#v, want %#v", stats, expected)
		}
		if err := lib.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func newBenchmarkIndexedLibrary(b *testing.B, galleries, photos int) *Library {
	b.Helper()
	root := filepath.Join(b.TempDir(), "photos")
	if err := os.MkdirAll(root, 0o750); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < galleries; i++ {
		if err := os.Mkdir(filepath.Join(root, benchmarkGalleryName(i)), 0o750); err != nil {
			b.Fatal(err)
		}
	}
	lib, err := New(root, filepath.Join(b.TempDir(), "thumbs"), filepath.Join(b.TempDir(), "photos.db"), 120)
	if err != nil {
		b.Fatal(err)
	}
	start := time.Now()
	seedBenchmarkIndex(b, lib, galleries, photos)
	b.Logf("seeded %d photos in %d galleries into %s in %s", photos, galleries, lib.DBPath(), time.Since(start).Round(time.Millisecond))
	return lib
}

func newBenchmarkFilesystemLibrary(b *testing.B, galleries, photos int) *Library {
	b.Helper()
	root := filepath.Join(b.TempDir(), "photos")
	if err := os.MkdirAll(root, 0o750); err != nil {
		b.Fatal(err)
	}
	source := filepath.Join(root, "source.jpg")
	if err := os.WriteFile(source, nil, 0o640); err != nil {
		b.Fatal(err)
	}
	start := time.Now()
	photosPerGallery := photos / galleries
	for i := 0; i < galleries; i++ {
		dir := filepath.Join(root, benchmarkGalleryName(i))
		if err := os.Mkdir(dir, 0o750); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < photosPerGallery; j++ {
			index := i*photosPerGallery + j
			name := fmt.Sprintf("IMG_%07d.jpg", index)
			if err := os.Link(source, filepath.Join(dir, name)); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := os.Remove(source); err != nil {
		b.Fatal(err)
	}
	b.Logf("created %d photo paths in %d galleries in %s", photos, galleries, time.Since(start).Round(time.Millisecond))
	lib, err := New(root, filepath.Join(b.TempDir(), "thumbs"), filepath.Join(b.TempDir(), "photos.db"), 120)
	if err != nil {
		b.Fatal(err)
	}
	return lib
}

func addBenchmarkPhotos(b *testing.B, lib *Library, folders, perFolder int) []string {
	b.Helper()
	if folders <= 0 {
		folders = 1
	}
	paths := make([]string, 0, folders*perFolder)
	for i := 0; i < folders; i++ {
		gallery := benchmarkGalleryName(i)
		for j := 0; j < perFolder; j++ {
			name := fmt.Sprintf("NEW_%05d_%03d.jpg", i, j)
			rel := gallery + "/" + name
			if err := os.WriteFile(filepath.Join(lib.Root(), filepath.FromSlash(rel)), nil, 0o640); err != nil {
				b.Fatal(err)
			}
			paths = append(paths, rel)
		}
		forceBenchmarkFolderScanStale(b, lib, gallery)
	}
	return paths
}

func addBenchmarkBlogs(b *testing.B, lib *Library, folders, perFolder int) int {
	b.Helper()
	total := 0
	for i := 0; i < folders; i++ {
		gallery := benchmarkGalleryName(i)
		for j := 0; j < perFolder; j++ {
			name := fmt.Sprintf("story_%05d_%03d.md", i, j)
			rel := gallery + "/" + name
			body := fmt.Sprintf("<!-- @pg-date 2024-06-%02d -->\n# Benchmark Blog %05d %03d\n\nViele Blogdateien fuer den Index.", (j%28)+1, i, j)
			if err := os.WriteFile(filepath.Join(lib.Root(), filepath.FromSlash(rel)), []byte(body), 0o640); err != nil {
				b.Fatal(err)
			}
			total++
		}
		forceBenchmarkFolderScanStale(b, lib, gallery)
	}
	return total
}

func seedBenchmarkTagsForMedia(b *testing.B, lib *Library, paths []string, tagsPerFile int) {
	b.Helper()
	tx, err := lib.index.db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO media_index(path, name, directory, type, mime_type, size_bytes, mod_time_unix_nano,
			captured_at, width, height, orientation, camera, lens, latitude, longitude, keywords, tags, faces, random_hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size_bytes = excluded.size_bytes,
			mod_time_unix_nano = excluded.mod_time_unix_nano,
			tags = excluded.tags,
			random_hash = excluded.random_hash`)
	if err != nil {
		b.Fatal(err)
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, path := range paths {
		tags := make([]string, 0, tagsPerFile)
		for j := 0; j < tagsPerFile; j++ {
			tags = append(tags, fmt.Sprintf("benchmark-tag-%02d", (i+j)%tagsPerFile))
		}
		if _, err := stmt.Exec(
			path,
			filepath.Base(filepath.FromSlash(path)),
			parentPath(path),
			MediaTypeImage,
			"image/jpeg",
			int64(-1),
			int64(0),
			"",
			0,
			0,
			"",
			"",
			"",
			nil,
			nil,
			"[]",
			tagsJSONString(tags),
			"[]",
			stableHashKey(path),
			now,
		); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func forceBenchmarkFolderScanStale(b *testing.B, lib *Library, path string) {
	b.Helper()
	if _, err := lib.index.db.Exec(`UPDATE photo_folder_scan SET mod_time_unix_nano = 0 WHERE path = ?`, path); err != nil {
		b.Fatal(err)
	}
}

func seedBenchmarkIndex(b *testing.B, lib *Library, galleries, photos int) {
	b.Helper()
	if _, err := lib.index.db.Exec(`PRAGMA synchronous = OFF`); err != nil {
		b.Fatal(err)
	}
	tx, err := lib.index.db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	mediaStmt, err := tx.Prepare(`
		INSERT INTO media_index(rowid, path, name, directory, type, mime_type, size_bytes, mod_time_unix_nano,
			captured_at, width, height, orientation, camera, lens, latitude, longitude, keywords, tags, faces, random_hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer mediaStmt.Close()
	tagStmt, err := tx.Prepare(`INSERT INTO media_tag_index(media_path, tag) VALUES (?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer tagStmt.Close()
	searchStmt, err := tx.Prepare(`INSERT INTO media_search(rowid, path, search_text) VALUES (?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer searchStmt.Close()
	folderStmt, err := tx.Prepare(`
		INSERT INTO folder_index(path, parent, name, media_count, public_media_count, recursive_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count, dir_count, mod_time_unix_nano, order_mode, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer folderStmt.Close()
	scanStmt, err := tx.Prepare(`
		INSERT INTO photo_folder_scan(path, mod_time_unix_nano, quick_signature_unix_nano, order_mode, scanned_at)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer scanStmt.Close()
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	photosPerGallery := photos / galleries
	if _, err := scanStmt.Exec("", 1, 1, "ascending_date", base.Format(time.RFC3339Nano)); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < galleries; i++ {
		name := benchmarkGalleryName(i)
		modUnix := base.Add(time.Duration(i) * time.Hour).UnixNano()
		if _, err := folderStmt.Exec(name, "", name, photosPerGallery, photosPerGallery, photosPerGallery, photosPerGallery, 0, modUnix, "descending_date", base.Format(time.RFC3339Nano)); err != nil {
			b.Fatal(err)
		}
		if _, err := scanStmt.Exec(name, modUnix, modUnix, "descending_date", base.Format(time.RFC3339Nano)); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < photos; i++ {
		gallery := i / photosPerGallery
		if gallery >= galleries {
			gallery = galleries - 1
		}
		dir := benchmarkGalleryName(gallery)
		name := fmt.Sprintf("IMG_%07d.jpg", i)
		path := dir + "/" + name
		captured := base.Add(time.Duration(i) * time.Second)
		var lat any
		var lon any
		if i%10 == 0 {
			lat = float64((i%18_000)-9_000) / 100
			lon = float64((i%36_000)-18_000) / 100
		}
		keywords := fmt.Sprintf(`["gallery-%05d","event-%03d"]`, gallery, i%1000)
		tags := "[]"
		if i%1000 == 0 {
			tags = `["benchmark"]`
		}
		searchText := dir + " " + name + " BearCam PrimeLens " + keywords + " " + tags
		if _, err := mediaStmt.Exec(
			i+1,
			path,
			name,
			dir,
			MediaTypeImage,
			"image/jpeg",
			int64(1_500_000+i%90_000),
			captured.UnixNano(),
			captured.Format(time.RFC3339Nano),
			4000,
			3000,
			"landscape",
			"BearCam",
			"PrimeLens",
			lat,
			lon,
			keywords,
			tags,
			"[]",
			stableHashKey(path),
			base.Format(time.RFC3339Nano),
		); err != nil {
			b.Fatal(err)
		}
		if tags != "[]" {
			if _, err := tagStmt.Exec(path, "benchmark"); err != nil {
				b.Fatal(err)
			}
		}
		if _, err := searchStmt.Exec(i+1, path, searchText); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO photo_stats(key, value) VALUES ('media_count', ?), ('gps_media_count', ?), ('folder_count', ?), ('blog_count', 0), ('root_media_count', 0), ('root_public_media_count', 0)`, photos, photos/10, galleries); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`ANALYZE`); err != nil {
		b.Fatal(err)
	}
}

func benchmarkGalleryName(i int) string {
	return fmt.Sprintf("gallery-%05d", i)
}
