package photos

import (
	"context"
	"database/sql"
	"errors"
)

type LabelMergeSuggestion struct {
	ID     int64       `json:"id"`
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
		out := LabelMergeSuggestion{ID: candidate.ID}
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
