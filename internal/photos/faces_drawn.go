package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"

	"bearstack/internal/facerec"
	"bearstack/internal/searchtext"
)

func setupFaceDrawnSchema(ctx context.Context, db *sql.DB) error {
	if _, err := ensurePhotoColumn(ctx, db, "photo_faces", "drawn", `ALTER TABLE photo_faces ADD COLUMN drawn INTEGER NOT NULL DEFAULT 0 CHECK(drawn IN (0,1))`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_face_drawn_active ON photo_faces(id) WHERE drawn=1 AND ignored=0`)
	return err
}

func faceSourceRevision(m Media) string {
	b, _ := json.Marshal([]any{m.Path, m.SizeBytes, m.ModTime.UnixNano(), m.XMPFingerprint})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// FaceDrawingImage uses the exact orientation and dimensions used by face crops.
// The revision binds the submitted coordinates to this source, not a later file.
func (l *Library) FaceDrawingImage(ctx context.Context, path string) ([]byte, string, error) {
	if !validGroupPhotoPath(path, false) || !CanThumbnail(path) {
		return nil, "", ErrLabelInvalid
	}
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return nil, "", err
	}
	m, err := l.MediaContext(ctx, path)
	if err != nil {
		return nil, "", err
	}
	b, err := l.FaceImage(ctx, path)
	if err != nil {
		return nil, "", err
	}
	after, err := l.MediaContext(ctx, path)
	if err != nil {
		return nil, "", err
	}
	if after.AdminOnly || faceSourceRevision(m) != faceSourceRevision(after) {
		return nil, "", ErrLabelConflict
	}
	return b, faceSourceRevision(m), nil
}

// AddDrawnFace stores a deliberate region without fabricating an embedding or
// running inference. Naming/assignment and creation are a single transaction.
func (l *Library) AddDrawnFace(ctx context.Context, path, revision string, box FaceRegion, target int64, name string) (int64, error) {
	if !validGroupPhotoPath(path, false) || !CanThumbnail(path) || len(revision) != 64 || target < 0 {
		return 0, ErrLabelInvalid
	}
	for _, v := range []float64{box.X, box.Y, box.Width, box.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return 0, ErrLabelInvalid
		}
	}
	if box.Width < .002 || box.Height < .002 || box.X+box.Width > 1 || box.Y+box.Height > 1 {
		return 0, ErrLabelInvalid
	}
	if target > 0 {
		name = ""
	}
	name, err := normalizedPersonName(name)
	if err != nil || (target == 0 && name == "") {
		return 0, ErrLabelInvalid
	}
	if err = l.refreshGroupPhoto(ctx, path); err != nil {
		return 0, err
	}
	if target > 0 {
		if err = l.refreshPeopleVisibility(ctx, `p.id=?`, target); err != nil {
			return 0, err
		}
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	m, err := l.MediaContext(ctx, path)
	if err != nil {
		return 0, err
	}
	if m.AdminOnly {
		return 0, ErrAdminOnly()
	}
	if faceSourceRevision(m) != revision {
		return 0, ErrLabelConflict
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Take the write lock before examining duplicates or target membership.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return 0, err
	}
	var sourceOK bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM media_index WHERE path=? AND size_bytes=? AND mod_time_unix_nano=? AND xmp_fingerprint=? AND admin_only=0)`, path, m.SizeBytes, m.ModTime.UnixNano(), m.XMPFingerprint).Scan(&sourceOK); err != nil {
		return 0, err
	}
	if !sourceOK {
		return 0, ErrLabelConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT x,y,width,height FROM photo_faces WHERE path=? LIMIT ?`, path, facerec.MaxFaces)
	if err != nil {
		return 0, err
	}
	count, duplicate := 0, false
	for rows.Next() {
		var existing Face
		if err = rows.Scan(&existing.X, &existing.Y, &existing.Width, &existing.Height); err != nil {
			rows.Close()
			return 0, err
		}
		count++
		duplicate = duplicate || overlap(existing, Face{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height}) >= .7
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if duplicate || count >= facerec.MaxFaces {
		return 0, ErrLabelConflict
	}
	if target == 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
		if err != nil {
			return 0, err
		}
		target, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path JOIN photo_people p ON p.id=f.person_id WHERE p.id=? AND f.ignored=0 AND m.admin_only=0)`, target).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			return 0, ErrLabelConflict
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,drawn,reference_eligible) VALUES(?,?,?,?,?,?,?,0,X'','',1,1,0)`, path, m.Directory, target, box.X, box.Y, box.Width, box.Height)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = refreshFaceMutationTx(ctx, tx, map[int64]bool{target: true}); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	l.faceRuntime.graph = nil
	return id, nil
}
