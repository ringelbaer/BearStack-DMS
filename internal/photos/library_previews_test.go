package photos

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestFolderPreviewPercentiles(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  []int
	}{
		{0, nil}, {1, []int{1}}, {2, []int{1, 2}},
		{3, []int{1, 2, 3}}, {4, []int{1, 2, 3, 4}},
		{5, []int{1, 2, 3, 4}}, {6, []int{2, 3, 4, 5}},
		{7, []int{2, 3, 5, 6}}, {8, []int{2, 4, 5, 7}},
		{9, []int{2, 4, 6, 8}}, {10, []int{2, 4, 6, 8}},
		{100, []int{20, 40, 60, 80}},
	} {
		t.Run(fmt.Sprint(tc.count), func(t *testing.T) {
			lib := newTestLibrary(t, t.TempDir())
			defer lib.Close()
			items := previewTestMedia("album", tc.count)
			if err := lib.saveMediaBatch(context.Background(), items); err != nil {
				t.Fatal(err)
			}
			for limit := 1; limit <= MaxFolderPreviewCount; limit++ {
				var want []string
				for _, rank := range tc.want[:min(limit, len(tc.want))] {
					want = append(want, fmt.Sprintf("album/photo-%03d.jpg", rank))
				}
				if got := mediaTestPaths(selectFolderPreviewMedia(items, limit)); !slices.Equal(got, want) {
					t.Fatalf("filesystem limit %d: %v, want %v", limit, got, want)
				}
				for _, recursive := range []bool{false, true} {
					previews, err := lib.indexFolderPreviewSamples(context.Background(), []Folder{{Path: "album"}}, limit, false, recursive)
					if err != nil {
						t.Fatal(err)
					}
					if got := mediaTestPaths(previews["album"]); !slices.Equal(got, want) {
						t.Fatalf("index limit %d recursive %v: %v, want %v", limit, recursive, got, want)
					}
				}
			}
		})
	}
}

func previewTestMedia(directory string, count int) []Media {
	items := make([]Media, 0, count)
	for i := count; i >= 1; i-- {
		name := fmt.Sprintf("photo-%03d.jpg", i)
		captured := time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour)
		rating := float64(i % 6)
		items = append(items, Media{
			Name: name, Path: directory + "/" + name, Directory: directory,
			Type: MediaTypeImage, MIMEType: "image/jpeg", CapturedAt: &captured,
			ModTime: captured.Add(time.Duration(i*2) * time.Hour), Rating: &rating,
		})
	}
	return items
}

func TestFolderPreviewImagesAndMediaFallback(t *testing.T) {
	for _, imageCount := range []int{0, 2, 10} {
		t.Run(fmt.Sprint(imageCount), func(t *testing.T) {
			lib := newTestLibrary(t, t.TempDir())
			defer lib.Close()
			items := previewTestMedia("album", imageCount)
			other := previewTestMedia("album/sub", 10)
			for i := range other {
				other[i].Type = MediaTypeVideo
				if i%2 == 0 {
					other[i].Type = MediaTypeAudio
				}
			}
			items = append(items, other...)
			if err := lib.saveMediaBatch(context.Background(), items); err != nil {
				t.Fatal(err)
			}
			want := []string{"album/photo-002.jpg", "album/photo-004.jpg", "album/photo-006.jpg", "album/photo-008.jpg"}
			if imageCount == 2 {
				want = []string{"album/photo-001.jpg", "album/photo-002.jpg"}
			} else if imageCount == 0 {
				want = []string{"album/sub/photo-002.jpg", "album/sub/photo-004.jpg", "album/sub/photo-006.jpg", "album/sub/photo-008.jpg"}
			}
			previews, err := lib.indexRecursiveFolderPreviewMediaBatch(context.Background(), []Folder{{Path: "album"}}, 4, false)
			if err != nil {
				t.Fatal(err)
			}
			for source, selected := range map[string][]Media{"filesystem": selectFolderPreviewMedia(items, 4), "index": previews["album"]} {
				if got := mediaTestPaths(selected); !slices.Equal(got, want) {
					t.Fatalf("%s: %v, want %v", source, got, want)
				}
			}
		})
	}
}

func TestFolderPreviewBatchesAndDateTies(t *testing.T) {
	ctx := context.Background()
	lib := newTestLibrary(t, t.TempDir())
	defer lib.Close()
	folders := make([]Folder, 0, folderPreviewIndexBatchSize+1)
	var items []Media
	for i := 0; i <= folderPreviewIndexBatchSize; i++ {
		path := fmt.Sprintf("album-%03d", i)
		folders = append(folders, Folder{Path: path})
		media := previewTestMedia(path, 10)
		for j := range media {
			media[j].CapturedAt = nil
			media[j].ModTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		items = append(items, media...)
	}
	if err := lib.saveMediaBatch(ctx, items); err != nil {
		t.Fatal(err)
	}
	for _, recursive := range []bool{false, true} {
		previews, err := lib.indexFolderPreviewSamples(ctx, folders, 4, false, recursive)
		if err != nil {
			t.Fatal(err)
		}
		for i, folder := range folders {
			want := []string{folder.Path + "/photo-009.jpg", folder.Path + "/photo-007.jpg", folder.Path + "/photo-005.jpg", folder.Path + "/photo-003.jpg"}
			for source, selected := range map[string][]Media{
				"index": previews[folder.Path], "filesystem": selectFolderPreviewMedia(items[i*10:(i+1)*10], 4),
			} {
				if got := mediaTestPaths(selected); !slices.Equal(got, want) {
					t.Fatalf("%s recursive %v: %v, want %v", source, recursive, got, want)
				}
			}
		}
	}
}

func TestFolderPreviewCacheScopesAndLegacyEntries(t *testing.T) {
	ctx := context.Background()
	lib := newTestLibrary(t, t.TempDir())
	defer lib.Close()
	items := previewTestMedia("album", 20)
	for i := range items {
		// Every other image is hidden, giving different percentage positions.
		items[i].AdminOnly = i%2 == 0
	}
	if err := lib.saveMediaBatch(ctx, items); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`INSERT INTO folder_preview_index(folder_path, rank, media_path) VALUES ('album', 0, 'album/photo-001.jpg')`); err != nil {
		t.Fatal(err)
	}
	for _, admin := range []bool{false, true, false, true} {
		folders := []Folder{{Path: "album", MediaCount: 20}}
		if err := lib.populateFolderPreviews(ctx, folders, 4, admin, false, false); err != nil {
			t.Fatal(err)
		}
		want := []string{"album/photo-003.jpg", "album/photo-007.jpg", "album/photo-011.jpg", "album/photo-015.jpg"}
		if admin {
			want = []string{"album/photo-004.jpg", "album/photo-008.jpg", "album/photo-012.jpg", "album/photo-016.jpg"}
		}
		if got := mediaTestPaths(folders[0].Previews); !slices.Equal(got, want) {
			t.Fatalf("admin %v: %v, want %v", admin, got, want)
		}
	}
	var legacy, total int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN rank = 0 THEN 1 ELSE 0 END) FROM folder_preview_index WHERE folder_path = 'album'`).Scan(&total, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 0 || total != 8 {
		t.Fatalf("cache rows = %d legacy = %d, want 8 and 0", total, legacy)
	}
}

func TestFolderPreviewDirectFilesystemAndIndex(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		path := filepath.Join(album, fmt.Sprintf("photo-%03d.jpg", i))
		writeJPEG(t, path, color.RGBA{R: 200, A: 255})
		mod := time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	lib := newTestLibrary(t, root)
	defer lib.Close()
	want := []string{"album/photo-002.jpg", "album/photo-004.jpg", "album/photo-006.jpg", "album/photo-008.jpg"}
	assertFolderPreviewPaths(t, lib, "shallow filesystem", ListOptions{}, want)
	previews, err := lib.filesystemDirectFolderPreviewMedia(context.Background(), "album", 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := mediaTestPaths(previews); !slices.Equal(got, want) {
		t.Fatalf("direct filesystem: %v, want %v", got, want)
	}
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFolderPreviewPaths(t, lib, "index", ListOptions{}, want)
	// Reindexing changed folder contents must replace both cached selections.
	if err := os.Remove(filepath.Join(album, "photo-001.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	want = []string{"album/photo-003.jpg", "album/photo-005.jpg", "album/photo-007.jpg", "album/photo-009.jpg"}
	assertFolderPreviewPaths(t, lib, "updated public index", ListOptions{}, want)
	assertFolderPreviewPaths(t, lib, "updated admin index", ListOptions{IncludeAdminOnly: true}, want)
}
