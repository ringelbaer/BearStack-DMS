package photos

import (
	"context"
	"database/sql"
	"errors"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

const MaxLabelBatchFaces = 500

func isLabelFaceBatch(action string) bool {
	return action == "unassign_faces" || action == "assign_faces" || action == "name_faces" || action == "ignore_faces"
}

func validateLabelFaceBatch(source int64, a LabelAction, name string) error {
	if !isLabelFaceBatch(a.Action) {
		if len(a.FaceIDs) != 0 {
			return ErrLabelInvalid
		}
		return nil
	}
	if len(a.FaceIDs) == 0 || len(a.FaceIDs) > MaxLabelBatchFaces || a.FaceID != 0 || a.Favorite != nil || a.AssignID != 0 || a.AssignRevision != 0 || a.SuggestionID != 0 {
		return ErrLabelInvalid
	}
	seen := make(map[int64]bool, len(a.FaceIDs))
	for _, id := range a.FaceIDs {
		if id <= 0 || seen[id] {
			return ErrLabelInvalid
		}
		seen[id] = true
	}
	if a.Action == "assign_faces" {
		if a.TargetID <= 0 || a.TargetID == source || a.TargetRevision <= 0 || name != "" {
			return ErrLabelInvalid
		}
	} else if a.TargetID != 0 || a.TargetRevision != 0 {
		return ErrLabelInvalid
	}
	if (a.Action == "name_faces") != (name != "") {
		return ErrLabelInvalid
	}
	return nil
}

// One bounded ID lookup validates the complete selection before any writes.
// The caller owns the writer transaction, revision checks and durable receipt.
func labelFaceBatchTx(ctx context.Context, tx *sql.Tx, a LabelAction, name string, out LabelReceipt) (LabelReceipt, error) {
	ids := make([]any, len(a.FaceIDs))
	for i, id := range a.FaceIDs {
		ids[i] = id
	}
	selection := `id IN (` + sqlutil.Placeholders(len(ids)) + `)`
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces WHERE `+selection+` AND person_id=? AND ignored=0`, append(append([]any{}, ids...), out.SourceID)...).Scan(&count); err != nil {
		return out, err
	}
	if count != len(ids) {
		return out, ErrLabelConflict
	}
	out.Faces, out.Groups = int64(count), 0
	if a.Action == "ignore_faces" {
		_, err := tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE `+selection, ids...)
		return out, err
	}
	target := a.TargetID
	if a.Action == "assign_faces" {
		p, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, target))
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrLabelConflict
		}
		if err != nil {
			return out, err
		}
		if p.Name == "" || p.Revision != a.TargetRevision {
			return out, ErrLabelConflict
		}
		out.TargetID = target
	} else {
		if name != "" && !a.AllowDuplicate {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.name=? AND `+labelExists+`)`, name).Scan(&exists); err != nil {
				return out, err
			}
			if exists {
				return out, ErrLabelNameExists
			}
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
		if err != nil {
			return out, err
		}
		target, err = result.LastInsertId()
		if err != nil {
			return out, err
		}
		if a.Action == "unassign_faces" {
			out.NewID = target
		} else {
			out.TargetID = target
		}
	}
	favorite := ""
	if a.Action == "unassign_faces" {
		favorite = ",favorite=0"
	}
	_, err := tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1`+favorite+` WHERE `+selection, append([]any{target}, ids...)...)
	return out, err
}
