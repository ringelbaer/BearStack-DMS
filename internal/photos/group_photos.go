package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	"bearstack/internal/facerec"
)

const DefaultGroupPhotoMinimum = 5
const MaxGroupPhotoMinimum = facerec.MaxFaces - 1

var ErrGroupPhotoChanged = errors.New("Das Gruppenbild wurde inzwischen geändert. Bitte die Ansicht aktualisieren und erneut prüfen")

type GroupPhoto struct {
	Path        string           `json:"path"`
	DisplayPath string           `json:"display_path"`
	Revision    string           `json:"revision"`
	Remaining   int              `json:"remaining"`
	ImageFaceID int64            `json:"image_face_id"`
	Faces       []RecognizedFace `json:"faces"`
}

type GroupPhotosPage struct {
	Minimum int         `json:"minimum"`
	Photo   *GroupPhoto `json:"photo"`
}

// The partial covering index keeps candidate scans away from embedding blobs.
// Existing path/id indexes remain useful for reading all detections of a photo.
func setupGroupPhotos(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_face_group_candidates ON photo_faces(path,person_id) WHERE ignored=0`)
	return err
}

func validGroupPhotoPath(path string, allowEmpty bool) bool {
	clean, err := CleanPath(path)
	return err == nil && clean == path && len(path) <= 4096 && (allowEmpty || path != "")
}

// NextGroupPhoto uses a path cursor, not offsets or a collection-wide filesystem
// scan. Skipping simply advances this cursor; a fresh pass starts at the beginning.
func (l *Library) NextGroupPhoto(ctx context.Context, after string, minimum int) (*GroupPhoto, error) {
	if !validGroupPhotoPath(after, true) || minimum < 0 || minimum > MaxGroupPhotoMinimum {
		return nil, ErrLabelInvalid
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var path string
		// Imported names are potential candidates until their source visibility has
		// been checked. Only the returned photo needs actual filesystem checks.
		err := l.index.db.QueryRowContext(ctx, `SELECT f.path
 FROM photo_faces f INDEXED BY idx_face_group_candidates
 CROSS JOIN photo_people p ON p.id=f.person_id
 CROSS JOIN media_index m ON m.path=f.path
 WHERE f.path>? AND f.ignored=0 AND (p.name='' OR p.name_source<>'')
 AND m.admin_only=0 AND m.type='image'
 GROUP BY f.path HAVING count(*)>? ORDER BY f.path LIMIT 1`, after, minimum).Scan(&path)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		after = path
		photo, err := l.GroupPhoto(ctx, path)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrAdminOnly()) || errors.Is(err, ErrPathEscapesRoot()) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if photo.Remaining > minimum {
			return &photo, nil
		}
	}
}

func (l *Library) refreshGroupPhoto(ctx context.Context, path string) error {
	if !validGroupPhotoPath(path, false) {
		return ErrLabelInvalid
	}
	if err := l.refreshFaceDirectories(ctx, `SELECT directory FROM media_index WHERE path=?
 UNION SELECT m.directory FROM photo_faces f JOIN photo_people p ON p.id=f.person_id
 JOIN media_index m ON m.path=p.name_source WHERE f.path=? AND p.manual_name=0`, path, path); err != nil {
		return err
	}
	return l.refreshFaceSource(ctx, path)
}

// GroupPhoto deliberately has no minimum check: editing must not evict the
// current photo as soon as its remaining count falls below the entry threshold.
func (l *Library) GroupPhoto(ctx context.Context, path string) (GroupPhoto, error) {
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return GroupPhoto{}, err
	}
	return readGroupPhoto(ctx, l.index.db, path)
}

func readGroupPhoto(ctx context.Context, query faceRowsQuery, path string) (GroupPhoto, error) {
	out := GroupPhoto{Path: path, DisplayPath: mediaDisplayPath(path), Faces: []RecognizedFace{}}
	rows, err := query.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f
 JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path
 WHERE f.path=? AND m.admin_only=0 ORDER BY f.id LIMIT ?`, path, facerec.MaxFaces+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		face, err := scanFace(rows)
		if err != nil {
			return out, err
		}
		out.Faces = append(out.Faces, face)
		if !face.Ignored {
			if out.ImageFaceID == 0 {
				out.ImageFaceID = face.ID
			}
			if face.Name == "" {
				out.Remaining++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(out.Faces) == 0 {
		return out, sql.ErrNoRows
	}
	if len(out.Faces) > facerec.MaxFaces {
		return out, errors.New("zu viele Gesichtsdaten für ein Gruppenbild")
	}
	if out.ImageFaceID == 0 {
		out.ImageFaceID = out.Faces[0].ID
	}
	encoded, err := json.Marshal(out.Faces)
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(encoded)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}

// IgnoreGroupPhoto ignores only this photo's remaining unnamed detections, never
// their entire groups. Compare the displayed snapshot inside the write transaction
// so concurrent naming, reassignment or reanalysis cannot change the selection.
func (l *Library) IgnoreGroupPhoto(ctx context.Context, path, revision string) (int, error) {
	return l.ignoreGroupPhoto(ctx, path, revision, 0)
}

// IgnoreGroupPhotoFace limits the same snapshot-checked operation to one unnamed
// detection. Other detections of this person, including in this photo, stay active.
func (l *Library) IgnoreGroupPhotoFace(ctx context.Context, path, revision string, faceID int64) (int, error) {
	if faceID <= 0 {
		return 0, ErrLabelInvalid
	}
	return l.ignoreGroupPhoto(ctx, path, revision, faceID)
}

func (l *Library) ignoreGroupPhoto(ctx context.Context, path, revision string, faceID int64) (int, error) {
	if len(revision) != sha256.Size*2 {
		return 0, ErrLabelInvalid
	}
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return 0, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return 0, err
	}
	photo, err := readGroupPhoto(ctx, tx, path)
	if err != nil {
		return 0, err
	}
	if photo.Revision != revision {
		return 0, ErrGroupPhotoChanged
	}
	var baseRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
		return 0, err
	}
	affected := map[int64]bool{}
	count := 0
	found := faceID == 0
	for _, face := range photo.Faces {
		if faceID != 0 && face.ID != faceID {
			continue
		}
		found = true
		if !face.Ignored && face.Name == "" {
			affected[face.PersonID] = true
			count++
		}
	}
	if !found {
		return 0, ErrLabelInvalid
	}
	if faceID != 0 && count == 0 {
		return 0, ErrGroupPhotoChanged
	}
	selection := ""
	args := []any{path}
	if faceID != 0 {
		selection = " AND id=?"
		args = append(args, faceID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE path=? AND ignored=0
 AND person_id IN (SELECT id FROM photo_people WHERE name='')`+selection, args...); err != nil {
		return 0, err
	}
	committedRevision, err := refreshFaceMutationTx(ctx, tx, affected)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	l.syncFaceMutation(ctx, affected, baseRevision, committedRevision)
	return count, nil
}
