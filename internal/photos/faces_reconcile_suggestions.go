package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"bearstack/internal/sqlutil"
)

// Score describes the best reference match for the group, not a probability.
type FaceMergeSuggestion struct {
	ID             int64   `json:"id"`
	SourceID       int64   `json:"source_id"`
	TargetID       int64   `json:"target_id"`
	SourceRevision int64   `json:"source_revision"`
	TargetRevision int64   `json:"target_revision"`
	SourceFaceID   int64   `json:"source_face_id"`
	TargetFaceID   int64   `json:"target_face_id"`
	SourceName     string  `json:"source_name"`
	TargetName     string  `json:"target_name"`
	Score          float64 `json:"score"`
}

func cacheFaceMergeSuggestion(ctx context.Context, tx *sql.Tx, item faceSuggestionEvidence, model string) (bool, error) {
	var rejected bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions WHERE rejected=1 AND ((source_id=? AND target_id=?) OR (source_id=? AND target_id=?)))`, item.source, item.target, item.target, item.source).Scan(&rejected); err != nil {
		return false, err
	}
	if rejected {
		return false, nil
	}
	var targetName string
	var targetManual bool
	if err := tx.QueryRowContext(ctx, `SELECT name,manual_name FROM photo_people WHERE id=?`, item.target).Scan(&targetName, &targetManual); err != nil {
		return false, err
	}
	// An explicitly split/unassigned unnamed group is a negative user decision.
	if targetName == "" && targetManual {
		return false, nil
	}
	var targetFace int64
	var sourceRevision, targetRevision int64
	err := tx.QueryRowContext(ctx, `SELECT
 witness.id,sr.revision,tr.revision
 FROM photo_faces original JOIN photo_people sp ON sp.id=original.person_id
 JOIN photo_person_revisions sr ON sr.person_id=sp.id JOIN photo_person_revisions tr ON tr.person_id=?
 JOIN photo_faces witness ON witness.id=? AND witness.person_id=? AND witness.model=? AND witness.ignored=0
 AND (witness.favorite=1 OR coalesce(witness.reference_eligible,1)=1)
 JOIN media_index wm ON wm.path=witness.path AND wm.admin_only=0
 WHERE original.id=? AND original.person_id=? AND original.manual=0 AND original.favorite=0 AND original.ignored=0
 AND sp.name='' AND sp.manual_name=0
 AND NOT EXISTS(SELECT 1 FROM photo_faces a JOIN photo_faces b ON a.path=b.path
 WHERE a.person_id=? AND b.person_id=? AND a.ignored=0 AND b.ignored=0)`, item.target, item.targetFace, item.target, model, item.face, item.source, item.source, item.target).Scan(&targetFace, &sourceRevision, &targetRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	source, target, sourceFace := item.source, item.target, item.face
	// One durable pair for unnamed fragments, regardless of discovery direction.
	if targetName == "" && target < source {
		source, target = target, source
		sourceFace, targetFace = targetFace, sourceFace
		sourceRevision, targetRevision = targetRevision, sourceRevision
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions WHERE source_id=? AND target_id=?)`, source, target).Scan(&exists); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO photo_face_merge_suggestions(source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model)
 VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(source_id,target_id) DO UPDATE SET source_revision=excluded.source_revision,target_revision=excluded.target_revision,
 source_face_id=excluded.source_face_id,target_face_id=excluded.target_face_id,
 score=CASE WHEN source_revision=excluded.source_revision AND target_revision=excluded.target_revision AND model=excluded.model THEN max(score,excluded.score) ELSE excluded.score END,model=excluded.model
 WHERE rejected=0 AND (source_revision<>excluded.source_revision OR target_revision<>excluded.target_revision OR score<excluded.score OR model<>excluded.model)`, source, target, sourceRevision, targetRevision, sourceFace, targetFace, item.score, model)
	return !exists && err == nil, err
}

const faceMergeSuggestionSelect = `SELECT s.id,s.source_id,s.target_id,s.source_revision,s.target_revision,s.source_face_id,s.target_face_id,sp.name,tp.name,s.score
 FROM photo_face_merge_suggestions s JOIN photo_people sp ON sp.id=s.source_id JOIN photo_people tp ON tp.id=s.target_id
 JOIN photo_person_revisions sr ON sr.person_id=s.source_id AND sr.revision=s.source_revision
 JOIN photo_person_revisions tr ON tr.person_id=s.target_id AND tr.revision=s.target_revision
 JOIN photo_faces sf ON sf.id=s.source_face_id AND sf.person_id=s.source_id AND sf.ignored=0
 JOIN photo_faces tf ON tf.id=s.target_face_id AND tf.person_id=s.target_id AND tf.ignored=0
 JOIN media_index sm ON sm.path=sf.path AND sm.admin_only=0 JOIN media_index tm ON tm.path=tf.path AND tm.admin_only=0
 WHERE s.rejected=0 AND s.model=(SELECT model FROM photo_face_state WHERE id=1)`

func scanFaceMergeSuggestion(row interface{ Scan(...any) error }) (FaceMergeSuggestion, error) {
	var s FaceMergeSuggestion
	err := row.Scan(&s.ID, &s.SourceID, &s.TargetID, &s.SourceRevision, &s.TargetRevision, &s.SourceFaceID, &s.TargetFaceID, &s.SourceName, &s.TargetName, &s.Score)
	return s, err
}

// FaceMergeSuggestions only reads cached candidates; page views never run matching.
func (l *Library) FaceMergeSuggestions(ctx context.Context, limit int) ([]FaceMergeSuggestion, error) {
	return l.faceMergeSuggestions(ctx, limit, [2]int64{})
}

func (l *Library) faceMergeSuggestions(ctx context.Context, limit int, excluded [2]int64) ([]FaceMergeSuggestion, error) {
	limit = min(max(limit, 1), 100)
	read := func(ids []any) ([]FaceMergeSuggestion, error) {
		filter := ""
		args := append([]any{}, ids...)
		if len(ids) > 0 {
			filter = ` AND s.id IN (` + sqlutil.Placeholders(len(ids)) + `)`
		}
		if excluded[0] > 0 && excluded[1] > 0 {
			filter += ` AND NOT ((s.source_id=? AND s.target_id=?) OR (s.source_id=? AND s.target_id=?))`
			args = append(args, excluded[0], excluded[1], excluded[1], excluded[0])
		}
		args = append(args, limit)
		rows, err := l.index.db.QueryContext(ctx, faceMergeSuggestionSelect+filter+` ORDER BY s.score DESC,s.id LIMIT ?`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []FaceMergeSuggestion{}
		for rows.Next() {
			s, err := scanFaceMergeSuggestion(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, rows.Err()
	}
	out, err := read(nil)
	if err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]int64, 0, len(out)*2)
	for _, s := range out {
		ids = append(ids, s.SourceID, s.TargetID)
	}
	if err := l.refreshPersonIDsVisibility(ctx, ids...); err != nil {
		return nil, err
	}
	checked := make([]any, 0, len(out))
	for _, s := range out {
		valid := true
		for _, face := range []int64{s.SourceFaceID, s.TargetFaceID} {
			if _, err := l.Face(ctx, face); err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrAdminOnly()) {
					valid = false
					break
				}
				return nil, err
			}
		}
		if valid {
			checked = append(checked, s.ID)
		}
	}
	if len(checked) == 0 {
		return []FaceMergeSuggestion{}, nil
	}
	// Refreshing visibility can remove rows from the first page. Never fill the
	// gaps with candidates whose groups/sources were not checked above.
	return read(checked)
}

func (l *Library) RejectFaceMergeSuggestion(ctx context.Context, id, sourceRevision, targetRevision int64) error {
	if id <= 0 || sourceRevision <= 0 || targetRevision <= 0 {
		return ErrLabelInvalid
	}
	var source, target int64
	if err := l.index.db.QueryRowContext(ctx, `SELECT source_id,target_id FROM photo_face_merge_suggestions WHERE id=? AND rejected=0`, id).Scan(&source, &target); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLabelConflict
		}
		return err
	}
	if err := l.refreshPersonIDsVisibility(ctx, source, target); err != nil {
		return err
	}
	// Keep the pair even after unrelated reference/name changes: rejection is an
	// identity correction and also vetoes future automatic reassociation.
	result, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_merge_suggestions SET rejected=1 WHERE id=? AND source_revision=? AND target_revision=? AND rejected=0
 AND EXISTS(SELECT 1 FROM photo_person_revisions r WHERE r.person_id=source_id AND r.revision=source_revision)
 AND EXISTS(SELECT 1 FROM photo_person_revisions r WHERE r.person_id=target_id AND r.revision=target_revision)`, id, sourceRevision, targetRevision)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrLabelConflict
	}
	return nil
}

type faceMergeExpectation struct {
	suggestion, sourceRevision, targetRevision int64
}

func (l *Library) AcceptFaceMergeSuggestion(ctx context.Context, id, sourceRevision, targetRevision int64) error {
	if id <= 0 || sourceRevision <= 0 || targetRevision <= 0 {
		return ErrLabelInvalid
	}
	var source, target int64
	if err := l.index.db.QueryRowContext(ctx, `SELECT source_id,target_id FROM photo_face_merge_suggestions WHERE id=? AND rejected=0`, id).Scan(&source, &target); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLabelConflict
		}
		return err
	}
	return l.mergePeopleChecked(ctx, source, target, nil, &faceMergeExpectation{id, sourceRevision, targetRevision})
}

// Called inside the same write-reserved transaction as the existing merge.
func validateFaceMergeSuggestionTx(ctx context.Context, tx *sql.Tx, source, target int64, expected *faceMergeExpectation) error {
	if expected == nil {
		return nil
	}
	var valid bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions s
 JOIN photo_person_revisions sr ON sr.person_id=s.source_id AND sr.revision=s.source_revision
 JOIN photo_person_revisions tr ON tr.person_id=s.target_id AND tr.revision=s.target_revision
 WHERE s.id=? AND s.source_id=? AND s.target_id=? AND s.source_revision=? AND s.target_revision=? AND s.rejected=0
 AND s.model=(SELECT model FROM photo_face_state WHERE id=1)
 AND NOT EXISTS(SELECT 1 FROM photo_faces a JOIN photo_faces b ON a.path=b.path
 WHERE a.person_id=s.source_id AND b.person_id=s.target_id AND a.ignored=0 AND b.ignored=0))`, expected.suggestion, source, target, expected.sourceRevision, expected.targetRevision).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrLabelConflict
	}
	return nil
}

func clearFaceReconciliationTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_face_merge_suggestions`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE photo_face_reconciliation SET pending=1,running=0,generation=generation+1,run_generation=0,cursor=0,upper_id=0,processed=0,reassigned=0,suggestions=0 WHERE id=1`)
	return err
}
