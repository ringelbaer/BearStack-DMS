package photos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"bearstack/internal/sqlutil"
)

func imageGroupLibrary(t *testing.T) *Library {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg", "20240102_Family_Trip/c.jpg", "other/d.jpg"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0444); err != nil {
			t.Fatal(err)
		}
	}
	l, err := New(root, t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestImageGroupAddMembersAtomicPrivacyAndConcurrentRevision(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	a, b, c, d := "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg", "20240102_Family_Trip/c.jpg", "other/d.jpg"
	id, err := l.CreateImageGroup(ctx, []string{a, b}, b, false)
	if err != nil {
		t.Fatal(err)
	}
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	primary := g.PrimaryID
	other, err := l.CreateImageGroup(ctx, []string{c, d}, c, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.AddImageGroupMembers(ctx, id, g.Revision, []string{c}, false); !errors.Is(err, ErrImageGroupConflict) {
		t.Fatalf("merged another group: %v", err)
	}
	otherGroup, _ := l.ImageGroup(ctx, other, false)
	if _, err = l.ApplyImageGroupAction(ctx, other, otherGroup.Revision, 0, "dissolve", false); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {c, c}, {a}, {c, "../outside.jpg"}, make([]string, MaxImageGroupSize+1)} {
		if err = l.AddImageGroupMembers(ctx, id, g.Revision, paths, false); err == nil {
			t.Fatalf("accepted %v", paths)
		}
	}
	// A private new member cannot partially add the preceding public candidate.
	marker := filepath.Join(l.root, "other/.adminonly")
	if err = os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = l.AddImageGroupMembers(ctx, id, g.Revision, []string{c, d}, false); !errors.Is(err, errAdminOnly) {
		t.Fatal(err)
	}
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	// Access to every existing member is required even when the new photo is public.
	marker = filepath.Join(l.root, "20240102_Family_Trip/.adminonly")
	if err = os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = l.AddImageGroupMembers(ctx, id, g.Revision, []string{d}, false); !errors.Is(err, errAdminOnly) {
		t.Fatal(err)
	}
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	after, err := l.ImageGroup(ctx, id, false)
	if err != nil || after.Revision != g.Revision || len(after.Members) != 2 {
		t.Fatalf("failed addition changed group: %+v %v", after, err)
	}
	// Two tabs adding to the same revision cannot overwrite one another.
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range []string{c, d} {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			errs <- l.AddImageGroupMembers(ctx, id, g.Revision, []string{p}, false)
		}(p)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrImageGroupConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflict)
	}
	after, err = l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	remaining := c
	for _, m := range after.Members {
		if m.Media.Path == c {
			remaining = d
		}
	}
	if err = l.AddImageGroupMembers(ctx, id, after.Revision, []string{remaining}, false); err != nil {
		t.Fatal(err)
	}
	after, err = l.ImageGroup(ctx, id, false)
	if err != nil || len(after.Members) != 4 || after.PrimaryID != primary {
		t.Fatalf("addition: %+v %v", after, err)
	}
	listing, err := l.List(ctx, ListOptions{Recursive: true, PageSize: 60})
	if err != nil || listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != b {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	for _, p := range []string{a, b, c, d} {
		content, e := os.ReadFile(filepath.Join(l.root, p))
		if e != nil || string(content) != p {
			t.Fatalf("original changed: %s %v", p, e)
		}
	}
}

func TestImageGroupAddDoesNotExceedSizeLimit(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	paths := []string{"20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg"}
	for i := 2; i < MaxImageGroupSize; i++ {
		p := fmt.Sprintf("limit/%03d.jpg", i)
		if err := os.MkdirAll(filepath.Join(l.root, "limit"), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(l.root, p), []byte(p), 0444); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	rebuildIdentity(t, l)
	id, err := l.CreateImageGroup(ctx, paths, paths[1], false)
	if err != nil {
		t.Fatal(err)
	}
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.AddImageGroupMembers(ctx, id, g.Revision, []string{"other/d.jpg"}, false); !errors.Is(err, ErrImageGroupAddInvalid) {
		t.Fatal(err)
	}
	after, err := l.ImageGroup(ctx, id, false)
	if err != nil || after.Revision != g.Revision || len(after.Members) != MaxImageGroupSize {
		t.Fatalf("limit changed group: %v", err)
	}
}

func TestImageGroupMissingPrimaryRestorationRelocationAndPreviews(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	dir := "20240102_Family_Trip"
	paths := []string{dir + "/a.jpg", dir + "/b.jpg", dir + "/c.jpg"}
	id, err := l.CreateImageGroup(ctx, paths, paths[1], false)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func() ([]Media, error){
		"direct":    func() ([]Media, error) { return l.filesystemDirectFolderPreviewMedia(ctx, dir, 4, false) },
		"recursive": func() ([]Media, error) { return l.filesystemFolderPreviewMedia(ctx, dir, 4, false) },
		"summary": func() ([]Media, error) {
			s, e := l.directFolderSummary(ctx, dir, filepath.Join(l.root, dir), false, 100)
			return s.Previews, e
		},
	} {
		m, e := read()
		if e != nil || len(m) != 1 || m[0].Path != paths[1] {
			t.Fatalf("%s: %+v %v", name, m, e)
		}
	}
	offline := filepath.Join(t.TempDir(), "missing.jpg")
	if err = os.Rename(filepath.Join(l.root, paths[1]), offline); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil || g.PrimaryID == initial.PrimaryID {
		t.Fatalf("fallback: %+v %v", g, err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, g.Revision, initial.PrimaryID, "primary", false); !errors.Is(err, ErrImageGroupInvalid) {
		t.Fatal(err)
	}
	if err = os.Rename(offline, filepath.Join(l.root, paths[1])); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	g, err = l.ImageGroup(ctx, id, false)
	if err != nil || g.PrimaryID != initial.PrimaryID {
		t.Fatalf("restored: %+v %v", g, err)
	}
	if err = os.Rename(filepath.Join(l.root, dir), filepath.Join(l.root, "moved_album")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	g, err = l.ImageGroup(ctx, id, false)
	if err != nil || g.PrimaryID != initial.PrimaryID {
		t.Fatalf("moved: %+v %v", g, err)
	}
	for _, m := range g.Members {
		if m.Missing || !strings.HasPrefix(m.Media.Path, "moved_album/") {
			t.Fatalf("member not moved: %+v", m)
		}
	}
	listing, err := l.List(ctx, ListOptions{Path: "moved_album", PageSize: 60})
	if err != nil || listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "moved_album/b.jpg" {
		t.Fatalf("moved flags: %+v %v", listing, err)
	}
}

func TestImageGroupConcurrentEditsAndRouteInvalidation(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	if _, err := l.index.db.Exec(`UPDATE media_index SET latitude=1,longitude=2`); err != nil {
		t.Fatal(err)
	}
	before, err := l.photoRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg", "20240102_Family_Trip/c.jpg"}
	id, err := l.CreateImageGroup(ctx, paths, paths[0], false)
	if err != nil {
		t.Fatal(err)
	}
	after, err := l.photoRouteRevision(ctx)
	if err != nil || after.Number <= before.Number {
		t.Fatalf("route revision: %+v %+v %v", before, after, err)
	}
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, m := range g.Members[1:] {
		wg.Add(1)
		go func(entity int64) {
			defer wg.Done()
			_, e := l.ApplyImageGroupAction(ctx, id, g.Revision, entity, "primary", false)
			errs <- e
		}(m.EntityID)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrImageGroupConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	for _, opts := range []ListOptions{{Path: "20240102_Family_Trip"}, {Recursive: true, Query: "-tag:excluded"}, {Recursive: true, Query: "gps:true"}} {
		listing, e := l.List(ctx, opts)
		want := 2
		if opts.Path != "" {
			want = 1
		}
		if e != nil || listing.Total != want || len(listing.Media) != want {
			t.Fatalf("%+v: %+v %v", opts, listing, e)
		}
	}
	var hidden int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM media_index INDEXED BY idx_media_image_group_hidden WHERE image_group_hidden=1`).Scan(&hidden); err != nil || hidden != 2 {
		t.Fatalf("partial index: %d %v", hidden, err)
	}
}

func TestImageGroupReadonlyMount(t *testing.T) {
	root := os.Getenv("BEARSTACK_READONLY_TEST_ROOT")
	if root == "" {
		t.Skip("requires disposable read-only mount")
	}
	if err := os.WriteFile(filepath.Join(root, "bearstack-group-write-probe"), nil, 0600); err == nil {
		os.Remove(filepath.Join(root, "bearstack-group-write-probe"))
		t.Fatal("fixture is writable")
	}
	l, err := New(root, t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	rebuildIdentity(t, l)
	listing, err := l.List(ctx, ListOptions{Recursive: true, MediaType: MediaTypeImage, PageSize: 10})
	if err != nil || len(listing.Media) < 3 {
		t.Fatalf("requires at least three read-only images: %v", err)
	}
	id, err := l.CreateImageGroup(ctx, []string{listing.Media[0].Path, listing.Media[1].Path}, listing.Media[0].Path, false)
	if err != nil {
		t.Fatal(err)
	}
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, g.Revision, g.Members[1].EntityID, "primary", false); err != nil {
		t.Fatal(err)
	}
	g, err = l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, g.Revision, g.PrimaryID, "remove", false); err != nil {
		t.Fatal(err)
	}
	id, err = l.CreateImageGroup(ctx, []string{listing.Media[0].Path, listing.Media[1].Path}, listing.Media[0].Path, false)
	if err != nil {
		t.Fatal(err)
	}
	g, err = l.ImageGroup(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.AddImageGroupMembers(ctx, id, g.Revision, []string{listing.Media[2].Path}, false); err != nil {
		t.Fatal(err)
	}
	g, err = l.ImageGroup(ctx, id, false)
	if err != nil || len(g.Members) != 3 {
		t.Fatalf("add: %+v %v", g, err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, g.Revision, 0, "dissolve", false); err != nil {
		t.Fatal(err)
	}
}
func TestImageGroupLifecycleListingsAndOriginals(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	paths := []string{"20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg", "20240102_Family_Trip/c.jpg"}
	before := map[string]os.FileInfo{}
	for _, p := range paths {
		info, _ := os.Stat(filepath.Join(l.Root(), p))
		before[p] = info
	}
	id, err := l.CreateImageGroup(ctx, paths, paths[1], false)
	if err != nil {
		t.Fatal(err)
	}
	group, err := l.ImageGroup(ctx, id, false)
	if err != nil || len(group.Members) != 3 {
		t.Fatalf("%+v %v", group, err)
	}
	if group.Members[0].DisplayPath != "Fotos / 02.01.2024 · Family Trip / a.jpg" {
		t.Fatal(group.Members[0].DisplayPath)
	}
	check := func(want []string) {
		t.Helper()
		for _, fs := range []bool{false, true} {
			for _, sort := range []string{"ascending_name", "random", "descending_date"} {
				var got []string
				for page := 1; page <= len(want)+1; page++ {
					listing, err := l.List(ctx, ListOptions{Recursive: true, Page: page, PageSize: 1, Sort: sort, FullFilesystem: fs})
					if err != nil || listing.Total != len(want) {
						t.Fatalf("fs=%v page=%d total=%d err=%v", fs, page, listing.Total, err)
					}
					for _, m := range listing.Media {
						got = append(got, m.Path)
					}
				}
				for _, p := range want {
					found := false
					for _, g := range got {
						found = found || g == p
					}
					if !found {
						t.Fatalf("want %v got %v", want, got)
					}
				}
			}
		}
	}
	check([]string{paths[1], "other/d.jpg"})
	if _, err = l.CreateImageGroup(ctx, paths[:2], paths[0], false); !errors.Is(err, ErrImageGroupConflict) {
		t.Fatal(err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, group.Revision, group.Members[2].EntityID, "primary", false); err != nil {
		t.Fatal(err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, group.Revision, 0, "dissolve", false); !errors.Is(err, ErrImageGroupConflict) {
		t.Fatal(err)
	}
	check([]string{paths[2], "other/d.jpg"})
	group, _ = l.ImageGroup(ctx, id, false)
	if _, err = l.ApplyImageGroupAction(ctx, id, group.Revision, group.PrimaryID, "remove", false); err != nil {
		t.Fatal(err)
	}
	check([]string{paths[0], paths[2], "other/d.jpg"})
	group, _ = l.ImageGroup(ctx, id, false)
	exists, err := l.ApplyImageGroupAction(ctx, id, group.Revision, group.Members[1].EntityID, "remove", false)
	if err != nil || exists {
		t.Fatalf("%v %v", exists, err)
	}
	check(append(paths, "other/d.jpg"))
	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(l.Root(), p))
		info, _ := os.Stat(filepath.Join(l.Root(), p))
		if err != nil || string(content) != p || !info.ModTime().Equal(before[p].ModTime()) || info.Mode() != before[p].Mode() {
			t.Fatalf("original changed: %s", p)
		}
	}
}
func TestImageGroupMapPrivacyAndDissolve(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	a, b := "20240102_Family_Trip/a.jpg", "other/d.jpg"
	id, err := l.CreateImageGroup(ctx, []string{a, b}, a, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`UPDATE media_index SET latitude=1,longitude=2`); err != nil {
		t.Fatal(err)
	}
	world := MapBounds{-90, -180, 90, 180}
	result, err := l.Map(ctx, ListOptions{}, world)
	if err != nil || result.Total != 3 {
		t.Fatalf("map %+v %v", result, err)
	}
	if err = os.WriteFile(filepath.Join(l.Root(), "other", ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	g, err := l.ImageGroup(ctx, id, false)
	if err != nil || len(g.Members) != 1 {
		t.Fatalf("private member leak: %+v %v", g, err)
	}
	if _, err = l.ApplyImageGroupAction(ctx, id, g.Revision, 0, "dissolve", false); !errors.Is(err, errAdminOnly) {
		t.Fatalf("private write: %v", err)
	}
	g, _ = l.ImageGroup(ctx, id, true)
	exists, err := l.ApplyImageGroupAction(ctx, id, g.Revision, 0, "dissolve", true)
	if err != nil || exists {
		t.Fatalf("dissolve %v %v", exists, err)
	}
	var hidden int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM media_index WHERE image_group_hidden=1`).Scan(&hidden); err != nil || hidden != 0 {
		t.Fatalf("remaining hidden %d %v", hidden, err)
	}
}
func TestImageGroupRejectsInvalidAndSurvivesReindex(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	a, b := "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg"
	for _, paths := range [][]string{{a}, {a, a}, {a, "../outside.jpg"}} {
		if _, err := l.CreateImageGroup(ctx, paths, a, false); err == nil {
			t.Fatal(paths)
		}
	}
	id, err := l.CreateImageGroup(ctx, []string{a, b}, b, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	group, err := l.ImageGroup(ctx, id, false)
	if err != nil || len(group.Members) != 2 {
		t.Fatalf("%+v %v", group, err)
	}
	items, err := l.MediaBatchContext(ctx, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]bool{items[0].ImageGroupHidden, items[1].ImageGroupHidden}, []bool{true, false}) {
		t.Fatal(items)
	}
}

func TestImageGroupMigrationFrom39AndDateAnchor(t *testing.T) {
	ctx := context.Background()
	l := imageGroupLibrary(t)
	rows, err := l.index.db.Query(`SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE 'image_group_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var triggers []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		triggers = append(triggers, name)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, name := range triggers {
		if _, err = l.index.db.Exec(`DROP TRIGGER "` + name + `"`); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`DROP TRIGGER photo_route_update`,
		`DROP TABLE photo_image_group_members`, `DROP TABLE photo_image_groups`,
		`DROP INDEX idx_media_image_group_hidden`, `ALTER TABLE media_index DROP COLUMN image_group_hidden`,
		`UPDATE schema_migrations SET version=39 WHERE component='photos'`,
	} {
		if _, err = l.index.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	root, cache, db := l.root, l.cacheDir, l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	version, found, err := sqlutil.CurrentSchemaVersion(ctx, l.index.db, photoSchemaComponent)
	if err != nil || !found || version != 40 {
		t.Fatalf("version=%d found=%v %v", version, found, err)
	}
	a, b := "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/b.jpg"
	if _, err = l.CreateImageGroup(ctx, []string{a, b}, a, false); err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`UPDATE media_index SET captured_at=CASE WHEN path=? THEN '2025-02-02T12:00:00Z' ELSE '2024-01-01T12:00:00Z' END`, b); err != nil {
		t.Fatal(err)
	}
	anchor, err := l.CatalogDate(ctx, "2025-02-02", false)
	if err != nil || anchor.Path == b || anchor.Date != "2024-01-01" {
		t.Fatalf("hidden date anchor: %+v %v", anchor, err)
	}
}
