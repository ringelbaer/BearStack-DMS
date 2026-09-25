package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func exportPaths(t *testing.T, l *Library, opts ListOptions) []string {
	t.Helper()
	var got []string
	if err := l.WalkExportMedia(context.Background(), opts, func(m Media) error { got = append(got, m.Path); return nil }); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}
func TestExportMediaRecursionTypesGroupsAndVirtualPagination(t *testing.T) {
	l := imageGroupLibrary(t)
	ctx := context.Background()
	root := l.Root()
	for _, name := range []string{"20240102_Family_Trip/sub/video.mp4", "20240102_Family_Trip/sub/audio.mp3", "20240102_Family_Trip/a.jpg.xmp", "20240102_Family_Trip/blog.md", "20240102_Family_Trip/.hidden.jpg", "private/hidden.jpg"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("data"), 0444); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "private/.adminonly"), nil, 0444)
	for i := 0; i < 225; i++ {
		p := filepath.Join(root, "other", fmt.Sprintf("holiday_%03d.jpg", i))
		if err := os.WriteFile(p, []byte("photo"), 0444); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	a, b := "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg"
	if _, err := l.CreateImageGroup(ctx, []string{a, b}, b, false); err != nil {
		t.Fatal(err)
	}
	got := exportPaths(t, l, ListOptions{Path: "20240102_Family_Trip"})
	want := []string{b, "20240102_Family_Trip/c.jpg", "20240102_Family_Trip/sub/audio.mp3", "20240102_Family_Trip/sub/video.mp4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recursive export %v", got)
	}
	got = exportPaths(t, l, ListOptions{Query: "holiday", Page: 2, PageSize: 60, MediaType: "image"})
	if len(got) != 225 {
		t.Fatalf("truncated search: %d", len(got))
	}
	got = exportPaths(t, l, ListOptions{MediaType: "audio"})
	if !reflect.DeepEqual(got, []string{"20240102_Family_Trip/sub/audio.mp3"}) {
		t.Fatalf("audio filter %v", got)
	}
	got = exportPaths(t, l, ListOptions{Query: "hidden"})
	if len(got) != 0 {
		t.Fatalf("private files leaked %v", got)
	}
	got = exportPaths(t, l, ListOptions{Query: "hidden", IncludeAdminOnly: true})
	if !reflect.DeepEqual(got, []string{"private/hidden.jpg"}) {
		t.Fatalf("private opt in %v", got)
	}
	var selected []string
	if err := l.WalkExportSelection(ctx, []string{a, b, b, "other/d.jpg"}, func(m Media) error { selected = append(selected, m.Path); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected, []string{b, "other/d.jpg"}) {
		t.Fatalf("selection expanded/duplicated: %v", selected)
	}
	for _, paths := range [][]string{{"../outside.jpg"}, {"20240102_Family_Trip/blog.md"}, {"20240102_Family_Trip/a.jpg.xmp"}, {"20240102_Family_Trip/.hidden.jpg"}} {
		if err := l.WalkExportSelection(ctx, paths, func(Media) error { return nil }); err == nil {
			t.Fatalf("accepted non-media %v", paths)
		}
	}
}

func TestExportPersonFiltersAndHiddenGroupMembers(t *testing.T) {
	l := imageGroupLibrary(t)
	ctx := context.Background()
	a, b := "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg"
	result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES('Thomas','thomas',1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	for _, name := range []string{a, b, "other/d.jpg"} {
		if _, err = l.index.db.Exec(`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model) VALUES(?,?,?,.1,.1,.2,.2,.99,zeroblob(512),'fixture')`, id, name, parentPath(name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = l.CreateImageGroup(ctx, []string{a, b}, b, false); err != nil {
		t.Fatal(err)
	}
	got := exportPaths(t, l, ListOptions{Path: fmt.Sprintf(".people/all/%d", id), Page: 2, PageSize: 1})
	if !reflect.DeepEqual(got, []string{b, "other/d.jpg"}) {
		t.Fatalf("person export %v", got)
	}
	got = exportPaths(t, l, ListOptions{Path: DirectoryPeoplePath("20240102_Family_Trip") + "/" + fmt.Sprint(id)})
	if !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("directory person scope %v", got)
	}
	got = exportPaths(t, l, ListOptions{Path: fmt.Sprintf(".people/all/%d", id), Query: "d.jpg"})
	if !reflect.DeepEqual(got, []string{"other/d.jpg"}) {
		t.Fatalf("person search %v", got)
	}
	got = exportPaths(t, l, ListOptions{Path: fmt.Sprintf(".people/all/%d", id), MediaType: "video"})
	if len(got) != 0 {
		t.Fatalf("person type filter %v", got)
	}
	got = exportPaths(t, l, ListOptions{Path: fmt.Sprintf(".people/all/%d", id), GPSOnly: true})
	if len(got) != 0 {
		t.Fatalf("person GPS filter %v", got)
	}
	if err = os.WriteFile(filepath.Join(l.Root(), "other/.adminonly"), nil, 0444); err != nil {
		t.Fatal(err)
	}
	got = exportPaths(t, l, ListOptions{Path: fmt.Sprintf(".people/all/%d", id), IncludeAdminOnly: true})
	if !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("private person media leaked %v", got)
	}
}

func BenchmarkExportWithConcurrentGallery50000(b *testing.B) {
	l := newBenchmarkIndexedLibrary(b, 500, 50000)
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		for {
			if ctx.Err() != nil {
				done <- nil
				return
			}
			count := 0
			if err := l.WalkExportMedia(ctx, ListOptions{}, func(Media) error { count++; return ctx.Err() }); err != nil {
				if ctx.Err() != nil {
					err = nil
				}
				done <- err
				return
			}
			if count != 50000 {
				done <- fmt.Errorf("export enumerated %d instead of 50000", count)
				return
			}
		}
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		listing, err := l.List(context.Background(), ListOptions{Path: benchmarkGalleryName(200), PageSize: 60, LeanMetadata: true, SkipFolders: true, SkipBlogs: true})
		if err != nil || len(listing.Media) != 60 {
			b.Fatalf("gallery during export %d %v", len(listing.Media), err)
		}
	}
	b.StopTimer()
	cancel()
	if err := <-done; err != nil {
		b.Fatal(err)
	}
}
