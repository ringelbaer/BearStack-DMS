package photos

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"os"

	"bearstack/internal/facerec"
	"bearstack/internal/searchtext"
)

type FaceSourceReview struct {
	Face           RecognizedFace `json:"face"`
	Revision       string         `json:"revision"`
	PersonRevision int64          `json:"-"`
}

func (l *Library) FaceSourceReview(ctx context.Context, id int64) (FaceSourceReview, error) {
	f, err := l.Face(ctx, id)
	if err != nil {
		return FaceSourceReview{}, err
	}
	abs, err := l.Resolve(f.Path)
	if err != nil {
		return FaceSourceReview{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return FaceSourceReview{}, err
	}
	var personRevision int64
	if err = l.index.db.QueryRowContext(ctx, `SELECT revision FROM photo_person_revisions WHERE person_id=?`, f.PersonID).Scan(&personRevision); err != nil {
		return FaceSourceReview{}, err
	}
	revision := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d:%s:%s:%s:%t:%t:%t:%g:%g:%g:%g", f.ID, f.PersonID, f.SourceRevision, personRevision, f.Name, f.Path, identityStat(info), f.Manual, f.Ignored, f.Favorite, f.X, f.Y, f.Width, f.Height))))
	return FaceSourceReview{Face: f, Revision: revision, PersonRevision: personRevision}, nil
}

func (l *Library) PendingFaceReviews(ctx context.Context, after int64) ([]RecognizedFace, error) {
	if err := l.RefreshFaceVisibility(ctx); err != nil {
		return nil, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.needs_review=1 AND m.admin_only=0 AND f.id>? ORDER BY f.id LIMIT 100`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecognizedFace{}
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Confirmation never deletes/recreates faces. Missing/ambiguous detections keep
// the old vector as historical data, but embedding_current prevents even a
// favorite from becoming a reference to the changed source.
func (l *Library) ConfirmFaceSource(ctx context.Context, id int64, revision string, box FaceRegion, name string, result *facerec.Result) error {
	for _, n := range []float64{box.X, box.Y, box.Width, box.Height} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1 {
			return ErrLabelInvalid
		}
	}
	if box.Width < .002 || box.Height < .002 || box.X+box.Width > 1 || box.Y+box.Height > 1 {
		return ErrLabelInvalid
	}
	name, err := normalizedPersonName(name)
	if err != nil {
		return err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	current, err := l.FaceSourceReview(ctx, id)
	if err != nil {
		return err
	}
	if current.Revision != revision || !current.Face.NeedsReview {
		return ErrLabelConflict
	}
	abs, err := l.Resolve(current.Face.Path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	hash, err := hashPhotoFile(ctx, abs, info)
	if err != nil {
		return err
	}
	var detection *facerec.Detection
	if result != nil {
		if result.Model != facerec.Model || len(result.Faces) > facerec.MaxFaces {
			return ErrLabelInvalid
		}
		for i := range result.Faces {
			d := &result.Faces[i]
			if err = facerec.Validate(d); err != nil {
				return err
			}
			if overlap(Face{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height}, Face{X: d.X, Y: d.Y, Width: d.Width, Height: d.Height}) >= .7 {
				if detection != nil {
					detection = nil
					break
				}
				detection = d
			}
		}
	}
	check, err := l.FaceSourceReview(ctx, id)
	if err != nil {
		return err
	}
	if check.Revision != revision {
		return ErrLabelConflict
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path JOIN photo_person_revisions r ON r.person_id=f.person_id WHERE f.id=? AND f.person_id=? AND f.source_revision=? AND r.revision=? AND f.needs_review=1 AND m.admin_only=0 AND m.size_bytes=? AND m.mod_time_unix_nano=?)`, id, current.Face.PersonID, current.Face.SourceRevision, current.PersonRevision, info.Size(), info.ModTime().UnixNano()).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrLabelConflict
	}
	person := current.Face.PersonID
	if name != current.Face.Name {
		var count int
		if name != "" {
			if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(min(id),0) FROM photo_people WHERE name_fold=?`, searchtext.GermanFold(name)).Scan(&count, &person); err != nil {
				return err
			}
			if count > 1 {
				return ErrLabelNameExists
			}
		}
		if count == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
			if err != nil {
				return err
			}
			person, err = res.LastInsertId()
			if err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_thumbnail_cache SET expires_at=unixepoch() WHERE face_id=?`, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET x=?,y=?,width=?,height=?,person_id=?,manual=1,needs_review=0,embedding_current=0,source_path='#face/'||id||'/'||(source_revision+1),source_revision=source_revision+1,source_hash=?,source_size=?,source_mtime=? WHERE id=?`, box.X, box.Y, box.Width, box.Height, person, hash, info.Size(), info.ModTime().UnixNano(), id)
	if err != nil {
		return err
	}
	if detection != nil {
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET embedding=?,model=?,confidence=?,embedding_current=1,reference_eligible=? WHERE id=?`, encodeVector(detection.Embedding), result.Model, detection.Confidence, detection.Quality == nil || detection.Quality.ReferenceEligible, id)
		if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM photo_face_jobs WHERE path=?`, current.Face.Path); err != nil {
		return err
	}
	if _, err = refreshFaceMutationTx(ctx, tx, map[int64]bool{person: true, current.Face.PersonID: true}); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		l.faceRuntime.graph = nil
		l.faceSuggestions.clear()
	}
	return err
}
