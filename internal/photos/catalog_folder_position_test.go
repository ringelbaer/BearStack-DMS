package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"testing"
)

func TestCatalogFolderPositionMatchesPagesAndVisibility(t *testing.T) {
	l := routeTestLibrary(t)
	for i := 0; i < 205; i++ {
		writeJPEG(t, filepath.Join(l.Root(), fmt.Sprintf("trip/%03d.jpg", i)), color.Black)
	}
	if _, err := l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET captured_at='2026-06-14T12:00:00Z',mod_time_unix_nano=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET admin_only=1 WHERE path='trip/204.jpg'; UPDATE media_index SET image_group_hidden=1 WHERE path='trip/203.jpg'`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		page int
	}{{"trip/202.jpg", 1}, {"trip/107.jpg", 1}, {"trip/106.jpg", 2}, {"trip/010.jpg", 3}} {
		got, err := l.CatalogFolderPosition(context.Background(), test.path)
		if err != nil || got.Page != test.page || got.Directory != "trip" {
			t.Fatalf("%+v: %+v %v", test, got, err)
		}
		listing, err := l.List(context.Background(), ListOptions{Path: got.Directory, Sort: "descending_date", Page: got.Page, PageSize: 96})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range listing.Media {
			found = found || item.Path == test.path
		}
		if !found {
			t.Fatalf("target missing from page: %+v", got)
		}
	}
	for _, path := range []string{"trip/204.jpg", "trip/203.jpg", "trip/missing.jpg"} {
		if _, err := l.CatalogFolderPosition(context.Background(), path); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.CatalogFolderPosition(ctx, "trip/010.jpg"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func BenchmarkCatalogFolderPosition100000(b *testing.B) {
	l := routeTestLibrary(b)
	_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<99999)
 INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at)
 SELECT printf('trip/%06d.jpg',n),printf('%06d.jpg',n),'trip','image','image/jpeg',1,n,'2026-09-09T12:00:00Z','2026-09-09' FROM seq`)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		position, err := l.CatalogFolderPosition(context.Background(), "trip/000000.jpg")
		if err != nil || position.Page != 1042 {
			b.Fatalf("%+v %v", position, err)
		}
	}
}
