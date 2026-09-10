package photos

import (
	"context"
	"database/sql"
	"errors"

	"bearstack/internal/searchtext"
)

type LabelMergeSuggestion struct {
	ID     int64       `json:"id"`
	Score  float64     `json:"score"`
	Source LabelPerson `json:"source"`
	Target LabelPerson `json:"target"`
}

// LabelNextMergeSuggestion reads a bounded set of cached candidates and decorates
// only the current pair. Its portraits are the actual matching witnesses, even
// when they are far beyond the first page of a large person group.
func (l *Library) LabelNextMergeSuggestion(ctx context.Context) (*LabelMergeSuggestion, error) {
	candidates, err := l.FaceMergeSuggestions(ctx, 20)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		out := LabelMergeSuggestion{ID: candidate.ID, Score: candidate.Score}
		valid := true
		for _, side := range []struct {
			id, revision, face int64
			person             *LabelPerson
		}{
			{candidate.SourceID, candidate.SourceRevision, candidate.SourceFaceID, &out.Source},
			{candidate.TargetID, candidate.TargetRevision, candidate.TargetFaceID, &out.Target},
		} {
			person, err := l.LabelPersonAfter(ctx, side.id, side.face-1, 1)
			if errors.Is(err, sql.ErrNoRows) {
				valid = false
				break
			}
			if err != nil {
				return nil, err
			}
			if person.Revision != side.revision || len(person.Faces) != 1 || person.Faces[0].ID != side.face {
				valid = false
				break
			}
			person.FaceID = side.face
			*side.person = person
		}
		if valid {
			return &out, nil
		}
	}
	return nil, nil
}

// nameMergeTx validates all three groups before changing either unnamed group.
// The caller reserves the writer and commits the merge and receipt together.
func nameMergeTx(ctx context.Context, tx *sql.Tx, source LabelPerson, a LabelAction, name string, out LabelReceipt) (LabelReceipt, error) {
	target, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, a.TargetID))
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrLabelConflict
	}
	if err != nil {
		return out, err
	}
	if source.Name != "" || target.Name != "" || target.Revision != a.TargetRevision {
		return out, ErrLabelConflict
	}
	destination := a.TargetID
	if a.AssignID != 0 {
		assigned, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, a.AssignID))
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrLabelConflict
		}
		if err != nil {
			return out, err
		}
		if assigned.Name == "" || assigned.Revision != a.AssignRevision {
			return out, ErrLabelConflict
		}
		destination = a.AssignID
	} else {
		if !a.AllowDuplicate {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.name=? AND `+labelExists+`)`, name).Scan(&exists); err != nil {
				return out, err
			}
			if exists {
				return out, ErrLabelNameExists
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=?`, name, searchtext.GermanFold(name), destination); err != nil {
			return out, err
		}
	}
	if err := mergePersonTx(ctx, tx, source.ID, destination); err != nil {
		return out, err
	}
	if destination != a.TargetID {
		if err := mergePersonTx(ctx, tx, a.TargetID, destination); err != nil {
			return out, err
		}
	}
	out.TargetID, out.Faces, out.Groups = destination, source.Count+target.Count, 2
	return out, nil
}
