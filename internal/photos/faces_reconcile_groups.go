package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"slices"
)

// A whole-group decision uses the best reference pair per target, first among
// named people, then among unnamed groups. Never turn an automatic merge into a
// manual confirmation. Caller owns the runtime lock and write transaction.
func (l *Library) reconcileFaceGroup(ctx context.Context, tx *sql.Tx, source, face int64, model string, named map[int64]bool) (int64, int64, error) {
	var first int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(min(id),0) FROM photo_faces WHERE person_id=? AND ignored=0`, source).Scan(&first); err != nil {
		return 0, 0, err
	}
	// Face IDs are the resumable queue cursor. Visit a group only at its first
	// active face, including when its faces span many batches.
	if first != face {
		return 0, 0, nil
	}
	excluded := map[int64]bool{source: true}
	// Co-occurrence anywhere in the group is a veto, not only in the witness
	// photo. Rejected identities also remain excluded after earlier merges.
	rows, err := tx.QueryContext(ctx, `SELECT b.person_id FROM
 (SELECT DISTINCT path FROM photo_faces WHERE person_id=? AND ignored=0) a
 JOIN photo_faces b ON b.path=a.path AND b.ignored=0
 UNION SELECT target_id FROM photo_face_merge_suggestions WHERE source_id=? AND rejected=1
 UNION SELECT source_id FROM photo_face_merge_suggestions WHERE target_id=? AND rejected=1`, source, source, source)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		excluded[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	thresholds := l.matchingThresholds()
	for _, scope := range []facePersonScope{facePersonsConfirmedNamed, facePersonsUnconfirmedUnnamed} {
		if scope == facePersonsConfirmedNamed && len(named) == 0 {
			continue
		}
		best := map[int64]facePersonCandidate{}
		for ref := range l.faceRuntime.references(source) {
			ranked, err := l.faceRuntime.rankFacePersonsInScope(ctx, ref.vector, excluded, named, scope)
			if err != nil {
				return 0, 0, err
			}
			candidates, err := l.validateFaceCandidatesInScope(ctx, tx, &l.faceRuntime, ref.vector, ranked, 2, scope)
			if err != nil {
				return 0, 0, err
			}
			// The global top two must occur in at least one reference's top two.
			for _, candidate := range candidates {
				if old, ok := best[candidate.person]; !ok || compareFaceCandidates(candidate, old) < 0 {
					best[candidate.person] = candidate
				}
			}
		}
		candidates := make([]facePersonCandidate, 0, len(best))
		for _, candidate := range best {
			candidates = append(candidates, candidate)
		}
		slices.SortFunc(candidates, compareFaceCandidates)
		if len(candidates) == 0 || candidates[0].score < thresholds.ReconcileSimilarity || !reviewCandidateAllowed(candidates, 0, thresholds.ReconcileMargin) {
			continue
		}
		target := candidates[0].person
		eligible, err := l.reconcileGroupEligible(ctx, tx, source, model)
		if err != nil || !eligible {
			return 0, 0, err
		}
		moved, err := mergeAutomaticFaceGroupTx(ctx, tx, source, target)
		return target, moved, err
	}
	return 0, 0, nil
}

// Stream metadata using the person index; never load every face embedding or
// decode originals. A protected member prevents moving the entire source group.
func (l *Library) reconcileGroupEligible(ctx context.Context, tx *sql.Tx, source int64, model string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT f.path,f.x,f.y,f.width,f.height,m.faces,m.size_bytes,m.mod_time_unix_nano,m.xmp_fingerprint,
 f.manual=0 AND f.favorite=0 AND f.ignored=0 AND f.drawn=0 AND f.needs_review=0 AND f.embedding_current=1 AND coalesce(f.reference_eligible,1)=1
 AND f.model=? AND p.name='' AND p.manual_name=0 AND m.admin_only=0
 FROM photo_faces f JOIN photo_people p ON p.id=f.person_id LEFT JOIN media_index m ON m.path=f.path WHERE f.person_id=?
 ORDER BY f.ignored,f.path,f.id`, model, source)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	visibility := newFaceDirectoryVisibility(l.root)
	found := false
	var checkedPath string
	for rows.Next() {
		var path string
		var region Face
		var annotations sql.NullString
		var eligible sql.NullBool
		var size, mtime sql.NullInt64
		var xmp sql.NullString
		if err := rows.Scan(&path, &region.X, &region.Y, &region.Width, &region.Height, &annotations, &size, &mtime, &xmp, &eligible); err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !eligible.Valid || !eligible.Bool || visibility.private(parentPath(path)) {
			return false, nil
		}
		if path != checkedPath {
			abs, err := l.Resolve(path)
			if err != nil {
				return false, nil
			}
			info, err := os.Stat(abs)
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if info.Size() != size.Int64 || info.ModTime().UnixNano() != mtime.Int64 || xmpSidecarFingerprint(abs) != xmp.String {
				return false, nil
			}
			checkedPath = path
		}
		var imported []Face
		if err := json.Unmarshal([]byte(annotations.String), &imported); err != nil {
			return false, err
		}
		for _, annotation := range imported {
			if name, err := normalizedPersonName(annotation.Name); err == nil && name != "" && overlap(region, annotation) >= .5 {
				return false, nil
			}
		}
		found = true
	}
	return found, rows.Err()
}

func mergeAutomaticFaceGroupTx(ctx context.Context, tx *sql.Tx, source, target int64) (int64, error) {
	// Family conflicts leave the pair for manual review without aborting the
	// maintenance queue or leaving partially transferred relationships behind.
	if _, err := tx.ExecContext(ctx, `SAVEPOINT automatic_face_group`); err != nil {
		return 0, err
	}
	if err := mergePersonFamilyTx(ctx, tx, source, target); err != nil {
		if !errors.Is(err, ErrParentMerge) && !errors.Is(err, ErrPersonDetailsMerge) {
			return 0, err
		}
		_, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO automatic_face_group; RELEASE automatic_face_group`)
		return 0, rollbackErr
	}
	if err := mergePersonTagsTx(ctx, tx, source, target); err != nil {
		return 0, err
	}
	// A rejection is an identity decision. Transfer both directions before the
	// source deletion trigger removes its old suggestions; existing open pairs
	// become rejected too. Revision/witness fields are unused for rejected pairs.
	if _, err := tx.ExecContext(ctx, `INSERT INTO photo_face_merge_suggestions
 (source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model,rejected)
 SELECT CASE WHEN source_id=? THEN ? ELSE source_id END,
 CASE WHEN target_id=? THEN ? ELSE target_id END,source_revision,target_revision,source_face_id,target_face_id,score,model,1
 FROM photo_face_merge_suggestions WHERE rejected=1 AND (source_id=? OR target_id=?)
 ON CONFLICT(source_id,target_id) DO UPDATE SET rejected=1`, source, target, source, target, source, source); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=? WHERE person_id=?`, target, source)
	if err != nil {
		return 0, err
	}
	moved, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_people WHERE id=?`, source); err != nil {
		return 0, err
	}
	// The surviving group may precede the cursor. A further bounded pass lets
	// it meet other groups; every successful merge reduces the number of people.
	_, err = tx.ExecContext(ctx, `UPDATE photo_face_reconciliation SET generation=generation+1 WHERE id=1; RELEASE automatic_face_group`)
	return moved, err
}
