package photos

import (
	"context"
	"database/sql"
	"errors"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

const MaxLabelMergeGroups = 60

type LabelGroupRef struct {
	ID       int64 `json:"id"`
	Revision int64 `json:"revision"`
}

// LabelMergeGroups scans the ID index in small batches. Only the returned page
// receives portrait metadata; browsing never loads all faces of a large group.
func (l *Library) LabelMergeGroups(ctx context.Context, after, upper int64, includeNamed bool) (LabelCandidates, error) {
	names := `p.name=''`
	if includeNamed {
		names = `1=1`
	}
	out, err := l.labelPeopleList(ctx, after, upper, names, "", nil)
	if err != nil || len(out.People) == 0 {
		return out, err
	}
	args := make([]any, len(out.People))
	positions := make(map[int64]int, len(out.People))
	for i, p := range out.People {
		args[i] = p.FaceID
		positions[p.ID] = i
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT f.person_id,f.id,f.path,f.x,f.y,f.width,f.height,f.favorite,m.size_bytes,m.mod_time_unix_nano
        FROM photo_faces f JOIN media_index m ON m.path=f.path
        WHERE f.id IN (`+sqlutil.Placeholders(len(args))+`) AND f.ignored=0 AND m.admin_only=0`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var person, size, modified int64
		var source string
		var face LabelFace
		if err := rows.Scan(&person, &face.ID, &source, &face.Bounds.X, &face.Bounds.Y, &face.Bounds.Width, &face.Bounds.Height, &face.Favorite, &size, &modified); err != nil {
			return out, err
		}
		if i, ok := positions[person]; ok && out.People[i].FaceID == face.ID {
			face.DisplayPath = mediaDisplayPath(source)
			face.OriginalKey = labelOriginalKey(source, size, modified)
			out.People[i].Faces = []LabelFace{face}
		}
	}
	return out, rows.Err()
}

func validateLabelGroupSelection(id int64, a LabelAction, name string) error {
	if a.Action != "merge_groups" && a.Action != "name_groups" {
		if len(a.Groups) != 0 {
			return ErrLabelInvalid
		}
		return nil
	}
	if len(a.Groups) < 2 || len(a.Groups) > MaxLabelMergeGroups || a.Groups[0].ID != id || a.Groups[0].Revision != a.Revision ||
		a.TargetID != 0 || a.TargetRevision != 0 || a.AssignID != 0 || a.AssignRevision != 0 || a.SuggestionID != 0 || a.FaceID != 0 || a.Favorite != nil ||
		(a.Action == "name_groups" && name == "") || (a.Action == "merge_groups" && (name != "" || a.AllowDuplicate)) {
		return ErrLabelInvalid
	}
	seen := make(map[int64]bool, len(a.Groups))
	for _, group := range a.Groups {
		if group.ID <= 0 || group.Revision <= 0 || seen[group.ID] {
			return ErrLabelInvalid
		}
		seen[group.ID] = true
	}
	return nil
}

// The surrounding labeling transaction reserves SQLite's writer and stores the
// receipt. Validate every revision before changing any group, then merge once.
func mergeLabelGroupsTx(ctx context.Context, tx *sql.Tx, a LabelAction, name string, out LabelReceipt) (LabelReceipt, error) {
	args := make([]any, len(a.Groups))
	out.TargetID, out.Faces, out.Groups = a.Groups[0].ID, 0, 0
	for i, group := range a.Groups {
		args[i] = group.ID
		person, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, group.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrLabelConflict
		}
		if err != nil {
			return out, err
		}
		if person.Revision != group.Revision {
			return out, ErrLabelConflict
		}
		out.Faces += person.Count
	}
	if a.Action == "name_groups" {
		if !a.AllowDuplicate {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.name=? AND p.id NOT IN (`+sqlutil.Placeholders(len(args))+`) AND `+labelExists+`)`, append([]any{name}, args...)...).Scan(&exists); err != nil {
				return out, err
			}
			if exists {
				return out, ErrLabelNameExists
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=?`, name, searchtext.GermanFold(name), out.TargetID); err != nil {
			return out, err
		}
		out.Groups = len(a.Groups)
	}
	for _, group := range a.Groups[1:] {
		// As in web merges: retain the target's name, or adopt the first named
		// source when the target is unnamed. Ignored faces stay ignored.
		if err := mergePersonTx(ctx, tx, group.ID, out.TargetID); err != nil {
			return out, err
		}
	}
	return out, nil
}
