package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"bearstack/internal/sqlutil"
)

func TestCleanPathRejectsTraversal(t *testing.T) {
	if got, err := CleanPath("family/2024"); err != nil || got != "family/2024" {
		t.Fatalf("clean path = %q %v", got, err)
	}
	if _, err := CleanPath("../secrets"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if _, err := CleanPath("/etc/passwd"); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestIndexSQLiteDSNIncludesConnectionPragmas(t *testing.T) {
	dsn, err := indexSQLiteDSN(filepath.Join(t.TempDir(), "photos.db"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pragmas := parsed.Query()["_pragma"]
	for _, want := range []string{
		"busy_timeout(5000)",
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"temp_store(MEMORY)",
	} {
		if !slices.Contains(pragmas, want) {
			t.Fatalf("missing pragma %q in %v", want, pragmas)
		}
	}
}

func TestOpenIndexDBRecordsSchemaVersion(t *testing.T) {
	db, _, err := openIndexDB(filepath.Join(t.TempDir(), "photos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	version, found, err := sqlutil.CurrentSchemaVersion(context.Background(), db, photoSchemaComponent)
	if err != nil {
		t.Fatal(err)
	}
	if !found || version != photoSchemaVersion {
		t.Fatalf("schema version = %d, found %v; want %d", version, found, photoSchemaVersion)
	}
}

func TestPhotoSchemaMigrationsEndAtSupportedVersion(t *testing.T) {
	if len(photoSchemaMigrations) == 0 {
		t.Fatal("photo schema migrations are empty")
	}
	if got := photoSchemaMigrations[len(photoSchemaMigrations)-1].Version; got != photoSchemaVersion {
		t.Fatalf("last migration version = %d, supported = %d", got, photoSchemaVersion)
	}
}

func TestOpenIndexDBRejectsNewerSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "photos.db")
	db, _, err := openIndexDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlutil.RecordSchemaVersion(ctx, db, photoSchemaComponent, photoSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, _, err := openIndexDB(dbPath)
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil {
		t.Fatal("expected newer schema version to be rejected")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestLibraryListsDirectoryFirstAndSortsByOrderFile(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "b.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "a.jpg"), color.RGBA{G: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, ".order_ascending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "c.jpg"), color.RGBA{B: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(listing.Folders) != 1 || listing.Folders[0].Name != "album" || listing.Folders[0].MediaCount != 1 {
		t.Fatalf("folders = %#v", listing.Folders)
	}
	if len(listing.Folders[0].Previews) != 1 || listing.Folders[0].Previews[0].Path != "album/c.jpg" {
		t.Fatalf("folder previews = %#v", listing.Folders[0].Previews)
	}
	if len(listing.Media) != 2 || listing.Media[0].Name != "a.jpg" || listing.Media[1].Name != "b.jpg" {
		t.Fatalf("media = %#v", listing.Media)
	}
}

func TestLibraryListsAudioMedia(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "track.mp3"), []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "cover.jpg"), color.RGBA{R: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{MediaType: MediaTypeAudio})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Media) != 1 || listing.Media[0].Name != "track.mp3" || listing.Media[0].Type != MediaTypeAudio {
		t.Fatalf("audio listing = %#v", listing.Media)
	}
	if listing.Media[0].MIMEType != "audio/mpeg" {
		t.Fatalf("audio mime type = %q", listing.Media[0].MIMEType)
	}
	if CanThumbnail(listing.Media[0].Path) {
		t.Fatal("mp3 should not enter thumbnail generation")
	}
}

func TestLibrarySearchFindsAudioMediaByFilename(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public-song.mp3"), []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "public-a.jpg"), color.RGBA{R: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.List(context.Background(), ListOptions{Sort: "ascending_name"}); err != nil {
		t.Fatal(err)
	}

	listing, err := lib.List(context.Background(), ListOptions{Query: "public-song", Sort: "ascending_name"})
	if err != nil {
		t.Fatal(err)
	}
	got := mediaTestPaths(listing.Media)
	if !slices.Equal(got, []string{"public-song.mp3"}) || listing.Total != 1 {
		t.Fatalf("audio search media = %v total = %d", got, listing.Total)
	}
}

func TestLibraryFallsBackWhenOnlyPartialMediaIndexExists(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "track.mp3"), []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	lib.saveMedia(Media{
		Name:      "photo.jpg",
		Path:      "photo.jpg",
		Directory: "",
		Type:      MediaTypeImage,
		MIMEType:  "image/jpeg",
		SizeBytes: 1,
		ModTime:   time.Now(),
	})
	listing, err := lib.List(context.Background(), ListOptions{Sort: "ascending_name"})
	if err != nil {
		t.Fatal(err)
	}
	if got := mediaTestPaths(listing.Media); !slices.Equal(got, []string{"photo.jpg", "track.mp3"}) {
		t.Fatalf("media = %v", got)
	}
	search, err := lib.List(context.Background(), ListOptions{Query: "track", PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if got := mediaTestPaths(search.Media); !slices.Equal(got, []string{"track.mp3"}) || search.Total != 1 {
		t.Fatalf("search media = %v total = %d", got, search.Total)
	}
}

func TestLibraryDBFallbackUsesShallowFolderSummaries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", "direct.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", "sub", "nested.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || listing.Folders[0].Path != "album" {
		t.Fatalf("folders = %#v", listing.Folders)
	}
	if listing.Folders[0].MediaCount != 1 || !listing.Folders[0].MediaCountApproximate {
		t.Fatalf("folder count = %#v", listing.Folders[0])
	}
	got := mediaTestPaths(listing.Folders[0].Previews)
	if !slices.Equal(got, []string{"album/direct.jpg"}) {
		t.Fatalf("folder previews = %v", got)
	}
}

func TestLibrarySearchesWithFieldFilters(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Berlin"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "Berlin", "summer.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "winter.jpg"), color.RGBA{B: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{Query: `directory:Berlin summer`})
	if err != nil {
		t.Fatal(err)
	}

	if len(listing.Media) != 1 || listing.Media[0].Path != "Berlin/summer.jpg" {
		t.Fatalf("search results = %#v", listing.Media)
	}
}

func TestLibrarySearchIgnoresCurrentPath(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, "album", "sub"),
		filepath.Join(root, "sibling"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeJPEG(t, filepath.Join(root, "needle-root.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "needle-album.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "needle-sub.jpg"), color.RGBA{B: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "sibling", "needle-sibling.jpg"), color.RGBA{R: 200, G: 200, A: 255})

	cases := []struct {
		name    string
		rebuild bool
	}{
		{name: "filesystem"},
		{name: "index", rebuild: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := ""
			if tc.rebuild {
				dbPath = filepath.Join(t.TempDir(), "photos.db")
			}
			lib, err := New(root, filepath.Join(t.TempDir(), "cache"), dbPath, 50)
			if err != nil {
				t.Fatal(err)
			}
			defer lib.Close()
			if tc.rebuild {
				if _, err := lib.RebuildIndex(context.Background()); err != nil {
					t.Fatal(err)
				}
			}

			listing, err := lib.List(context.Background(), ListOptions{Path: "album", Query: "needle", PageSize: 50})
			if err != nil {
				t.Fatal(err)
			}
			got := mediaTestPaths(listing.Media)
			slices.Sort(got)
			want := []string{"album/needle-album.jpg", "album/sub/needle-sub.jpg", "needle-root.jpg", "sibling/needle-sibling.jpg"}
			if !slices.Equal(got, want) || listing.Total != len(want) {
				t.Fatalf("root search media = %v total = %d, want %v", got, listing.Total, want)
			}
		})
	}
}

func TestLibrarySearchMatchesGermanUmlautTransliterations(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Maerz"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "maerz.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "märz.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "Maerz", "sommer.jpg"), color.RGBA{B: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	assertPhotoSearchContains(t, lib, "März", "maerz.jpg")
	assertPhotoSearchContains(t, lib, "Maerz", "märz.jpg")
	assertPhotoSearchContains(t, lib, "ä", "maerz.jpg")
	assertPhotoSearchContains(t, lib, "directory:März", "Maerz/sommer.jpg")

	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPhotoSearchContains(t, lib, "März", "maerz.jpg")
	assertPhotoSearchContains(t, lib, "Maerz", "märz.jpg")
	assertPhotoSearchContains(t, lib, "ä", "maerz.jpg")
	assertPhotoSearchContains(t, lib, "directory:März", "Maerz/sommer.jpg")
}

func TestLibraryTagFieldSearchMatchesWholeNormalizedTags(t *testing.T) {
	if matchesQuery(Media{Tags: []string{"catalog"}}, "tag:cat") {
		t.Fatal("media tag search matched partial tag")
	}
	if !matchesQuery(Media{Tags: []string{"Cat"}}, "tag:cat") {
		t.Fatal("media tag search did not normalize exact tag")
	}
	if !matchesQuery(Media{Tags: []string{"zwei worte"}}, `tag:"Zwei Worte"`) {
		t.Fatal("media tag search did not match quoted normalized tag")
	}
	if matchesBlogQuery(BlogPost{Tags: []string{"catalog"}}, "tag:cat") {
		t.Fatal("blog tag search matched partial tag")
	}
	if matchesFolderQuery(Folder{Tags: []string{"catalog"}}, "tag:cat") {
		t.Fatal("folder tag search matched partial tag")
	}
}

func TestLibraryFilesystemAndIndexSearchUseSameDirectoryOrder(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha.jpg")
	zeta := filepath.Join(root, "zeta.jpg")
	writeJPEG(t, alpha, color.RGBA{R: 200, A: 255})
	writeJPEG(t, zeta, color.RGBA{B: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, ".order_ascending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(24 * time.Hour)
	if err := os.Chtimes(alpha, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(zeta, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	fsLib, err := New(root, filepath.Join(t.TempDir(), "cache"), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	defer fsLib.Close()
	indexLib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer indexLib.Close()
	if _, err := indexLib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	opts := ListOptions{Query: "jpg", PageSize: 10}
	filesystemListing, err := fsLib.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	indexListing, err := indexLib.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if filesystemListing.Order != "ascending_name" || indexListing.Order != filesystemListing.Order {
		t.Fatalf("orders: filesystem=%q index=%q", filesystemListing.Order, indexListing.Order)
	}
	want := mediaTestPaths(filesystemListing.Media)
	got := mediaTestPaths(indexListing.Media)
	if !slices.Equal(got, want) {
		t.Fatalf("media order mismatch: filesystem=%v index=%v", want, got)
	}
	if len(got) != 2 || got[0] != "alpha.jpg" || got[1] != "zeta.jpg" {
		t.Fatalf("ordered media = %v", got)
	}
}

func TestLibraryDisplaysFolderDatesFromNames(t *testing.T) {
	root := t.TempDir()
	dated := filepath.Join(root, "2026_05_11_Demoordner")
	german := filepath.Join(root, "15.03.2024 - Urlaub")
	if err := os.Mkdir(dated, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(german, 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(dated, "photo.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(german, "photo.jpg"), color.RGBA{G: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Folder{}
	for _, folder := range listing.Folders {
		byName[folder.Name] = folder
	}
	if got := byName["2026_05_11_Demoordner"]; got.DisplayName != "Demoordner" || got.DisplayDate == nil || got.DisplayDate.Format("2006-01-02") != "2026-05-11" {
		t.Fatalf("dated folder display = %#v", got)
	}
	if got := byName["15.03.2024 - Urlaub"]; got.DisplayName != "Urlaub" || got.DisplayDate == nil || got.DisplayDate.Format("2006-01-02") != "2024-03-15" {
		t.Fatalf("german folder display = %#v", got)
	}

	child, err := lib.List(context.Background(), ListOptions{Path: "2026_05_11_Demoordner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(child.Breadcrumbs) != 2 || child.Breadcrumbs[1].DisplayName != "Demoordner" || child.Breadcrumbs[1].DisplayDate == nil {
		t.Fatalf("breadcrumbs = %#v", child.Breadcrumbs)
	}
}

func TestLibraryUsesFilenameDateFallbackForMedia(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "2024-03-15 Urlaub.jpg"), color.RGBA{R: 200, A: 255})
	videoPath := filepath.Join(root, "2024-03-16 Clip.mp4")
	if err := os.WriteFile(videoPath, []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(root, "Clip ohne Datum.mp4")
	if err := os.WriteFile(fallbackPath, []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	fallbackTime := time.Date(2026, 5, 11, 13, 14, 0, 0, time.UTC)
	if err := os.Chtimes(fallbackPath, fallbackTime, fallbackTime); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Media{}
	for _, item := range listing.Media {
		byName[item.Name] = item
	}
	for name, want := range map[string]string{
		"2024-03-15 Urlaub.jpg": "2024-03-15",
		"2024-03-16 Clip.mp4":   "2024-03-16",
		"Clip ohne Datum.mp4":   "2026-05-11",
	} {
		item := byName[name]
		if item.CapturedAt == nil || item.CapturedAt.Format("2006-01-02") != want {
			t.Fatalf("%s captured at = %#v, want %s", name, item.CapturedAt, want)
		}
	}
}

func TestLibraryReadsXMPRatingSidecar(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "rated.jpg"), color.RGBA{R: 200, A: 255})
	writeXMPRating(t, filepath.Join(root, "rated.jpg"), 4)
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	listing, err := lib.List(context.Background(), ListOptions{FullFilesystem: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Media) != 1 || listing.Media[0].Rating == nil || *listing.Media[0].Rating != 4 {
		t.Fatalf("rating = %#v", listing.Media)
	}
}

func TestParseXMPFacesAdobeMWGRegions(t *testing.T) {
	meta := parseXMPMetadataWithBase(`<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/" xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#">
<mwg-rs:Regions rdf:parseType="Resource">
<mwg-rs:RegionList><rdf:Bag>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area rdf:parseType="Resource">
<stArea:h>0.20</stArea:h><stArea:unit>normalized</stArea:unit><stArea:w>0.10</stArea:w><stArea:x>0.50</stArea:x><stArea:y>0.40</stArea:y>
</mwg-rs:Area>
<mwg-rs:Name>Marie &amp; Curie</mwg-rs:Name><mwg-rs:Type>Face</mwg-rs:Type>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area stArea:h="0.30" stArea:w="0.20" stArea:x="0.25" stArea:y="0.70"/>
<mwg-rs:Name>æÆøØåÅ</mwg-rs:Name><mwg-rs:Type>Face</mwg-rs:Type>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area stArea:h="0.10" stArea:w="0.10" stArea:x="0.10" stArea:y="0.10"/>
<mwg-rs:Name>Ignored</mwg-rs:Name><mwg-rs:Type>Pet</mwg-rs:Type>
</rdf:li>
</rdf:Bag></mwg-rs:RegionList>
</mwg-rs:Regions>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>`, Metadata{})

	if len(meta.Faces) != 2 {
		t.Fatalf("faces = %#v", meta.Faces)
	}
	assertFace(t, meta.Faces[0], "Marie & Curie", 0.45, 0.30, 0.10, 0.20)
	assertFace(t, meta.Faces[1], "æÆøØåÅ", 0.15, 0.55, 0.20, 0.30)
}

func TestParseXMPFacesAppliesOrientation(t *testing.T) {
	meta := parseXMPMetadataWithBase(`<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/" xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#" xmlns:tiff="http://ns.adobe.com/tiff/1.0/">
<tiff:Orientation>6</tiff:Orientation>
<mwg-rs:Regions rdf:parseType="Resource"><mwg-rs:RegionList><rdf:Bag>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area rdf:parseType="Resource"><stArea:h>0.10</stArea:h><stArea:w>0.20</stArea:w><stArea:x>0.30</stArea:x><stArea:y>0.60</stArea:y></mwg-rs:Area>
<mwg-rs:Name>Rotated</mwg-rs:Name><mwg-rs:Type>Face</mwg-rs:Type>
</rdf:li>
</rdf:Bag></mwg-rs:RegionList></mwg-rs:Regions>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>`, Metadata{Orientation: 1})

	if len(meta.Faces) != 1 {
		t.Fatalf("faces = %#v", meta.Faces)
	}
	assertFace(t, meta.Faces[0], "Rotated", 0.35, 0.20, 0.10, 0.20)
}

func TestLibraryIndexesAndRefreshesXMPFaceSidecars(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	facePath := filepath.Join(root, "face.jpg")
	tagPath := filepath.Join(root, "tag-only.jpg")
	writeJPEG(t, facePath, color.RGBA{R: 200, A: 255})
	writeJPEG(t, tagPath, color.RGBA{B: 200, A: 255})
	writeXMPFace(t, facePath, "Marie Curie", 0.50, 0.40, 0.20, 0.10)
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	if _, err := lib.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	media, err := lib.MediaContext(ctx, "face.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Faces) != 1 {
		t.Fatalf("faces = %#v", media.Faces)
	}
	assertFace(t, media.Faces[0], "Marie Curie", 0.40, 0.35, 0.20, 0.10)
	tags, err := lib.ListTags(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("face metadata was mirrored into tags: %#v", tags)
	}

	if _, err := lib.SetMediaTagsContext(ctx, "tag-only.jpg", []string{"Marie Curie"}); err != nil {
		t.Fatal(err)
	}
	results, err := lib.List(ctx, ListOptions{Query: `person:"Marie Curie"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Media) != 1 || results.Media[0].Path != "face.jpg" {
		t.Fatalf("person search results = %#v", results.Media)
	}
	results, err = lib.List(ctx, ListOptions{Query: "face:Marie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Media) != 1 || results.Media[0].Path != "face.jpg" {
		t.Fatalf("face search results = %#v", results.Media)
	}

	writeXMPFace(t, facePath, "Ada Lovelace", 0.40, 0.50, 0.10, 0.20)
	media, err = lib.MediaContext(ctx, "face.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Faces) != 1 {
		t.Fatalf("updated faces = %#v", media.Faces)
	}
	assertFace(t, media.Faces[0], "Ada Lovelace", 0.35, 0.40, 0.10, 0.20)

	if err := os.Remove(facePath + ".xmp"); err != nil {
		t.Fatal(err)
	}
	media, err = lib.MediaContext(ctx, "face.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Faces) != 0 {
		t.Fatalf("faces after sidecar removal = %#v", media.Faces)
	}
}

func TestOpenIndexDBMigratesIndexCountAndOrderColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "photos.db")
	dsn, err := indexSQLiteDSN(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE media_index (
		path TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		directory TEXT NOT NULL,
		type TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		mod_time_unix_nano INTEGER NOT NULL,
		captured_at TEXT NOT NULL DEFAULT '',
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		orientation TEXT NOT NULL DEFAULT '',
		camera TEXT NOT NULL DEFAULT '',
		lens TEXT NOT NULL DEFAULT '',
		rating REAL,
		latitude REAL,
		longitude REAL,
		keywords TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		admin_only INTEGER NOT NULL DEFAULT 0,
		indexed_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE photo_folder_scan (
		path TEXT PRIMARY KEY,
		mod_time_unix_nano INTEGER NOT NULL,
		quick_signature_unix_nano INTEGER NOT NULL DEFAULT 0,
		scanned_at TEXT NOT NULL
	) WITHOUT ROWID`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE folder_index (
		path TEXT PRIMARY KEY,
		parent TEXT NOT NULL,
		name TEXT NOT NULL,
		media_count INTEGER NOT NULL DEFAULT 0,
		recursive_media_count INTEGER NOT NULL DEFAULT 0,
		dir_count INTEGER NOT NULL DEFAULT 0,
		mod_time_unix_nano INTEGER NOT NULL DEFAULT 0,
		order_mode TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		admin_only INTEGER NOT NULL DEFAULT 0,
		indexed_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media_index(path, name, directory, type, mime_type, size_bytes, mod_time_unix_nano, indexed_at)
		VALUES ('photo.jpg', 'photo.jpg', '', 'image', 'image/jpeg', 12, 123, 'old')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO folder_index(path, parent, name, media_count, recursive_media_count, dir_count, mod_time_unix_nano, indexed_at)
		VALUES ('album', '', 'album', 1, 1, 0, 123, 'old')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO photo_folder_scan(path, mod_time_unix_nano, quick_signature_unix_nano, scanned_at) VALUES ('', 1, 1, 'old')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, _, err := openIndexDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	for _, column := range []string{"faces", "xmp_fingerprint", "random_hash"} {
		if !photoColumnExists(t, migrated, "media_index", column) {
			t.Fatalf("missing migrated column %s", column)
		}
	}
	for _, column := range []string{"public_media_count", "public_recursive_media_count", "recursive_blog_count", "public_recursive_blog_count"} {
		if !photoColumnExists(t, migrated, "folder_index", column) {
			t.Fatalf("missing migrated folder_index column %s", column)
		}
	}
	if !photoColumnExists(t, migrated, "photo_folder_scan", "order_mode") {
		t.Fatal("missing migrated photo_folder_scan order_mode column")
	}
	var scanRows int
	if err := migrated.QueryRow(`SELECT COUNT(*) FROM photo_folder_scan`).Scan(&scanRows); err != nil {
		t.Fatal(err)
	}
	if scanRows != 0 {
		t.Fatalf("photo_folder_scan rows after migration = %d", scanRows)
	}
	var modTime int64
	if err := migrated.QueryRow(`SELECT mod_time_unix_nano FROM media_index WHERE path = 'photo.jpg'`).Scan(&modTime); err != nil {
		t.Fatal(err)
	}
	if modTime != 0 {
		t.Fatalf("media row was not marked stale, mod_time_unix_nano = %d", modTime)
	}
	var randomHash string
	if err := migrated.QueryRow(`SELECT random_hash FROM media_index WHERE path = 'photo.jpg'`).Scan(&randomHash); err != nil {
		t.Fatal(err)
	}
	if randomHash != stableHashKey("photo.jpg") {
		t.Fatalf("random_hash = %q, want %q", randomHash, stableHashKey("photo.jpg"))
	}
	var publicMediaCount, publicRecursiveCount, recursiveBlogCount, publicRecursiveBlogCount int
	if err := migrated.QueryRow(`SELECT public_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count FROM folder_index WHERE path = 'album'`).Scan(&publicMediaCount, &publicRecursiveCount, &recursiveBlogCount, &publicRecursiveBlogCount); err != nil {
		t.Fatal(err)
	}
	if publicMediaCount != -1 || publicRecursiveCount != -1 || recursiveBlogCount != 0 || publicRecursiveBlogCount != -1 {
		t.Fatalf("migrated folder counts = public=%d recursive_public=%d blogs=%d public_blogs=%d", publicMediaCount, publicRecursiveCount, recursiveBlogCount, publicRecursiveBlogCount)
	}
}

func TestLibraryFolderPreviewsPreferDiverseRatedSubfolders(t *testing.T) {
	root := t.TempDir()
	for _, item := range []struct {
		path   string
		rating float64
		color  color.RGBA
	}{
		{"album/a/best-a.jpg", 5, color.RGBA{R: 220, A: 255}},
		{"album/a/second-a.jpg", 4, color.RGBA{R: 180, A: 255}},
		{"album/b/best-b.jpg", 3, color.RGBA{G: 220, A: 255}},
		{"album/c/best-c.jpg", 2, color.RGBA{B: 220, A: 255}},
		{"album/d/best-d.jpg", 1, color.RGBA{R: 120, G: 120, A: 255}},
	} {
		abs := filepath.Join(root, filepath.FromSlash(item.path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatal(err)
		}
		writeJPEG(t, abs, item.color)
		writeXMPRating(t, abs, item.rating)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	want := []string{"album/a/best-a.jpg", "album/b/best-b.jpg", "album/c/best-c.jpg", "album/d/best-d.jpg"}
	assertFolderPreviewPaths(t, lib, "filesystem-single", ListOptions{FullFilesystem: true, FolderPreviewSize: 1}, want[:1])

	lib, err = New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	assertFolderPreviewPaths(t, lib, "filesystem-double", ListOptions{FullFilesystem: true, FolderPreviewSize: 2}, want[:2])

	lib, err = New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	assertFolderPreviewPaths(t, lib, "filesystem", ListOptions{FullFilesystem: true}, want)
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var persisted int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM folder_preview_index WHERE folder_path = ?`, "album").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != len(want) {
		t.Fatalf("persisted previews = %d, want %d", persisted, len(want))
	}
	assertFolderPreviewPaths(t, lib, "index", ListOptions{}, want)
	if _, err := lib.index.db.Exec(`UPDATE media_index SET keywords = '["keyword"]', tags = '["tag"]', faces = '[{"Name":"Ada","X":0.1,"Y":0.2,"Width":0.3,"Height":0.4}]' WHERE path = ?`, want[0]); err != nil {
		t.Fatal(err)
	}
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || len(listing.Folders[0].Previews) == 0 {
		t.Fatalf("folder previews missing after metadata update: %#v", listing.Folders)
	}
	preview := listing.Folders[0].Previews[0]
	if len(preview.Keywords) != 0 || len(preview.Tags) != 0 || len(preview.Faces) != 0 {
		t.Fatalf("folder preview loaded full metadata: %#v", preview)
	}
	assertFolderPreviewPaths(t, lib, "index-single", ListOptions{FolderPreviewSize: 1}, want[:1])
	assertFolderPreviewPaths(t, lib, "index-double", ListOptions{FolderPreviewSize: 2}, want[:2])
}

func TestLibraryFolderPreviewCacheFiltersAdminOnlyBeforeApplyingLimit(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"album/public/one.jpg",
		"album/public/two.jpg",
		"album/secret/hidden.jpg",
	} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatal(err)
		}
		writeJPEG(t, abs, color.RGBA{R: 200, A: 255})
	}
	writeXMPRating(t, filepath.Join(root, "album", "public", "one.jpg"), 4)
	writeXMPRating(t, filepath.Join(root, "album", "public", "two.jpg"), 3)
	writeXMPRating(t, filepath.Join(root, "album", "secret", "hidden.jpg"), 5)
	if err := os.WriteFile(filepath.Join(root, "album", "secret", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var persisted int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM folder_preview_index WHERE folder_path = ?`, "album").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted == 0 {
		t.Fatal("folder preview cache was not populated")
	}

	listing, err := lib.List(context.Background(), ListOptions{FolderPreviewSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || listing.Folders[0].Path != "album" {
		t.Fatalf("folders = %#v", listing.Folders)
	}
	previews := listing.Folders[0].Previews
	if len(previews) != 2 {
		t.Fatalf("public preview count = %d, want 2: %#v", len(previews), previews)
	}
	paths := make([]string, 0, len(previews))
	for _, preview := range previews {
		if strings.HasPrefix(preview.Path, "album/secret/") {
			t.Fatalf("public preview leaks admin-only media: %#v", previews)
		}
		paths = append(paths, preview.Path)
	}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"album/public/one.jpg", "album/public/two.jpg"}) {
		t.Fatalf("public preview paths = %#v", paths)
	}
}

func TestLibraryListBackfillsMissingFolderPreviewIndex(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"album/a.jpg",
		"album/b.jpg",
		"album/c.jpg",
		"album/d.jpg",
	} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatal(err)
		}
		writeJPEG(t, abs, color.RGBA{R: 200, A: 255})
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`DELETE FROM folder_preview_index`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM folder_preview_index`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("folder preview index was not cleared: %d rows", before)
	}

	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 {
		t.Fatalf("folders = %d, want 1", len(listing.Folders))
	}
	if len(listing.Folders[0].Previews) == 0 {
		t.Fatal("folder previews were not populated")
	}

	var persisted int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM folder_preview_index WHERE folder_path = ?`, "album").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted == 0 {
		t.Fatal("folder preview index was not backfilled by list")
	}
}

func TestLibraryCreatesThumbnail(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, G: 100, A: 255})
	installFakeWebPThumbnailer(t, 120, 90)

	cacheDir := filepath.Join(t.TempDir(), "cache")
	dbPath := filepath.Join(t.TempDir(), "photos.db")
	lib, err := New(root, cacheDir, dbPath, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if lib.DBPath() != dbPath {
		t.Fatalf("db path = %q", lib.DBPath())
	}
	thumb, err := lib.Thumbnail(context.Background(), "album/photo.jpg", 120)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := lib.thumbnailCachePath("album/photo.jpg", 120)
	if thumb != wantPath {
		t.Fatalf("thumbnail path = %q, want %q", thumb, wantPath)
	}
	if info, err := os.Stat(thumb); err != nil || info.Size() == 0 {
		t.Fatalf("thumbnail stat = %v %#v", err, info)
	}
	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		t.Fatalf("photo db stat = %v %#v", err, info)
	}
}

func TestLibraryReportsThumbnailReadyWithoutCreating(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 200, G: 100, A: 255})
	installFakeWebPThumbnailer(t, 120, 90)

	cacheDir := filepath.Join(t.TempDir(), "cache")
	lib, err := New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	ready, err := lib.ThumbnailReady("photo.jpg", 120)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("thumbnail reported ready before it was created")
	}
	if lib.CachedThumbnailReady("photo.jpg", 120) {
		t.Fatal("cached thumbnail reported ready before it was created")
	}
	wantPath := lib.thumbnailCachePath("photo.jpg", 120)
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("thumbnail status created file unexpectedly: %v", err)
	}
	if _, err := lib.Thumbnail(context.Background(), "photo.jpg", 120); err != nil {
		t.Fatal(err)
	}
	ready, err = lib.ThumbnailReady("photo.jpg", 120)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("thumbnail was not reported ready after creation")
	}
	if !lib.CachedThumbnailReady("photo.jpg", 120) {
		t.Fatal("cached thumbnail was not reported ready after creation")
	}
}

func TestLibraryCachedThumbnailsReadyTrustsGeneratedIndexMetadata(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	mod := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	media := Media{
		Name:      "ready.jpg",
		Path:      "album/ready.jpg",
		Directory: "album",
		Type:      MediaTypeImage,
		SizeBytes: 42,
		ModTime:   mod,
	}
	if err := lib.markThumbnailGenerated(context.Background(), media, 420); err != nil {
		t.Fatal(err)
	}

	ready := lib.CachedThumbnailsReadyForMediaContext(context.Background(), []Media{media}, 420)
	if !ready[media.Path] {
		t.Fatalf("generated index metadata should report thumbnail as ready: %#v", ready)
	}
}

func TestLibraryThumbnailReadyBatchChunksLargeLookups(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	const count = thumbnailLookupBatchSize + 25
	requests := make([]ThumbnailReadyRequest, 0, count)
	for i := 0; i < count; i++ {
		media := Media{
			Name:      fmt.Sprintf("photo-%03d.jpg", i),
			Path:      fmt.Sprintf("album/photo-%03d.jpg", i),
			Directory: "album",
			Type:      MediaTypeImage,
			MIMEType:  "image/jpeg",
			SizeBytes: int64(100 + i),
			ModTime:   time.Date(2026, 5, 17, 10, 0, i, 0, time.UTC),
		}
		lib.saveMedia(media)
		if err := lib.markThumbnailGenerated(context.Background(), media, 120); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, ThumbnailReadyRequest{Path: media.Path, Size: 120})
	}

	ready, err := lib.ThumbnailReadyBatchContext(context.Background(), requests, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != count {
		t.Fatalf("ready count = %d, want %d", len(ready), count)
	}
	for _, request := range requests {
		if !ready[request] {
			t.Fatalf("thumbnail %q was not reported ready", request.Path)
		}
	}
}

func TestLibraryCachedThumbnailsReadyRejectsGeneratedIndexMetadataMismatch(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	mod := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	media := Media{
		Name:      "ready.jpg",
		Path:      "album/ready.jpg",
		Directory: "album",
		Type:      MediaTypeImage,
		SizeBytes: 42,
		ModTime:   mod,
	}
	if err := lib.markThumbnailGenerated(context.Background(), media, 420); err != nil {
		t.Fatal(err)
	}

	mismatched := media
	mismatched.ModTime = media.ModTime.Add(time.Second)
	ready := lib.CachedThumbnailsReadyForMediaContext(context.Background(), []Media{mismatched}, 420)
	if ready[mismatched.Path] {
		t.Fatalf("mismatched generated index metadata should not report ready: %#v", ready)
	}
}

func TestLibraryDetectsStaleThumbnailAfterSourceChange(t *testing.T) {
	root := t.TempDir()
	photoPath := filepath.Join(root, "photo.jpg")
	writeJPEG(t, photoPath, color.RGBA{R: 200, A: 255})
	installFakeWebPThumbnailer(t, 120, 90)

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	media, err := lib.MediaContext(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Thumbnail(context.Background(), "photo.jpg", 120); err != nil {
		t.Fatal(err)
	}
	if !lib.CachedThumbnailReadyForMedia(media, 120) {
		t.Fatal("thumbnail was not ready for original source")
	}

	time.Sleep(10 * time.Millisecond)
	writeJPEG(t, photoPath, color.RGBA{G: 200, A: 255})
	updated, err := lib.MediaContext(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if lib.CachedThumbnailReadyForMedia(updated, 120) {
		t.Fatal("stale thumbnail was reported ready after source change")
	}
}

func TestLibraryQueuesAndRecordsThumbnailFailures(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{G: 200, A: 255})
	t.Setenv("PATH", t.TempDir())

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if err := lib.QueueThumbnailContext(context.Background(), "photo.jpg", 120, ThumbnailVisiblePriority()); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Thumbnail(context.Background(), "photo.jpg", 120); err == nil {
		t.Fatal("expected thumbnail generation to fail without tools")
	}
	var status string
	var attempts int
	var message string
	if err := lib.index.db.QueryRow(`SELECT status, attempts, error FROM photo_thumbnail_index WHERE media_path = ? AND size = ?`, "photo.jpg", 120).Scan(&status, &attempts, &message); err != nil {
		t.Fatal(err)
	}
	if status != thumbnailStatusFailed || attempts != 1 || !strings.Contains(message, "ffmpeg") {
		t.Fatalf("thumbnail status=%q attempts=%d error=%q", status, attempts, message)
	}
}

func TestLibraryQueuesThumbnailsBatchFromIndexMetadata(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	media := Media{
		Name:      "missing.jpg",
		Path:      "missing.jpg",
		Directory: "",
		Type:      MediaTypeImage,
		MIMEType:  "image/jpeg",
		SizeBytes: 42,
		ModTime:   time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC),
	}
	lib.saveMedia(media)

	err = lib.QueueThumbnailsContext(context.Background(), []ThumbnailReadyRequest{
		{Path: "missing.jpg", Size: 120},
		{Path: "missing.jpg", Size: 120},
		{Path: "missing.jpg", Size: 240},
	}, ThumbnailVisiblePriority(), true)
	if err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM photo_thumbnail_index WHERE media_path = ?`, "missing.jpg").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("queued rows = %d, want 2", rows)
	}
	var priority int
	if err := lib.index.db.QueryRow(`SELECT priority FROM photo_thumbnail_index WHERE media_path = ? AND size = ?`, "missing.jpg", 120).Scan(&priority); err != nil {
		t.Fatal(err)
	}
	if priority != ThumbnailVisiblePriority() {
		t.Fatalf("priority = %d, want %d", priority, ThumbnailVisiblePriority())
	}
}

func TestLibraryEnsureThumbnailsLimitsInitialIndexQueueSeed(t *testing.T) {
	installFakeWebPThumbnailer(t, 120, 90)
	root := t.TempDir()
	for i := 0; i < 65; i++ {
		writeJPEG(t, filepath.Join(root, fmt.Sprintf("photo-%03d.jpg", i)), color.RGBA{R: 200, A: 255})
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	generated, err := lib.EnsureThumbnails(context.Background(), []int{120}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 {
		t.Fatalf("generated = %d, want 1", generated)
	}
	var queuedRows int
	if err := lib.index.db.QueryRow(`SELECT COUNT(*) FROM photo_thumbnail_index WHERE size = ?`, 120).Scan(&queuedRows); err != nil {
		t.Fatal(err)
	}
	if queuedRows != 50 {
		t.Fatalf("queued rows = %d, want 50", queuedRows)
	}
}

func TestLibraryCreatesVideoThumbnailWithFFmpeg(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	frame := filepath.Join(tools, "frame.jpg")
	writeSizedJPEG(t, frame, 120, 67, color.RGBA{R: 40, G: 120, B: 200, A: 255})
	ffmpeg := filepath.Join(tools, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\ncp \"$FAKE_FFMPEG_FRAME\" \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_FFMPEG_FRAME", frame)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	thumb, err := lib.Thumbnail(context.Background(), "clip.mp4", 120)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(thumb)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 120 || cfg.Height != 67 {
		t.Fatalf("video thumbnail size = %dx%d", cfg.Width, cfg.Height)
	}
}

func TestLibraryVideoThumbnailReportsMissingFFmpeg(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	cacheDir := filepath.Join(t.TempDir(), "cache")
	lib, err := New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	_, err = lib.Thumbnail(context.Background(), "clip.mp4", 120)
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("thumbnail error = %v", err)
	}
	if _, statErr := os.Stat(lib.thumbnailCachePath("clip.mp4", 120)); !os.IsNotExist(statErr) {
		t.Fatalf("failed video thumbnail left cache file: %v", statErr)
	}
}

func TestLibraryCoalescesConcurrentThumbnailRequests(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	frame := filepath.Join(tools, "frame.jpg")
	writeSizedJPEG(t, frame, 120, 67, color.RGBA{R: 40, G: 120, B: 200, A: 255})
	countFile := filepath.Join(tools, "count")
	ffmpeg := filepath.Join(tools, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nprintf x >> \"$FAKE_FFMPEG_COUNT\"\nsleep 0.2\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\ncp \"$FAKE_FFMPEG_FRAME\" \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_FFMPEG_COUNT", countFile)
	t.Setenv("FAKE_FFMPEG_FRAME", frame)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	paths := make([]string, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			paths[index], errs[index] = lib.Thumbnail(context.Background(), "clip.mp4", 120)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if paths[0] == "" || paths[0] != paths[1] {
		t.Fatalf("thumbnail paths = %#v", paths)
	}
	count, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(count) != 1 {
		t.Fatalf("ffmpeg calls = %d, want 1", len(count))
	}
}

func TestLibraryCreatesLargePreviewThumbnail(t *testing.T) {
	root := t.TempDir()
	writeSizedJPEG(t, filepath.Join(root, "photo.jpg"), 2400, 1600, color.RGBA{R: 40, G: 120, B: 200, A: 255})
	installFakeWebPThumbnailer(t, 1920, 1280)

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	thumb, err := lib.Thumbnail(context.Background(), "photo.jpg", 1920)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(thumb)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1920 || cfg.Height != 1280 {
		t.Fatalf("large preview size = %dx%d", cfg.Width, cfg.Height)
	}
}

func TestLibraryEnsureThumbnailsGeneratesMissingConfiguredSizes(t *testing.T) {
	root := t.TempDir()
	writeSizedJPEG(t, filepath.Join(root, "photo.jpg"), 2400, 1600, color.RGBA{R: 40, G: 120, B: 200, A: 255})
	installFakeWebPThumbnailer(t, 120, 80)

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	generated, err := lib.EnsureThumbnails(context.Background(), []int{120, 1920}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 {
		t.Fatalf("first batch generated = %d", generated)
	}
	generated, err = lib.EnsureThumbnails(context.Background(), []int{120, 1920}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 {
		t.Fatalf("second batch generated = %d", generated)
	}
	generated, err = lib.EnsureThumbnails(context.Background(), []int{120, 1920}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 0 {
		t.Fatalf("complete batch generated = %d", generated)
	}
}

func TestLibraryEnsureThumbnailsFromIndexHonorsBatchSize(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		writeJPEG(t, filepath.Join(root, name), color.RGBA{R: 120, B: 200, A: 255})
	}
	installFakeWebPThumbnailer(t, 120, 90)

	cacheDir := filepath.Join(t.TempDir(), "cache")
	lib, err := New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	generated, err := lib.EnsureThumbnails(context.Background(), []int{120}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 2 || countPhotoTestThumbnailFiles(t, cacheDir) != 2 {
		t.Fatalf("first indexed batch generated=%d files=%d", generated, countPhotoTestThumbnailFiles(t, cacheDir))
	}
	generated, err = lib.EnsureThumbnails(context.Background(), []int{120}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 || countPhotoTestThumbnailFiles(t, cacheDir) != 3 {
		t.Fatalf("second indexed batch generated=%d files=%d", generated, countPhotoTestThumbnailFiles(t, cacheDir))
	}
	generated, err = lib.EnsureThumbnails(context.Background(), []int{120}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 0 || countPhotoTestThumbnailFiles(t, cacheDir) != 3 {
		t.Fatalf("complete indexed batch generated=%d files=%d", generated, countPhotoTestThumbnailFiles(t, cacheDir))
	}
}

func TestLibraryEnsureThumbnailsFallsBackWhenIndexIsPartial(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "a.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "b.jpg"), color.RGBA{B: 200, A: 255})
	installFakeWebPThumbnailer(t, 120, 90)

	cacheDir := filepath.Join(t.TempDir(), "cache")
	lib, err := New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.MediaContext(context.Background(), "a.jpg"); err != nil {
		t.Fatal(err)
	}

	generated, err := lib.EnsureThumbnails(context.Background(), []int{120}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 2 {
		t.Fatalf("generated = %d, want 2", generated)
	}
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if _, err := os.Stat(lib.thumbnailCachePath(name, 120)); err != nil {
			t.Fatalf("thumbnail for %s missing: %v", name, err)
		}
	}
}

func TestLibraryEnsureThumbnailsReportsMissingImageTools(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{G: 200, A: 255})
	t.Setenv("PATH", t.TempDir())

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	generated, err := lib.EnsureThumbnails(context.Background(), []int{120}, 10)
	if err == nil {
		t.Fatal("expected missing thumbnail tools error")
	}
	if generated != 0 {
		t.Fatalf("generated = %d, want 0", generated)
	}
	message := err.Error()
	for _, want := range []string{"image thumbnail webp failed", "vipsthumbnail", "ffmpeg"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q misses %q", message, want)
		}
	}
}

func TestLibraryUsesIndexForDirectoryListing(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "album", Path: "album", MediaCount: 1, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	lib.saveMedia(Media{
		Name:       "indexed.jpg",
		Path:       "album/indexed.jpg",
		Directory:  "album",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Width:      1200,
		Height:     800,
	})

	listing, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Media) != 1 || listing.Media[0].Name != "indexed.jpg" || listing.Total != 1 {
		t.Fatalf("indexed listing = %#v total=%d", listing.Media, listing.Total)
	}
}

func TestLibraryUsesIndexedGridWhenFolderIsUnavailable(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(album, ".order_descending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(album, "a.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(album, "b.jpg"), color.RGBA{G: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(album, filepath.Join(root, "album-offline")); err != nil {
		t.Fatal(err)
	}

	listing, err := lib.List(context.Background(), ListOptions{Path: "album", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Order != "descending_name" || listing.Total != 2 {
		t.Fatalf("indexed listing order=%q total=%d", listing.Order, listing.Total)
	}
	got := mediaTestPaths(listing.Media)
	if !slices.Equal(got, []string{"album/b.jpg", "album/a.jpg"}) {
		t.Fatalf("indexed media = %#v", got)
	}
}

func TestLibraryIndexesAndOutputsRecursiveFolderMediaCounts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "direct.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "nested.jpg"), color.RGBA{G: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var directCount, recursiveCount int
	if err := lib.index.db.QueryRow(`SELECT media_count, recursive_media_count FROM folder_index WHERE path = ?`, "album").Scan(&directCount, &recursiveCount); err != nil {
		t.Fatal(err)
	}
	if directCount != 1 || recursiveCount != 2 {
		t.Fatalf("cached folder counts = direct %d recursive %d", directCount, recursiveCount)
	}

	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 {
		t.Fatalf("folders = %#v", listing.Folders)
	}
	if listing.Folders[0].DirectMediaCount != 1 || listing.Folders[0].MediaCount != 2 {
		t.Fatalf("output folder counts = direct %d recursive %d", listing.Folders[0].DirectMediaCount, listing.Folders[0].MediaCount)
	}
}

func TestLibraryListsRecursiveMedia(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "cover.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "inside.jpg"), color.RGBA{B: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	recursive, err := lib.List(context.Background(), ListOptions{Path: "album", Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, item := range recursive.Media {
		paths[item.Path] = true
	}
	if len(recursive.Media) != 2 || !paths["album/cover.jpg"] || !paths["album/sub/inside.jpg"] {
		t.Fatalf("recursive media = %#v", recursive.Media)
	}
	direct, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if len(direct.Media) != 1 || direct.Media[0].Path != "album/cover.jpg" {
		t.Fatalf("direct media = %#v", direct.Media)
	}
}

func TestLibraryIndexMediaPageQueryTraceSkippedWhenTotalZero(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "inside.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	trace := NewListTrace()
	ctx := ContextWithListTrace(context.Background(), trace)
	listing, err := lib.List(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 0 || len(listing.Media) != 0 {
		t.Fatalf("listing = total %d media %#v", listing.Total, listing.Media)
	}
	fields, ok := listTraceStepFields(trace.Snapshot(), "photos.index.media_page_query")
	if !ok {
		t.Fatal("trace missing photos.index.media_page_query")
	}
	if fields["skipped"] != "true" || fields["count"] != "0" {
		t.Fatalf("trace fields = %#v", fields)
	}
}

func TestLibraryIndexMediaPageQueryTraceSkippedWhenOffsetExceedsTotal(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "root.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	trace := NewListTrace()
	ctx := ContextWithListTrace(context.Background(), trace)
	listing, err := lib.List(ctx, ListOptions{Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 0 {
		t.Fatalf("listing = total %d media %#v", listing.Total, listing.Media)
	}
	fields, ok := listTraceStepFields(trace.Snapshot(), "photos.index.media_page_query")
	if !ok {
		t.Fatal("trace missing photos.index.media_page_query")
	}
	if fields["skipped"] != "true" || fields["count"] != "0" {
		t.Fatalf("trace fields = %#v", fields)
	}
}

func TestLibraryAdminOnlyMarkerFiltersFilesystemListings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "secret", "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "public.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "secret", "private.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "secret", "nested", "private-nested.jpg"), color.RGBA{B: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "secret", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()

	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 0 {
		t.Fatalf("public folder listing leaks admin-only folder: %#v", listing.Folders)
	}
	if len(listing.Media) != 1 || listing.Media[0].Path != "public.jpg" {
		t.Fatalf("public media listing = %#v", listing.Media)
	}

	recursive, err := lib.List(context.Background(), ListOptions{Recursive: true, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if recursive.Total != 1 || len(recursive.Media) != 1 || recursive.Media[0].Path != "public.jpg" {
		t.Fatalf("public recursive listing = total %d media %#v", recursive.Total, recursive.Media)
	}

	search, err := lib.List(context.Background(), ListOptions{Query: "private", PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Media) != 0 {
		t.Fatalf("public search leaks admin-only media: %#v", search.Media)
	}

	if _, err := lib.List(context.Background(), ListOptions{Path: "secret"}); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("admin-only folder error = %v", err)
	}
	adminListing, err := lib.List(context.Background(), ListOptions{Path: "secret", IncludeAdminOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminListing.Media) != 1 || adminListing.Media[0].Path != "secret/private.jpg" {
		t.Fatalf("admin listing = %#v", adminListing.Media)
	}
}

func TestLibraryMediaAdminOnlyBatchMatchesSingleChecks(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"public", "secret", "secret/nested"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "secret", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lib := newTestLibrary(t, root)
	defer lib.Close()

	paths := []string{
		"public/one.jpg",
		"public/two.jpg",
		"secret/private.jpg",
		"secret/nested/deeper.jpg",
		"secret/private.jpg",
	}
	batch, err := lib.MediaAdminOnlyBatch(paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		single, err := lib.MediaAdminOnly(path)
		if err != nil {
			t.Fatal(err)
		}
		if batch[path] != single {
			t.Fatalf("batch admin-only for %q = %t, single = %t", path, batch[path], single)
		}
	}
	if batch["public/one.jpg"] || !batch["secret/private.jpg"] || !batch["secret/nested/deeper.jpg"] {
		t.Fatalf("batch admin-only map = %#v", batch)
	}
}

func TestLibraryAdminOnlyMarkerFiltersIndexedListingsAndInheritedState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "public.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "private.jpg"), color.RGBA{G: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Private Reise"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	listing, err := lib.List(context.Background(), ListOptions{Recursive: true, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "public.jpg" {
		t.Fatalf("indexed public recursive listing = total %d media %#v", listing.Total, listing.Media)
	}
	if len(listing.Folders) != 0 {
		t.Fatalf("indexed public listing leaks folder: %#v", listing.Folders)
	}

	search, err := lib.List(context.Background(), ListOptions{Query: "Private", PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Media) != 0 || len(search.Blogs) != 0 {
		t.Fatalf("indexed public search leaks admin-only content: media %#v blogs %#v", search.Media, search.Blogs)
	}

	if _, err := lib.List(context.Background(), ListOptions{Path: "album/sub"}); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("nested admin-only folder error = %v", err)
	}
	adminListing, err := lib.List(context.Background(), ListOptions{Path: "album/sub", IncludeAdminOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminListing.Media) != 1 || adminListing.Media[0].Path != "album/sub/private.jpg" || !adminListing.Media[0].AdminOnly {
		t.Fatalf("admin nested listing = %#v", adminListing.Media)
	}
}

func TestLibraryIndexedFolderVisibleMediaCountsExcludeAdminOnlyDescendants(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mixed", "private"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "public"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "mixed", "visible.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "mixed", "private", "hidden.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "public", "visible.jpg"), color.RGBA{B: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "mixed", "private", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var cachedMixedDirectCount, cachedMixedCount, cachedPublicCount int
	if err := lib.index.db.QueryRow(`SELECT public_media_count, public_recursive_media_count FROM folder_index WHERE path = ?`, "mixed").Scan(&cachedMixedDirectCount, &cachedMixedCount); err != nil {
		t.Fatal(err)
	}
	if err := lib.index.db.QueryRow(`SELECT public_recursive_media_count FROM folder_index WHERE path = ?`, "public").Scan(&cachedPublicCount); err != nil {
		t.Fatal(err)
	}
	if cachedMixedDirectCount != 1 || cachedMixedCount != 1 || cachedPublicCount != 1 {
		t.Fatalf("cached public folder counts = mixed direct %d mixed recursive %d public %d", cachedMixedDirectCount, cachedMixedCount, cachedPublicCount)
	}
	if _, err := lib.index.db.Exec(`UPDATE folder_index SET public_recursive_media_count = -1 WHERE path = ?`, "mixed"); err != nil {
		t.Fatal(err)
	}

	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, folder := range listing.Folders {
		counts[folder.Path] = folder.MediaCount
	}
	if counts["mixed"] != 1 || counts["public"] != 1 {
		t.Fatalf("public folder counts = %#v", counts)
	}
	if err := lib.index.db.QueryRow(`SELECT public_recursive_media_count FROM folder_index WHERE path = ?`, "mixed").Scan(&cachedMixedCount); err != nil {
		t.Fatal(err)
	}
	if cachedMixedCount != 1 {
		t.Fatalf("fallback cached public folder count = %d", cachedMixedCount)
	}

	adminListing, err := lib.List(context.Background(), ListOptions{IncludeAdminOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	counts = map[string]int{}
	for _, folder := range adminListing.Folders {
		counts[folder.Path] = folder.MediaCount
	}
	if counts["mixed"] != 2 || counts["public"] != 1 {
		t.Fatalf("admin folder counts = %#v", counts)
	}
}

func TestLibraryStartupRefreshesPublicRecursiveCountsAfterAdminOnlyMarkerChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "private"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "visible.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "private", "hidden.jpg"), color.RGBA{G: 200, A: 255})
	marker := filepath.Join(root, "album", "private", AdminOnlyMarkerName)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "photos.db")
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), dbPath, 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var cachedCount int
	if err := lib.index.db.QueryRow(`SELECT public_recursive_media_count FROM folder_index WHERE path = ?`, "album").Scan(&cachedCount); err != nil {
		t.Fatal(err)
	}
	if cachedCount != 1 {
		t.Fatalf("cached count before marker removal = %d", cachedCount)
	}
	if err := lib.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	lib, err = New(root, filepath.Join(t.TempDir(), "cache"), dbPath, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if err := lib.index.db.QueryRow(`SELECT public_recursive_media_count FROM folder_index WHERE path = ?`, "album").Scan(&cachedCount); err != nil {
		t.Fatal(err)
	}
	if cachedCount != 2 {
		t.Fatalf("cached count after startup refresh = %d", cachedCount)
	}
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || listing.Folders[0].Path != "album" || listing.Folders[0].MediaCount != 2 {
		t.Fatalf("listing after startup refresh = %#v", listing.Folders)
	}
}

func TestLibraryHidesEmptyFoldersContainingOnlyAdminOnlyDescendants(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		"admin-only-shell/secret",
		"blog-shell/secret",
		"empty-public",
		"mixed/secret",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeJPEG(t, filepath.Join(root, "admin-only-shell", "secret", "hidden.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "blog-shell", "secret", "hidden.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "mixed", "visible.jpg"), color.RGBA{B: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "mixed", "secret", "hidden.jpg"), color.RGBA{R: 120, A: 255})
	if err := os.WriteFile(filepath.Join(root, "blog-shell", "story.md"), []byte("# Sichtbar"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"admin-only-shell/secret",
		"blog-shell/secret",
		"mixed/secret",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(dir), AdminOnlyMarkerName), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	assertRootFolders := func(label string, opts ListOptions, want []string) {
		t.Helper()
		listing, err := lib.List(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(listing.Folders))
		for _, folder := range listing.Folders {
			got = append(got, folder.Path)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("%s folders = %#v, want %#v", label, got, want)
		}
	}

	assertRootFolders("filesystem", ListOptions{FullFilesystem: true}, []string{"blog-shell", "empty-public", "mixed"})
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	var blogCount, publicBlogCount int
	if err := lib.index.db.QueryRow(`SELECT recursive_blog_count, public_recursive_blog_count FROM folder_index WHERE path = ?`, "blog-shell").Scan(&blogCount, &publicBlogCount); err != nil {
		t.Fatal(err)
	}
	if blogCount != 1 || publicBlogCount != 1 {
		t.Fatalf("cached blog-shell blog counts = all %d public %d", blogCount, publicBlogCount)
	}
	var adminPublicMediaCount, adminPublicBlogCount int
	if err := lib.index.db.QueryRow(`SELECT public_recursive_media_count, public_recursive_blog_count FROM folder_index WHERE path = ?`, "admin-only-shell").Scan(&adminPublicMediaCount, &adminPublicBlogCount); err != nil {
		t.Fatal(err)
	}
	if adminPublicMediaCount != 0 || adminPublicBlogCount != 0 {
		t.Fatalf("cached admin-only shell public counts = media %d blogs %d", adminPublicMediaCount, adminPublicBlogCount)
	}
	assertRootFolders("indexed", ListOptions{}, []string{"blog-shell", "empty-public", "mixed"})
	assertRootFolders("admin", ListOptions{IncludeAdminOnly: true}, []string{"admin-only-shell", "blog-shell", "empty-public", "mixed"})
}

func TestLibraryFilesystemAndIndexedListingParity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".order_ascending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "album", "private"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", ".order_ascending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "root.jpg"), color.RGBA{R: 220, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "b.jpg"), color.RGBA{G: 220, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "a.jpg"), color.RGBA{B: 220, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "nested.jpg"), color.RGBA{R: 120, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "private", "hidden.jpg"), color.RGBA{G: 120, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "private", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("<!-- @pg-date 2024-06-01 -->\n# Sommer\n\nReisebericht"), 0o600); err != nil {
		t.Fatal(err)
	}

	options := []ListOptions{
		{},
		{IncludeAdminOnly: true},
		{Path: "album"},
		{Path: "album", IncludeAdminOnly: true},
		{Path: "album", Recursive: true, PageSize: 50},
		{Path: "album", Recursive: true, IncludeAdminOnly: true, PageSize: 50},
		{Query: "Sommer", PageSize: 50},
	}
	for _, opts := range options {
		fs := listingDriftSummaryFor(t, root, false, opts)
		indexed := listingDriftSummaryFor(t, root, true, opts)
		if fs != indexed {
			t.Fatalf("listing drift for %#v\nfilesystem: %#v\nindexed:    %#v", opts, fs, indexed)
		}
	}
}

func TestLibraryFilesystemAndIndexedRandomOrderParity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".order_random.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.jpg", "beta.jpg", "gamma.jpg", "delta.jpg", "epsilon.jpg"} {
		writeJPEG(t, filepath.Join(root, name), color.RGBA{R: 180, A: 255})
	}

	fs := listingDriftSummaryFor(t, root, false, ListOptions{})
	indexed := listingDriftSummaryFor(t, root, true, ListOptions{})
	if fs.Media != indexed.Media {
		t.Fatalf("random order drift\nfilesystem: %#v\nindexed:    %#v", fs.Media, indexed.Media)
	}
}

func TestLibrarySortsFoldersWithRequestedSort(t *testing.T) {
	root := t.TempDir()
	names := []string{
		"02.01.2026 - Ski",
		"15.03.2024 - Urlaub",
		"30.06.2025 - Sommer",
		"Alpha",
		"zeta",
	}
	for _, name := range names {
		writePhotoFolder(t, root, name)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	random := append([]string(nil), names...)
	slices.SortFunc(random, func(a, b string) int {
		left, right := stableHash(a), stableHash(b)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})

	cases := []struct {
		sort string
		want []string
	}{
		{sort: "ascending_name", want: []string{"02.01.2026 - Ski", "15.03.2024 - Urlaub", "30.06.2025 - Sommer", "Alpha", "zeta"}},
		{sort: "descending_name", want: []string{"zeta", "Alpha", "30.06.2025 - Sommer", "15.03.2024 - Urlaub", "02.01.2026 - Ski"}},
		{sort: "ascending_date", want: []string{"15.03.2024 - Urlaub", "30.06.2025 - Sommer", "02.01.2026 - Ski", "Alpha", "zeta"}},
		{sort: "descending_date", want: []string{"02.01.2026 - Ski", "30.06.2025 - Sommer", "15.03.2024 - Urlaub", "zeta", "Alpha"}},
		{sort: "random", want: random},
	}
	for _, tc := range cases {
		assertFolderListingPaths(t, lib, "filesystem "+tc.sort, ListOptions{FullFilesystem: true, Sort: tc.sort}, tc.want)
		assertFolderListingPaths(t, lib, "indexed "+tc.sort, ListOptions{Sort: tc.sort}, tc.want)
	}
}

func TestLibrarySortsFoldersByDirectoryOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".order_descending_date.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"02.01.2026 - Ski",
		"15.03.2024 - Urlaub",
		"30.06.2025 - Sommer",
	} {
		writePhotoFolder(t, root, name)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	wantDefault := []string{"02.01.2026 - Ski", "30.06.2025 - Sommer", "15.03.2024 - Urlaub"}
	wantOverride := []string{"02.01.2026 - Ski", "15.03.2024 - Urlaub", "30.06.2025 - Sommer"}

	assertFolderListingPaths(t, lib, "filesystem folder order", ListOptions{FullFilesystem: true}, wantDefault)
	assertFolderListingPaths(t, lib, "filesystem explicit sort", ListOptions{FullFilesystem: true, Sort: "ascending_name"}, wantOverride)
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFolderListingPaths(t, lib, "indexed folder order", ListOptions{}, wantDefault)
	assertFolderListingPaths(t, lib, "indexed explicit sort", ListOptions{Sort: "ascending_name"}, wantOverride)
}

func TestLibraryRebuildIndexUpdatesModifiedBlogWhenFolderMTimeUnchanged(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	story := filepath.Join(album, "story.md")
	if err := os.WriteFile(story, []byte("<!-- @pg-date 2024-06-01 -->\n# Sommer\n\nAlt"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	albumInfo, err := os.Stat(album)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(story, []byte("<!-- @pg-date 2025-07-02 -->\n# Aktualisiert\n\nNeu"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(album, albumInfo.ModTime(), albumInfo.ModTime()); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	results, err := lib.List(context.Background(), ListOptions{Query: "Aktualisiert"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Blogs) != 1 || results.Blogs[0].Path != "album/story.md" || results.Blogs[0].Date == nil || results.Blogs[0].Date.Format("2006-01-02") != "2025-07-02" {
		t.Fatalf("updated blog results = %#v", results.Blogs)
	}
}

func TestLibraryRebuildIndexUpdatesAdminOnlyMarkerRemoval(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	marker := filepath.Join(root, "album", AdminOnlyMarkerName)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.List(context.Background(), ListOptions{Path: "album"}); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("admin-only album error = %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	listing, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "album/photo.jpg" || listing.Media[0].AdminOnly {
		t.Fatalf("listing after admin-only removal = total %d media %#v", listing.Total, listing.Media)
	}
}

func TestLibraryRebuildIndexIndexesRootAndPrunesMissingFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "cover.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{G: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("<!-- @pg-date 2024-06-01 -->\n# Sommer"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 2 || stats.Folders != 1 || stats.Blogs != 1 {
		t.Fatalf("index stats = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 2 {
		t.Fatalf("indexed recursive listing total = %d media = %#v", listing.Total, listing.Media)
	}
	search, err := lib.List(context.Background(), ListOptions{Query: "Sommer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Blogs) != 1 || search.Blogs[0].Path != "album/story.md" {
		t.Fatalf("indexed blog search = %#v", search.Blogs)
	}

	if err := os.Remove(filepath.Join(root, "album", "photo.jpg")); err != nil {
		t.Fatal(err)
	}
	stats, err = lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 || stats.Blogs != 1 {
		t.Fatalf("second index stats = %#v", stats)
	}
	listing, err = lib.List(context.Background(), ListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "cover.jpg" {
		t.Fatalf("pruned recursive listing total = %d media = %#v", listing.Total, listing.Media)
	}
}

func TestLibraryRebuildIndexPrunesDeletedFolderSubtree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "keep.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "sub", "photo.jpg"), color.RGBA{G: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Sommer\n\nBleibt nicht."), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 2 || stats.Folders != 2 || stats.Blogs != 1 {
		t.Fatalf("initial stats = %#v", stats)
	}

	if err := os.RemoveAll(filepath.Join(root, "album")); err != nil {
		t.Fatal(err)
	}
	stats, err = lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 0 || stats.Blogs != 0 {
		t.Fatalf("stats after deleted folder = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "keep.jpg" {
		t.Fatalf("listing after deleted folder = total %d media %#v", listing.Total, listing.Media)
	}
	search, err := lib.List(context.Background(), ListOptions{Query: "Sommer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Blogs) != 0 || len(search.Media) != 0 {
		t.Fatalf("stale deleted subtree search = media %#v blogs %#v", search.Media, search.Blogs)
	}
}

func TestLibraryRebuildIndexSkipsUnexpectedEmptyRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 {
		t.Fatalf("initial stats = %#v", stats)
	}
	if err := os.RemoveAll(filepath.Join(root, "album")); err != nil {
		t.Fatal(err)
	}

	stats, err = lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 {
		t.Fatalf("empty root should keep cached index stats = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "album/photo.jpg" {
		t.Fatalf("empty root should keep cached index listing = total %d media %#v", listing.Total, listing.Media)
	}
}

func TestLibraryRebuildIndexPrunesRenamedMediaAndSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(root, "album", "old-name.jpg")
	newPath := filepath.Join(root, "album", "new-name.jpg")
	writeJPEG(t, oldPath, color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 {
		t.Fatalf("stats after rename = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || len(listing.Media) != 1 || listing.Media[0].Path != "album/new-name.jpg" {
		t.Fatalf("listing after rename = total %d media %#v", listing.Total, listing.Media)
	}
	oldSearch, err := lib.List(context.Background(), ListOptions{Query: "old-name"})
	if err != nil {
		t.Fatal(err)
	}
	if len(oldSearch.Media) != 0 {
		t.Fatalf("stale renamed media search = %#v", oldSearch.Media)
	}
	newSearch, err := lib.List(context.Background(), ListOptions{Query: "new-name"})
	if err != nil {
		t.Fatal(err)
	}
	if len(newSearch.Media) != 1 || newSearch.Media[0].Path != "album/new-name.jpg" {
		t.Fatalf("new renamed media search = %#v", newSearch.Media)
	}
}

func TestLibraryRebuildIndexSkipsSystemTrashFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "$RECYCLE.BIN"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "$RECYCLE.BIN", "deleted.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{G: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 {
		t.Fatalf("stats with ignored trash folder = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || listing.Folders[0].Path != "album" {
		t.Fatalf("root folders with ignored trash folder = %#v", listing.Folders)
	}
}

func TestLibraryRebuildIndexTraversesChildrenWhenParentIsUnchanged(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(album, "first.jpg"), color.RGBA{R: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	writeJPEG(t, filepath.Join(album, "second.jpg"), color.RGBA{G: 200, A: 255})
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`UPDATE photo_folder_scan SET mod_time_unix_nano = ? WHERE path = ''`, rootInfo.ModTime().UnixNano()); err != nil {
		t.Fatal(err)
	}

	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 2 || stats.Folders != 1 {
		t.Fatalf("index stats = %#v", stats)
	}
	listing, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, item := range listing.Media {
		paths[item.Path] = true
	}
	if listing.Total != 2 || !paths["album/first.jpg"] || !paths["album/second.jpg"] {
		t.Fatalf("album listing = %#v total=%d", listing.Media, listing.Total)
	}
}

func TestLibraryRebuildIndexDoesNotRewriteUnchangedBlogsWhenFolderChanges(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(album, "story.md"), []byte("<!-- @pg-date 2024-06-01 -->\n# Sommer\n\nBleibt gleich."), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(album, "first.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	const sentinel = "2000-01-01T00:00:00Z"
	if _, err := lib.index.db.Exec(`UPDATE blog_index SET indexed_at = ? WHERE path = ?`, sentinel, "album/story.md"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	writeJPEG(t, filepath.Join(album, "second.jpg"), color.RGBA{G: 200, A: 255})
	forceFolderScanStale(t, lib, "album")
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 2 || stats.Blogs != 1 {
		t.Fatalf("stats after adding photo next to unchanged blog = %#v", stats)
	}
	var indexedAt string
	if err := lib.index.db.QueryRow(`SELECT indexed_at FROM blog_index WHERE path = ?`, "album/story.md").Scan(&indexedAt); err != nil {
		t.Fatal(err)
	}
	if indexedAt != sentinel {
		t.Fatalf("unchanged blog indexed_at = %q, want %q", indexedAt, sentinel)
	}
}

func TestLibraryRebuildIndexSkipsDelayForFreshFolders(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(album, "photo.jpg"), color.RGBA{R: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := lib.RebuildIndexWithOptions(ctx, IndexOptions{EntryDelay: time.Hour}); err != nil {
		t.Fatalf("fresh rebuild should skip entry delay: %v", err)
	}
}

func TestLibraryRebuildIndexTelemetryTracksLastRun(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(album, "photo.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	telemetry := lib.IndexTelemetry()
	if telemetry.Running || telemetry.StartedAt.IsZero() || telemetry.FinishedAt.IsZero() || telemetry.Duration <= 0 {
		t.Fatalf("telemetry timing = %#v", telemetry)
	}
	if telemetry.Stats != stats {
		t.Fatalf("telemetry stats = %#v, want %#v", telemetry.Stats, stats)
	}
	if telemetry.ScannedFolders != 2 || telemetry.SkippedFolders != 0 || telemetry.Files != 1 || telemetry.DBWrites == 0 || telemetry.FilesPerSecond <= 0 {
		t.Fatalf("telemetry first run = %#v", telemetry)
	}

	stats, err = lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	telemetry = lib.IndexTelemetry()
	if telemetry.Stats != stats {
		t.Fatalf("warm telemetry stats = %#v, want %#v", telemetry.Stats, stats)
	}
	if telemetry.ScannedFolders != 0 || telemetry.SkippedFolders != 2 || telemetry.Files != 0 || telemetry.DBWrites != 0 || len(telemetry.LastErrors) != 0 {
		t.Fatalf("telemetry warm run = %#v", telemetry)
	}
}

func TestLibraryRebuildIndexPreservesTagsAcrossRescan(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	photoPath := filepath.Join(root, "album", "photo.jpg")
	writeJPEG(t, photoPath, color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Sommer\n\nTagbarer Blog"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", []string{"Reise"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"Familie"}); err != nil {
		t.Fatal(err)
	}
	post, err := lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	post.Tags = []string{"Journal"}
	lib.saveBlog(post)

	time.Sleep(10 * time.Millisecond)
	writeJPEG(t, photoPath, color.RGBA{G: 200, A: 255})
	forceFolderScanStale(t, lib, "album")
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}

	mediaResults, err := lib.List(context.Background(), ListOptions{Query: "tag:reise"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mediaResults.Media) != 1 || mediaResults.Media[0].Path != "album/photo.jpg" || len(mediaResults.Media[0].Tags) != 1 || mediaResults.Media[0].Tags[0] != "reise" {
		t.Fatalf("media tag after rebuild = %#v", mediaResults.Media)
	}
	folderResults, err := lib.List(context.Background(), ListOptions{Query: "tag:familie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(folderResults.Folders) != 1 || folderResults.Folders[0].Path != "album" || len(folderResults.Folders[0].Tags) != 1 || folderResults.Folders[0].Tags[0] != "familie" {
		t.Fatalf("folder tag after rebuild = %#v", folderResults.Folders)
	}
	blogResults, err := lib.List(context.Background(), ListOptions{Query: "tag:journal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blogResults.Blogs) != 1 || blogResults.Blogs[0].Path != "album/story.md" || len(blogResults.Blogs[0].Tags) != 1 || blogResults.Blogs[0].Tags[0] != "journal" {
		t.Fatalf("blog tag after rebuild = %#v", blogResults.Blogs)
	}
}

func TestLibraryRebuildIndexIgnoresSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.Symlink(filepath.Join(root, "album", "photo.jpg"), filepath.Join(root, "album", "linked-photo.jpg")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "album"), filepath.Join(root, "linked-album")); err != nil {
		t.Skipf("directory symlinks not available: %v", err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	stats, err := lib.RebuildIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Media != 1 || stats.Folders != 1 {
		t.Fatalf("stats with symlinks = %#v", stats)
	}
	rootListing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rootListing.Folders) != 1 || rootListing.Folders[0].Path != "album" {
		t.Fatalf("root folders with symlink = %#v", rootListing.Folders)
	}
	albumListing, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if albumListing.Total != 1 || len(albumListing.Media) != 1 || albumListing.Media[0].Path != "album/photo.jpg" {
		t.Fatalf("album media with symlink = total %d media %#v", albumListing.Total, albumListing.Media)
	}
}

func TestLibraryRebuildIndexCanBeCancelledDuringDelay(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 200, A: 255})

	lib := newTestLibrary(t, root)
	defer lib.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := lib.RebuildIndexWithOptions(ctx, IndexOptions{EntryDelay: time.Hour})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rebuild err = %v, want deadline exceeded", err)
	}
}

func TestLibraryUsesIndexForRecursiveListing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "album", Path: "album", MediaCount: 1, DirCount: 1, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	if err := lib.saveFolder(Folder{Name: "sub", Path: "album/sub", MediaCount: 1, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []Media{
		{Name: "cover.jpg", Path: "album/cover.jpg", Directory: "album"},
		{Name: "inside.jpg", Path: "album/sub/inside.jpg", Directory: "album/sub"},
	} {
		item.Type = MediaTypeImage
		item.MIMEType = "image/jpeg"
		item.SizeBytes = 12
		item.ModTime = captured
		item.CapturedAt = &captured
		lib.saveMedia(item)
	}

	recursive, err := lib.List(context.Background(), ListOptions{Path: "album", Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, item := range recursive.Media {
		paths[item.Path] = true
	}
	if len(recursive.Media) != 2 || !paths["album/cover.jpg"] || !paths["album/sub/inside.jpg"] {
		t.Fatalf("recursive indexed media = %#v", recursive.Media)
	}
	direct, err := lib.List(context.Background(), ListOptions{Path: "album"})
	if err != nil {
		t.Fatal(err)
	}
	if len(direct.Media) != 1 || direct.Media[0].Path != "album/cover.jpg" {
		t.Fatalf("direct indexed media = %#v", direct.Media)
	}
}

func TestLibraryLeanMetadataSkipsTagKeywordFaceDecoding(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	captured := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "album", Path: "album", MediaCount: 1, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	lib.saveMedia(Media{
		Name:       "photo.jpg",
		Path:       "album/photo.jpg",
		Directory:  "album",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Latitude:   float64Ptr(52.52),
		Longitude:  float64Ptr(13.405),
		Keywords:   []string{"berlin", "trip"},
		Tags:       []string{"favorite"},
		Faces: []Face{
			{Name: "Ada"},
		},
	})

	full, err := lib.List(context.Background(), ListOptions{Path: "album", Recursive: true, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Media) != 1 {
		t.Fatalf("full media = %#v", full.Media)
	}
	if len(full.Media[0].Keywords) == 0 || len(full.Media[0].Tags) == 0 || len(full.Media[0].Faces) == 0 {
		t.Fatalf("full metadata should be decoded, got keywords=%v tags=%v faces=%v", full.Media[0].Keywords, full.Media[0].Tags, full.Media[0].Faces)
	}

	lean, err := lib.List(context.Background(), ListOptions{Path: "album", Recursive: true, PageSize: 10, LeanMetadata: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(lean.Media) != 1 {
		t.Fatalf("lean media = %#v", lean.Media)
	}
	if len(lean.Media[0].Keywords) != 0 || len(lean.Media[0].Tags) != 0 || len(lean.Media[0].Faces) != 0 {
		t.Fatalf("lean metadata should stay empty, got keywords=%v tags=%v faces=%v", lean.Media[0].Keywords, lean.Media[0].Tags, lean.Media[0].Faces)
	}
	if lean.Media[0].Latitude == nil || lean.Media[0].Longitude == nil {
		t.Fatalf("lean coordinates missing: %#v", lean.Media[0])
	}

	leanTagQuery, err := lib.List(context.Background(), ListOptions{
		Path:         "album",
		Recursive:    true,
		Query:        "tag:favorite",
		PageSize:     10,
		LeanMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leanTagQuery.Media) != 1 || leanTagQuery.Total != 1 {
		t.Fatalf("lean tag query media = %#v total=%d", leanTagQuery.Media, leanTagQuery.Total)
	}
	if len(leanTagQuery.Media[0].Keywords) != 0 || len(leanTagQuery.Media[0].Tags) != 0 || len(leanTagQuery.Media[0].Faces) != 0 {
		t.Fatalf("lean tag query metadata should stay empty, got keywords=%v tags=%v faces=%v", leanTagQuery.Media[0].Keywords, leanTagQuery.Media[0].Tags, leanTagQuery.Media[0].Faces)
	}

	leanPersonQuery, err := lib.List(context.Background(), ListOptions{
		Path:         "album",
		Recursive:    true,
		Query:        "person:ada",
		PageSize:     10,
		LeanMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leanPersonQuery.Media) != 1 || leanPersonQuery.Total != 1 {
		t.Fatalf("lean person query media = %#v total=%d", leanPersonQuery.Media, leanPersonQuery.Total)
	}
	if len(leanPersonQuery.Media[0].Keywords) != 0 || len(leanPersonQuery.Media[0].Tags) != 0 || len(leanPersonQuery.Media[0].Faces) != 0 {
		t.Fatalf("lean person query metadata should stay empty, got keywords=%v tags=%v faces=%v", leanPersonQuery.Media[0].Keywords, leanPersonQuery.Media[0].Tags, leanPersonQuery.Media[0].Faces)
	}
}

func TestLibraryBuildsAggregatedRoutePoints(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "trip"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	base := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "trip", Path: "trip", MediaCount: 4, ModTime: base}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		at   time.Time
		lat  float64
		lon  float64
	}{
		{"a1.jpg", base, 52.52000, 13.40500},
		{"a2.jpg", base.Add(45 * time.Minute), 52.52045, 13.40540},
		{"b.jpg", base.Add(2 * time.Hour), 52.50000, 13.37000},
		{"a3.jpg", base.Add(3 * time.Hour), 52.52020, 13.40520},
	} {
		lib.saveMedia(Media{
			Name:       item.name,
			Path:       "trip/" + item.name,
			Directory:  "trip",
			Type:       MediaTypeImage,
			MIMEType:   "image/jpeg",
			SizeBytes:  12,
			ModTime:    item.at,
			CapturedAt: &item.at,
			Latitude:   float64Ptr(item.lat),
			Longitude:  float64Ptr(item.lon),
		})
	}

	listing, err := lib.List(context.Background(), ListOptions{Path: "trip", GPSOnly: true, Recursive: true, IncludeMapData: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.RoutePoints) != 3 {
		t.Fatalf("route points = %#v", listing.RoutePoints)
	}
	if listing.RoutePoints[0].Count != 2 || listing.RoutePoints[1].Count != 1 || listing.RoutePoints[2].Count != 1 {
		t.Fatalf("route point counts = %#v", listing.RoutePoints)
	}
	if !listing.RoutePoints[0].StartedAt.Equal(base) || !listing.RoutePoints[1].StartedAt.Equal(base.Add(2*time.Hour)) || !listing.RoutePoints[2].StartedAt.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("route point order = %#v", listing.RoutePoints)
	}
	if routeDistanceMeters(listing.RoutePoints[0].Lat, listing.RoutePoints[0].Lon, listing.RoutePoints[2].Lat, listing.RoutePoints[2].Lon) > DefaultRouteClusterRadiusMeters {
		t.Fatalf("return point is not near first point: %#v", listing.RoutePoints)
	}

	coarse, err := lib.List(context.Background(), ListOptions{Path: "trip", GPSOnly: true, Recursive: true, IncludeMapData: true, RouteClusterRadiusMeters: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if len(coarse.RoutePoints) != 1 || coarse.RoutePoints[0].Count != 4 {
		t.Fatalf("coarse route points = %#v", coarse.RoutePoints)
	}

	fine, err := lib.List(context.Background(), ListOptions{Path: "trip", GPSOnly: true, Recursive: true, IncludeMapData: true, RouteClusterRadiusMeters: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(fine.RoutePoints) != 3 {
		t.Fatalf("fine route points = %#v", fine.RoutePoints)
	}
}

func TestLibrarySkipsMapDataWhenDisabled(t *testing.T) {
	root := t.TempDir()
	tripDir := filepath.Join(root, "trip")
	if err := os.MkdirAll(tripDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tripDir, "track.gpx"), []byte(`<?xml version="1.0"?>
<gpx><trk><trkseg>
<trkpt lat="52.520000" lon="13.405000"></trkpt>
<trkpt lat="52.521000" lon="13.406000"></trkpt>
</trkseg></trk></gpx>`), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	captured := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "trip", Path: "trip", MediaCount: 1, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	lib.saveMedia(Media{
		Name:       "photo.jpg",
		Path:       "trip/photo.jpg",
		Directory:  "trip",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Latitude:   float64Ptr(52.52),
		Longitude:  float64Ptr(13.405),
	})

	withoutMap, err := lib.List(context.Background(), ListOptions{Path: "trip", Recursive: true, GPSOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutMap.GPXTracks) != 0 || len(withoutMap.RoutePoints) != 0 {
		t.Fatalf("unexpected map data without IncludeMapData: tracks=%d route=%d", len(withoutMap.GPXTracks), len(withoutMap.RoutePoints))
	}

	withMap, err := lib.List(context.Background(), ListOptions{Path: "trip", Recursive: true, GPSOnly: true, IncludeMapData: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withMap.GPXTracks) == 0 || len(withMap.RoutePoints) == 0 {
		t.Fatalf("expected map data with IncludeMapData=true, tracks=%d route=%d", len(withMap.GPXTracks), len(withMap.RoutePoints))
	}
}

func TestLibraryCollectsReducedGPXTracksFromIndexedListing(t *testing.T) {
	root := t.TempDir()
	tripDir := filepath.Join(root, "album", "trip")
	if err := os.MkdirAll(tripDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tripDir, "2024-06-01-run.gpx"), []byte(`<?xml version="1.0"?>
<gpx><trk><trkseg>
<trkpt lat="52.520000" lon="13.405000"></trkpt>
<trkpt lat="52.520010" lon="13.405000"></trkpt>
<trkpt lat="52.520120" lon="13.405000"></trkpt>
</trkseg></trk></gpx>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tripDir, "wanderung.gpx"), []byte(`<?xml version="1.0"?>
<gpx><trk><trkseg>
<trkpt lat="52.500000" lon="13.370000"></trkpt>
<trkpt lat="52.500100" lon="13.370000"></trkpt>
</trkseg></trk></gpx>`), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	captured := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	lib.saveMedia(Media{
		Name:       "photo.jpg",
		Path:       "album/trip/photo.jpg",
		Directory:  "album/trip",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Latitude:   float64Ptr(52.52),
		Longitude:  float64Ptr(13.405),
	})

	listing, err := lib.List(context.Background(), ListOptions{Path: "album/trip", Recursive: true, GPSOnly: true, IncludeMapData: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.GPXTracks) != 2 {
		t.Fatalf("gpx tracks = %#v", listing.GPXTracks)
	}
	if listing.GPXTracks[0].Name != "2024-06-01-run.gpx" || listing.GPXTracks[0].Label != "01.06.2024" {
		t.Fatalf("dated track label = %#v", listing.GPXTracks[0])
	}
	if len(listing.GPXTracks[0].Points) != 2 {
		t.Fatalf("reduced points = %#v", listing.GPXTracks[0].Points)
	}
	if listing.GPXTracks[1].Label != "wanderung.gpx" {
		t.Fatalf("fallback track label = %#v", listing.GPXTracks[1])
	}
	if !strings.HasPrefix(listing.GPXTracks[0].Color, "#") || listing.GPXTracks[0].Color == listing.GPXTracks[1].Color {
		t.Fatalf("track colors = %#v %#v", listing.GPXTracks[0].Color, listing.GPXTracks[1].Color)
	}
}

func TestLibraryInvalidatesGPXCacheWhenFileChanges(t *testing.T) {
	root := t.TempDir()
	trackDir := filepath.Join(root, "album")
	if err := os.MkdirAll(trackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	trackPath := filepath.Join(trackDir, "route.gpx")
	if err := os.WriteFile(trackPath, []byte(`<?xml version="1.0"?>
<gpx><trk><trkseg>
<trkpt lat="52.520000" lon="13.405000"></trkpt>
</trkseg></trk></gpx>`), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	initial, err := lib.gpxFromPath("album/route.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Points) != 1 {
		t.Fatalf("initial track points = %#v", initial.Points)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(trackPath, []byte(`<?xml version="1.0"?>
<gpx><trk><trkseg>
<trkpt lat="52.520000" lon="13.405000"></trkpt>
<trkpt lat="52.521000" lon="13.406000"></trkpt>
</trkseg></trk></gpx>`), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := lib.gpxFromPath("album/route.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Points) != 2 {
		t.Fatalf("updated track points = %#v", updated.Points)
	}
}

func TestLibraryUsesIndexForSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Berlin"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "Berlin", Path: "Berlin", MediaCount: 2, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	lib.saveMedia(Media{
		Name:       "summer.jpg",
		Path:       "Berlin/summer.jpg",
		Directory:  "Berlin",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Camera:     "BearCam",
	})
	lib.saveMedia(Media{
		Name:       "winter.jpg",
		Path:       "Berlin/winter.jpg",
		Directory:  "Berlin",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Camera:     "BearCam",
	})

	listing, err := lib.List(context.Background(), ListOptions{Query: `directory:Berlin summer`})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Media) != 1 || listing.Media[0].Path != "Berlin/summer.jpg" {
		t.Fatalf("indexed search = %#v", listing.Media)
	}
}

func TestLibraryIndexedSearchORMatchesAnyTerm(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "album", Path: "album", MediaCount: 3, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"summer.jpg", "winter.jpg", "autumn.jpg"} {
		lib.saveMedia(Media{
			Name:       name,
			Path:       "album/" + name,
			Directory:  "album",
			Type:       MediaTypeImage,
			MIMEType:   "image/jpeg",
			SizeBytes:  12,
			ModTime:    captured,
			CapturedAt: &captured,
		})
	}

	listing, err := lib.List(context.Background(), ListOptions{Query: "summer or winter"})
	if err != nil {
		t.Fatal(err)
	}
	got := mediaTestPaths(listing.Media)
	slices.Sort(got)
	want := []string{"album/summer.jpg", "album/winter.jpg"}
	if !slices.Equal(got, want) || listing.Total != len(want) {
		t.Fatalf("indexed OR search media = %v total = %d, want %v", got, listing.Total, want)
	}
}

func TestLibraryIndexedSearchORDoesNotANDTagTerms(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveFolder(Folder{Name: "album", Path: "album", MediaCount: 3, ModTime: captured}); err != nil {
		t.Fatal(err)
	}
	items := []Media{
		{Name: "urlaub.jpg", Path: "album/urlaub.jpg", Tags: []string{"urlaub"}},
		{Name: "winter.jpg", Path: "album/winter.jpg", Tags: []string{"winter"}},
		{Name: "neutral.jpg", Path: "album/neutral.jpg"},
	}
	for _, item := range items {
		item.Directory = "album"
		item.Type = MediaTypeImage
		item.MIMEType = "image/jpeg"
		item.SizeBytes = 12
		item.ModTime = captured
		item.CapturedAt = &captured
		lib.saveMedia(item)
	}

	listing, err := lib.List(context.Background(), ListOptions{Query: "tag:urlaub or tag:winter"})
	if err != nil {
		t.Fatal(err)
	}
	got := mediaTestPaths(listing.Media)
	slices.Sort(got)
	want := []string{"album/urlaub.jpg", "album/winter.jpg"}
	if !slices.Equal(got, want) || listing.Total != len(want) {
		t.Fatalf("indexed tag OR search media = %v total = %d, want %v", got, listing.Total, want)
	}

	listing, err = lib.List(context.Background(), ListOptions{Query: "-tag:winter"})
	if err != nil {
		t.Fatal(err)
	}
	got = mediaTestPaths(listing.Media)
	slices.Sort(got)
	want = []string{"album/neutral.jpg", "album/urlaub.jpg"}
	if !slices.Equal(got, want) || listing.Total != len(want) {
		t.Fatalf("indexed negated tag search media = %v total = %d, want %v", got, listing.Total, want)
	}
}

func TestLibraryIndexedSearchUsesSQLForGPSFalseAndResolution(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := lib.saveMediaContext(context.Background(), Media{
		Name:       "mapped.jpg",
		Path:       "mapped.jpg",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Width:      4000,
		Height:     3000,
		Latitude:   float64Ptr(52.52),
		Longitude:  float64Ptr(13.405),
	}); err != nil {
		t.Fatal(err)
	}
	if err := lib.saveMediaContext(context.Background(), Media{
		Name:       "unmapped.jpg",
		Path:       "unmapped.jpg",
		Type:       MediaTypeImage,
		MIMEType:   "image/jpeg",
		SizeBytes:  12,
		ModTime:    captured,
		CapturedAt: &captured,
		Width:      1600,
		Height:     1000,
	}); err != nil {
		t.Fatal(err)
	}

	noGPS, err := lib.List(context.Background(), ListOptions{Query: "gps:false", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := mediaTestPaths(noGPS.Media); !slices.Equal(got, []string{"unmapped.jpg"}) || noGPS.Total != 1 {
		t.Fatalf("gps:false media = %v total = %d", got, noGPS.Total)
	}

	large, err := lib.List(context.Background(), ListOptions{Query: "resolution:>=10", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := mediaTestPaths(large.Media); !slices.Equal(got, []string{"mapped.jpg"}) || large.Total != 1 {
		t.Fatalf("resolution media = %v total = %d", got, large.Total)
	}
}

func TestLibraryPostFilterSearchTooBroad(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	oldMax := indexPostFilterCandidateMax
	indexPostFilterCandidateMax = 3
	defer func() { indexPostFilterCandidateMax = oldMax }()

	captured := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		media := Media{
			Name:      fmt.Sprintf("photo-%d.jpg", i),
			Path:      fmt.Sprintf("photo-%d.jpg", i),
			Type:      MediaTypeImage,
			MIMEType:  "image/jpeg",
			SizeBytes: 12,
			ModTime:   captured.Add(time.Duration(i) * time.Second),
			Camera:    "BearCam",
			Lens:      "PrimeLens",
		}
		if err := lib.saveMediaContext(context.Background(), media); err != nil {
			t.Fatal(err)
		}
	}

	_, err = lib.List(context.Background(), ListOptions{Query: "2-of:(camera:BearCam,lens:PrimeLens)", PageSize: 2})
	if !errors.Is(err, ErrSearchTooBroad()) {
		t.Fatalf("err = %v, want search-too-broad", err)
	}
}

func TestLibrarySearchesMediaAndFolderTags(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "urlaub-name-only.jpg"), color.RGBA{G: 200, A: 255})

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.List(context.Background(), ListOptions{Path: "album"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", []string{"Urlaub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"Familie"}); err != nil {
		t.Fatal(err)
	}

	mediaResults, err := lib.List(context.Background(), ListOptions{Query: "tag:urlaub"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mediaResults.Media) != 1 || mediaResults.Media[0].Tags[0] != "urlaub" {
		t.Fatalf("media tag results = %#v", mediaResults.Media)
	}
	folderResults, err := lib.List(context.Background(), ListOptions{Query: "tag:familie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(folderResults.Folders) != 1 || folderResults.Folders[0].Path != "album" || folderResults.Folders[0].Tags[0] != "familie" {
		t.Fatalf("folder tag results = %#v", folderResults.Folders)
	}
}

func TestLibraryClearingPhotoTagsRemovesAssignments(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Tags\n\nBlogtext"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.List(context.Background(), ListOptions{Path: "album"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", []string{"Clear"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"Clear"}); err != nil {
		t.Fatal(err)
	}
	post, err := lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	post.Tags = []string{"Clear"}
	lib.saveBlog(post)

	tag, err := lib.GetTag(context.Background(), "clear")
	if err != nil {
		t.Fatal(err)
	}
	if tag.Count != 3 {
		t.Fatalf("tag before clearing = %#v", tag)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"  "}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`UPDATE blog_index SET tags = '[]' WHERE path = ?`, post.Path); err != nil {
		t.Fatal(err)
	}
	post.Tags = nil
	if err := lib.saveBlogBatch(context.Background(), []BlogPost{post}); err != nil {
		t.Fatal(err)
	}

	tag, err = lib.GetTag(context.Background(), "clear")
	if err != nil {
		t.Fatal(err)
	}
	if tag.Count != 0 {
		t.Fatalf("tag after clearing = %#v", tag)
	}
	results, err := lib.List(context.Background(), ListOptions{Query: "tag:clear"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Media) != 0 || len(results.Folders) != 0 || len(results.Blogs) != 0 {
		t.Fatalf("cleared tag results = media %#v folders %#v blogs %#v", results.Media, results.Folders, results.Blogs)
	}
}

func TestLibraryStatisticsCountsPhotoTagAssignments(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Statistik\n\nBlogtext"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.List(context.Background(), ListOptions{Path: "album"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", []string{"Reise", "Sommer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"Familie"}); err != nil {
		t.Fatal(err)
	}
	post, err := lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	post.Tags = []string{"Journal"}
	lib.saveBlog(post)
	if _, err := lib.SaveTag(context.Background(), "Leer"); err != nil {
		t.Fatal(err)
	}

	stats, err := lib.Statistics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.MediaTagAssignments != 2 || stats.FolderTagAssignments != 1 || stats.BlogTagAssignments != 1 || stats.PhotoTagCount != 5 {
		t.Fatalf("tag statistics = media %d folder %d blog %d tags %d", stats.MediaTagAssignments, stats.FolderTagAssignments, stats.BlogTagAssignments, stats.PhotoTagCount)
	}
}

func TestLibraryListTagsForNonAdminHidesAdminOnlyOnlyTags(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "public.jpg"), color.RGBA{G: 200, A: 255})
	writeJPEG(t, filepath.Join(root, "secret", "private.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "public.md"), []byte("# Public\n\nBlog"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "private.md"), []byte("# Secret\n\nBlog"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.SetMediaTagsContext(context.Background(), "public.jpg", []string{"Public"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTagsContext(context.Background(), "secret/private.jpg", []string{"Secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTagsContext(context.Background(), "secret", []string{"SecretFolder"}); err != nil {
		t.Fatal(err)
	}
	publicPost, err := lib.blogFromPath("public.md")
	if err != nil {
		t.Fatal(err)
	}
	publicPost.Tags = []string{"PublicBlog"}
	lib.saveBlog(publicPost)
	secretPost, err := lib.blogFromPath("secret/private.md")
	if err != nil {
		t.Fatal(err)
	}
	secretPost.Tags = []string{"SecretBlog"}
	lib.saveBlog(secretPost)
	if _, err := lib.SaveTag(context.Background(), "Leer"); err != nil {
		t.Fatal(err)
	}

	tags, err := lib.ListTags(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	visible := map[string]int{}
	for _, tag := range tags {
		visible[tag.Name] = tag.Count
	}
	if _, ok := visible["secret"]; ok {
		t.Fatalf("non-admin tags include admin-only-only tag: %#v", tags)
	}
	if _, ok := visible["secretfolder"]; ok {
		t.Fatalf("non-admin tags include admin-only folder tag: %#v", tags)
	}
	if _, ok := visible["secretblog"]; ok {
		t.Fatalf("non-admin tags include admin-only blog tag: %#v", tags)
	}
	if visible["public"] != 1 || visible["publicblog"] != 1 || visible["leer"] != 0 {
		t.Fatalf("non-admin tags = %#v", tags)
	}

	tags, err = lib.ListTags(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	admin := map[string]int{}
	for _, tag := range tags {
		admin[tag.Name] = tag.Count
	}
	if admin["secret"] != 1 || admin["secretfolder"] != 1 || admin["secretblog"] != 1 || admin["public"] != 1 || admin["publicblog"] != 1 || admin["leer"] != 0 {
		t.Fatalf("admin tags = %#v", tags)
	}
}

func TestLibraryIndexesMarkdownBlogsForSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte("# Reisebericht\n\n<!-- @pg-date 2024-06-01 -->\n\nSommer am See"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{B: 200, A: 255})
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	listing, err := lib.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Blogs) != 1 || listing.Blogs[0].Text == "" {
		t.Fatalf("blogs = %#v", listing.Blogs)
	}
	results, err := lib.List(context.Background(), ListOptions{Query: "Sommer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Blogs) != 1 || results.Blogs[0].Name != "index.md" {
		t.Fatalf("blog search = %#v", results.Blogs)
	}
}

func newTestLibrary(t *testing.T, root string) *Library {
	t.Helper()
	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

type listingDriftSummary struct {
	Order   string
	Total   int
	Folders string
	Media   string
	Blogs   string
}

func listingDriftSummaryFor(t *testing.T, root string, rebuild bool, opts ListOptions) listingDriftSummary {
	t.Helper()
	lib := newTestLibrary(t, root)
	defer lib.Close()
	prepareListingDriftMetadata(t, lib)
	if rebuild {
		if _, err := lib.RebuildIndex(context.Background()); err != nil {
			t.Fatal(err)
		}
	} else {
		opts.FullFilesystem = true
	}
	listing, err := lib.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return summarizeListingDrift(listing)
}

func prepareListingDriftMetadata(t *testing.T, lib *Library) {
	t.Helper()
	if _, err := lib.SetFolderTagsContext(context.Background(), "album", []string{"Familie"}); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	post, err := lib.blogFromPath("album/story.md")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Fatal(err)
	}
	post.Tags = []string{"Journal"}
	lib.saveBlog(post)
}

func summarizeListingDrift(listing Listing) listingDriftSummary {
	folders := make([]string, 0, len(listing.Folders))
	for _, folder := range listing.Folders {
		folders = append(folders, fmt.Sprintf("%s|count=%d|dirs=%d|admin=%t|tags=%s", folder.Path, folder.MediaCount, folder.DirCount, folder.AdminOnly, driftTags(folder.Tags)))
	}
	media := make([]string, 0, len(listing.Media))
	for _, item := range listing.Media {
		media = append(media, fmt.Sprintf("%s|type=%s|admin=%t|tags=%s|captured=%s", item.Path, item.Type, item.AdminOnly, driftTags(item.Tags), driftDate(item.CapturedAt)))
	}
	blogs := make([]string, 0, len(listing.Blogs))
	for _, post := range listing.Blogs {
		blogs = append(blogs, fmt.Sprintf("%s|admin=%t|tags=%s|date=%s|text=%q", post.Path, post.AdminOnly, driftTags(post.Tags), driftDate(post.Date), post.Text))
	}
	return listingDriftSummary{
		Order:   listing.Order,
		Total:   listing.Total,
		Folders: strings.Join(folders, "\n"),
		Media:   strings.Join(media, "\n"),
		Blogs:   strings.Join(blogs, "\n"),
	}
}

func driftTags(tags []string) string {
	tags = append([]string(nil), tags...)
	slices.Sort(tags)
	return strings.Join(tags, ",")
}

func driftDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func mediaTestPaths(items []Media) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func folderTestPaths(items []Folder) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func assertFolderListingPaths(t *testing.T, lib *Library, label string, opts ListOptions, want []string) {
	t.Helper()
	listing, err := lib.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	got := folderTestPaths(listing.Folders)
	if !slices.Equal(got, want) {
		t.Fatalf("%s folders = %#v, want %#v", label, got, want)
	}
}

func writePhotoFolder(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(dir, "photo.jpg"), color.RGBA{R: 180, G: 120, B: 80, A: 255})
}

func assertPhotoSearchContains(t *testing.T, lib *Library, query, path string) {
	t.Helper()
	listing, err := lib.List(context.Background(), ListOptions{Query: query, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(mediaTestPaths(listing.Media), path) {
		t.Fatalf("query %q media = %#v, want %q", query, listing.Media, path)
	}
}

func forceFolderScanStale(t *testing.T, lib *Library, path string) {
	t.Helper()
	if _, err := lib.index.db.Exec(`UPDATE photo_folder_scan SET mod_time_unix_nano = 0 WHERE path = ?`, path); err != nil {
		t.Fatal(err)
	}
}

func countPhotoTestThumbnailFiles(t *testing.T, cacheDir string) int {
	t.Helper()
	count := 0
	root := filepath.Join(cacheDir, "thumbnails")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		count++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return count
}

func writeJPEG(t *testing.T, path string, c color.Color) {
	writeSizedJPEG(t, path, 16, 12, c)
}

func writeSizedJPEG(t *testing.T, path string, width, height int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatal(err)
	}
}

func writeXMPRating(t *testing.T, path string, rating float64) {
	t.Helper()
	data := fmt.Sprintf(`<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="%g"/>
</rdf:RDF>
</x:xmpmeta>`, rating)
	if err := os.WriteFile(path+".xmp", []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeXMPFace(t *testing.T, path, name string, x, y, w, h float64) {
	t.Helper()
	data := fmt.Sprintf(`<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/" xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#">
<mwg-rs:Regions rdf:parseType="Resource">
<mwg-rs:RegionList><rdf:Bag>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area rdf:parseType="Resource">
<stArea:h>%g</stArea:h><stArea:unit>normalized</stArea:unit><stArea:w>%g</stArea:w><stArea:x>%g</stArea:x><stArea:y>%g</stArea:y>
</mwg-rs:Area>
<mwg-rs:Name>%s</mwg-rs:Name><mwg-rs:Type>Face</mwg-rs:Type>
</rdf:li>
</rdf:Bag></mwg-rs:RegionList>
</mwg-rs:Regions>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>`, h, w, x, y, name)
	if err := os.WriteFile(path+".xmp", []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFace(t *testing.T, got Face, name string, x, y, w, h float64) {
	t.Helper()
	if got.Name != name || !almostEqual(got.X, x) || !almostEqual(got.Y, y) || !almostEqual(got.Width, w) || !almostEqual(got.Height, h) {
		t.Fatalf("face = %#v, want %q %.4f %.4f %.4f %.4f", got, name, x, y, w, h)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0000001
}

func photoColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func assertFolderPreviewPaths(t *testing.T, lib *Library, label string, opts ListOptions, want []string) {
	t.Helper()
	listing, err := lib.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Folders) != 1 || listing.Folders[0].Path != "album" {
		t.Fatalf("%s folders = %#v", label, listing.Folders)
	}
	got := make([]string, 0, len(listing.Folders[0].Previews))
	for _, preview := range listing.Folders[0].Previews {
		got = append(got, preview.Path)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s previews = %#v, want %#v", label, got, want)
	}
}

func listTraceStepFields(snapshot ListTraceSnapshot, name string) (map[string]string, bool) {
	for _, step := range snapshot.Steps {
		if step.Name != name {
			continue
		}
		fields := make(map[string]string, len(step.Fields))
		for _, field := range step.Fields {
			fields[field.Key] = field.Value
		}
		return fields, true
	}
	return nil, false
}

func installFakeWebPThumbnailer(t *testing.T, width, height int) {
	t.Helper()
	tools := t.TempDir()
	frame := filepath.Join(tools, "frame.jpg")
	writeSizedJPEG(t, frame, width, height, color.RGBA{R: 40, G: 120, B: 200, A: 255})
	vips := filepath.Join(tools, "vipsthumbnail")
	if err := os.WriteFile(vips, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ffmpeg := filepath.Join(tools, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\ncp \"$FAKE_FFMPEG_FRAME\" \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_FFMPEG_FRAME", frame)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func float64Ptr(value float64) *float64 {
	return &value
}
