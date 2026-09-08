package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFaceSourceRejectsFailedFingerprintUpdate(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "one.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "one.jpg")
	if err != nil {
		t.Fatal(err)
	}
	face := faces[0]
	if _, err := l.FaceThumbnail(ctx, face.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_fingerprint BEFORE UPDATE ON media_index BEGIN SELECT RAISE(ABORT,'fingerprint write failed'); END`); err != nil {
		t.Fatal(err)
	}
	// An unchanged source needs no write, even if the index cannot be updated.
	if _, err := l.Face(ctx, face.ID); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(l.Root(), "one.jpg"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte("replacement"))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { _, err := l.Face(ctx, face.ID); return err },
		func() error { _, err := l.FaceThumbnail(ctx, face.ID); return err },
		func() error { _, err := l.GroupPhoto(ctx, face.Path); return err },
		func() error { _, err := l.SetFaceFavorite(ctx, face.ID, face.PersonID, true); return err },
	} {
		if err := check(); err == nil || !strings.Contains(err.Error(), "fingerprint write failed") {
			t.Fatalf("stale source accepted: %v", err)
		}
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_fingerprint`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Face(ctx, face.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale face survived successful retry: %v", err)
	}
}
