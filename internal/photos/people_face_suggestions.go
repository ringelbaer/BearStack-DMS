package photos

import (
	"context"

	"bearstack/internal/sqlutil"
)

// SuggestPeopleForFace performs an on-demand comparison with current named
// reference groups. It never runs inference or changes person assignments.
func (l *Library) SuggestPeopleForFace(ctx context.Context, id int64) (PeopleSuggestions, error) {
	out := PeopleSuggestions{People: []PersonSuggestion{}}
	if id <= 0 {
		return out, ErrLabelInvalid
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	source, err := l.Face(ctx, id)
	if err != nil {
		return out, err
	}
	if source.Ignored {
		return out, ErrLabelConflict
	}
	var encoded []byte
	var model string
	if err = l.index.db.QueryRowContext(ctx, `SELECT embedding,model FROM photo_faces WHERE id=? AND ignored=0`, id).Scan(&encoded, &model); err != nil {
		return out, err
	}
	vector := decodeVector(encoded)
	if vector == nil {
		return out, ErrLabelInvalid
	}
	if err = l.ensureFaceGraph(ctx, model); err != nil {
		return out, err
	}
	// Filter before scoring: many unnamed groups must not crowd out named results.
	excluded := make(map[int64]bool, len(l.faceRuntime.nodes))
	for person := range l.faceRuntime.nodes {
		excluded[person] = true
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT id FROM photo_people WHERE name<>''`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var person int64
		if err = rows.Scan(&person); err != nil {
			rows.Close()
			return out, err
		}
		delete(excluded, person)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	excluded[source.PersonID] = true
	candidates, err := l.facePersonCandidates(ctx, l.index.db, vector, excluded, 20)
	if err != nil {
		return out, err
	}
	ids := []int64{}
	args := []any{}
	for _, candidate := range candidates {
		if candidate.score < faceSuggestionMinimum {
			break
		}
		ids = append(ids, candidate.person)
		args = append(args, candidate.face)
	}
	if len(ids) == 0 {
		return out, nil
	}
	// Counts and names must also respect fresh protection markers in other folders
	// belonging to a candidate, not just the matching reference's folder.
	if err = l.refreshPersonIDsVisibility(ctx, ids...); err != nil {
		return out, err
	}
	rows, err = l.index.db.QueryContext(ctx, `SELECT p.id,p.name,f.id,
 (SELECT count(DISTINCT path) FROM photo_faces WHERE person_id=p.id AND ignored=0)
 FROM photo_faces f JOIN photo_people p ON p.id=f.person_id
 WHERE f.id IN (`+sqlutil.Placeholders(len(args))+`) AND f.ignored=0 AND p.name<>''`, args...)
	if err != nil {
		return out, err
	}
	people := map[int64]PersonSuggestion{}
	for rows.Next() {
		var p PersonSuggestion
		if err = rows.Scan(&p.ID, &p.Name, &p.FaceID, &p.Count); err != nil {
			rows.Close()
			return out, err
		}
		people[p.ID] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// A concurrent source removal must not produce suggestions for an obsolete face.
	var current int64
	if err = l.index.db.QueryRowContext(ctx, `SELECT person_id FROM photo_faces WHERE id=? AND ignored=0`, id).Scan(&current); err != nil {
		return out, err
	}
	if current != source.PersonID {
		return out, ErrLabelConflict
	}
	for _, candidate := range candidates {
		if p, ok := people[candidate.person]; ok && p.FaceID == candidate.face {
			out.People = append(out.People, p)
		}
	}
	return out, nil
}
