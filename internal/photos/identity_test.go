package photos

import (
	"bytes"
	"context"
	"database/sql"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhotoRelocationPreservesIdentityFacesAndCaches(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg")
	finishFace(t, l, 0)
	fs, err := l.AutomaticFaces(ctx, "old/a.jpg")
	if err != nil || len(fs) != 1 {
		t.Fatal(fs, err)
	}
	face := fs[0]
	if err = l.RenamePerson(ctx, face.PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SetFaceFavorite(ctx, face.ID, face.PersonID, true); err != nil {
		t.Fatal(err)
	}
	if err = l.index.setMediaTags(ctx, "old/a.jpg", []string{"family"}); err != nil {
		t.Fatal(err)
	}
	preview, err := l.FaceThumbnail(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := scanEntity(l.index.db.QueryRow(`SELECT ` + entityColumns + ` FROM photo_entities WHERE path='old/a.jpg'`))
	if err != nil {
		t.Fatal(err)
	}
	thumb := l.thumbnailCachePath("old/a.jpg", 420)
	if err = os.MkdirAll(filepath.Dir(thumb), 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(thumb, []byte("thumbnail"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := l.Media("old/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err = l.markThumbnailGenerated(ctx, m, 420); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "new")); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := scanEntity(l.index.db.QueryRow(`SELECT ` + entityColumns + ` FROM photo_entities WHERE path='new/a.jpg'`))
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Fatalf("identity changed: %d -> %d", before.ID, after.ID)
	}
	got, err := l.Face(ctx, face.ID)
	if err != nil || got.Path != "new/a.jpg" || got.Name != "Ada" || !got.Favorite || got.NeedsReview {
		t.Fatalf("face: %+v %v", got, err)
	}
	if b, err := l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(b, preview) {
		t.Fatal("preview changed", err)
	}
	if path, err := l.Thumbnail(ctx, "new/a.jpg", 420); err != nil || path != thumb {
		t.Fatal("thumbnail not reused", path, err)
	}
	if m, err = l.Media("new/a.jpg"); err != nil || len(m.Tags) != 1 || m.Tags[0] != "family" {
		t.Fatal("tags", m.Tags, err)
	}
}

func TestPhotoRetentionRestoresAndExpires(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "album/a.jpg")
	face := fs[0]
	preview, err := l.FaceThumbnail(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	offline := filepath.Join(t.TempDir(), "offline")
	if err = os.Rename(filepath.Join(l.root, "album"), offline); err != nil {
		t.Fatal(err)
	}
	// Keep a healthy, nonempty root to distinguish deletion from an offline mount.
	writeJPEG(t, filepath.Join(l.root, "other.jpg"), color.Black)
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := scanEntity(l.index.db.QueryRow(`SELECT ` + entityColumns + ` FROM photo_entities WHERE path='album/a.jpg'`))
	if err != nil || e.MissingSince == 0 {
		t.Fatal(e, err)
	}
	var count int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_faces`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = os.Rename(offline, filepath.Join(l.root, "album")); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if b, err := l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(b, preview) {
		t.Fatal("retained preview", err)
	}
	if err = os.Remove(filepath.Join(l.root, "album/a.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`UPDATE photo_entities SET missing_since=? WHERE path='album/a.jpg'`, time.Now().Add(-PhotoRetention-time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.index.db.QueryRow(`SELECT id FROM photo_entities WHERE path='album/a.jpg'`).Scan(&count); err != sql.ErrNoRows {
		t.Fatal("not expired", err)
	}
}

func TestChangedPhotoPreservesFaceAndMarksReview(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "album/a.jpg")
	face := fs[0]
	preview, err := l.FaceThumbnail(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(l.root, "album/a.jpg"), color.Black)
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := l.Face(ctx, face.ID)
	if err != nil || !got.NeedsReview || got.PersonID != face.PersonID {
		t.Fatal(got, err)
	}
	if b, err := l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(b, preview) {
		t.Fatal("review snapshot lost", err)
	}
	var count int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references WHERE face_id=?`, face.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale reference", count, err)
	}
}
