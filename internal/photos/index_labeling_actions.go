package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"bearstack/internal/searchtext"
)

// labelMutation describes cache work that may run only after a successful commit.
// A replay returns an empty mutation because it does not write the database.
type labelMutation struct {
	affected                        map[int64]bool
	baseRevision, committedRevision int64
	syncReferences, resetGraph      bool
}

// applyLabelAction owns the complete writer reservation, revision checks,
// mutations and durable receipt. The library must check visibility and hold the
// face runtime lock across this call and subsequent cache synchronization.
func (s *photoIndexStore) applyLabelAction(ctx context.Context, actor string, id int64, a LabelAction, name, fingerprint string) (LabelReceipt, labelMutation, error) {
	var out LabelReceipt
	manualMerge := a.Action == "merge_groups" || a.Action == "name_groups"
	batch := isLabelFaceBatch(a.Action)
	merging := a.Action == "accept_merge" || a.Action == "reject_merge" || a.Action == "name_merge"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, labelMutation{}, err
	}
	defer tx.Rollback()
	// Reserve SQLite's writer before reading revisions. Web renames and index
	// invalidation also write this database, outside the face runtime mutex.
	// This avoids a WAL snapshot-upgrade race between revision check and mutation.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_labeling_identity SET id=id WHERE id=1`); err != nil {
		return out, labelMutation{}, err
	}
	var dataset string
	if err = tx.QueryRowContext(ctx, `SELECT dataset FROM photo_labeling_identity WHERE id=1`).Scan(&dataset); err != nil {
		return out, labelMutation{}, err
	}
	if dataset != a.Dataset {
		return out, labelMutation{}, ErrLabelConflict
	}
	var previous, receipt string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,result FROM photo_labeling_actions WHERE actor=? AND operation_id=?`, actor, a.OperationID).Scan(&previous, &receipt)
	if err == nil {
		if previous != fingerprint {
			return out, labelMutation{}, ErrLabelConflict
		}
		err = json.Unmarshal([]byte(receipt), &out)
		return out, labelMutation{}, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, labelMutation{}, err
	}
	source, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, labelMutation{}, ErrLabelConflict
	}
	if err != nil {
		return out, labelMutation{}, err
	}
	managing := a.Action == "rename" || a.Action == "unassign" || a.Action == "favorite" || batch
	if source.Revision != a.Revision || (!merging && !manualMerge && (source.Name != "") != managing) {
		return out, labelMutation{}, ErrLabelConflict
	}
	var baseRevision int64
	if managing {
		if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
			return out, labelMutation{}, err
		}
	}
	out = LabelReceipt{OperationID: a.OperationID, Action: a.Action, SourceID: id, Faces: source.Count, Groups: 1, At: time.Now().Unix()}
	switch a.Action {
	case "unassign_faces", "assign_faces", "name_faces", "ignore_faces":
		out, err = labelFaceBatchTx(ctx, tx, a, name, out)
	case "merge_groups", "name_groups":
		out, err = mergeLabelGroupsTx(ctx, tx, a, name, out)
	case "accept_merge", "reject_merge", "name_merge":
		if err = validateFaceMergeSuggestionTx(ctx, tx, id, a.TargetID, &faceMergeExpectation{a.SuggestionID, a.Revision, a.TargetRevision}); err != nil {
			return out, labelMutation{}, err
		}
		out.TargetID, out.Groups = a.TargetID, 0
		if a.Action == "name_merge" {
			out, err = nameMergeTx(ctx, tx, source, a, name, out)
		} else if a.Action == "reject_merge" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_face_merge_suggestions SET rejected=1 WHERE id=?`, a.SuggestionID)
			out.Faces = 0
		} else {
			err = mergePersonTx(ctx, tx, id, a.TargetID)
		}
	case "name", "rename":
		if !a.AllowDuplicate {
			var exists bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.name=? AND p.id<>? AND `+labelExists+`)`, name, id).Scan(&exists)
			if err != nil {
				return out, labelMutation{}, err
			}
			if exists {
				return out, labelMutation{}, ErrLabelNameExists
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=?`, name, searchtext.GermanFold(name), id)
		if a.Action == "rename" {
			out.Groups = 0
		}
	case "assign":
		target, e := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, a.TargetID))
		if errors.Is(e, sql.ErrNoRows) {
			return out, labelMutation{}, ErrLabelConflict
		}
		if e != nil {
			return out, labelMutation{}, e
		}
		if a.TargetID == id || target.Name == "" || target.Revision != a.TargetRevision {
			return out, labelMutation{}, ErrLabelConflict
		}
		if err = mergePersonTagsTx(ctx, tx, id, a.TargetID); err != nil {
			return out, labelMutation{}, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE person_id=? AND ignored=0`, a.TargetID, id)
		out.TargetID = a.TargetID
	case "ignore":
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE person_id=? AND ignored=0`, id)
	case "detach", "unassign", "favorite":
		if a.Action == "detach" && source.Count < 2 {
			return out, labelMutation{}, ErrLabelConflict
		}
		var belongs bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_faces WHERE id=? AND person_id=? AND ignored=0)`, a.FaceID, id).Scan(&belongs); err != nil {
			return out, labelMutation{}, err
		}
		if !belongs {
			return out, labelMutation{}, ErrLabelConflict
		}
		out.Faces = 1
		out.Groups = 0
		if a.Action == "favorite" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET favorite=? WHERE id=?`, *a.Favorite, a.FaceID)
			break
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES('','',1)`)
		if e != nil {
			return out, labelMutation{}, e
		}
		out.NewID, err = result.LastInsertId()
		if err != nil {
			return out, labelMutation{}, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE id=?`, out.NewID, a.FaceID)
		if err == nil && a.Action == "unassign" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET favorite=0 WHERE id=?`, a.FaceID)
		}
		out.Faces = 1
		out.Groups = 0
	}
	if err != nil {
		return out, labelMutation{}, err
	}
	affected := map[int64]bool{}
	if manualMerge {
		for _, group := range a.Groups {
			affected[group.ID] = true
		}
	}
	for _, pid := range []int64{id, a.TargetID, out.TargetID, out.NewID} {
		if pid > 0 {
			affected[pid] = true
		}
	}
	var committedRevision int64
	if a.Action != "reject_merge" {
		committedRevision, err = refreshFaceMutationTx(ctx, tx, affected)
		if err != nil {
			return out, labelMutation{}, err
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT revision FROM photo_person_revisions WHERE person_id=?),0)`, id).Scan(&out.SourceRevision); err != nil {
		return out, labelMutation{}, err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, labelMutation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO photo_labeling_actions VALUES(?,?,?,?)`, actor, a.OperationID, fingerprint, string(encoded)); err != nil {
		return out, labelMutation{}, err
	}
	if err = tx.Commit(); err != nil {
		return out, labelMutation{}, err
	}
	return out, labelMutation{affected: affected, baseRevision: baseRevision, committedRevision: committedRevision, syncReferences: managing, resetGraph: !managing && a.Action != "reject_merge"}, nil
}
