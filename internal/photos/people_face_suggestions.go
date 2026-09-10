package photos

import (
	"context"
	"math"
	"slices"

	"bearstack/internal/sqlutil"
)

// SuggestPeopleForFace performs an on-demand comparison with current named
// reference groups. It never runs inference or changes person assignments.
func (l *Library) SuggestPeopleForFace(ctx context.Context, id int64) (PeopleSuggestions, error) {
	return l.suggestPeopleForFace(ctx, id, nil)
}

// SuggestPeopleForFaceStream emits checked interim rankings while more named
// groups are still being scored. Returning an error from emit stops the search.
func (l *Library) SuggestPeopleForFaceStream(ctx context.Context, id int64, emit func(PeopleSuggestions) error) (PeopleSuggestions, error) {
	return l.suggestPeopleForFace(ctx, id, emit)
}

func (l *Library) suggestPeopleForFace(ctx context.Context, id int64, emit func(PeopleSuggestions) error) (PeopleSuggestions, error) {
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
	if source.Drawn {
		return out, nil
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
	var named []int64
	rows, err := l.index.db.QueryContext(ctx, `SELECT id FROM photo_people WHERE name<>'' ORDER BY id`)
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
		if person != source.PersonID {
			named = append(named, person)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	excluded[source.PersonID] = true
	var candidates []facePersonCandidate
	// Partial rankings cannot establish a positive lead over the runner-up.
	if emit == nil || l.matchingThresholds().SuggestionMargin > 0 {
		candidates, err = l.facePersonCandidates(ctx, l.index.db, vector, excluded, 20)
	} else {
		candidates, err = l.streamNamedFaceCandidates(ctx, vector, named, func(ranking []facePersonCandidate) error {
			current, err := l.faceSuggestionPeople(ctx, id, source.PersonID, ranking)
			if err != nil {
				return err
			}
			return emit(current)
		})
	}
	if err != nil {
		return out, err
	}
	return l.faceSuggestionPeople(ctx, id, source.PersonID, candidates)
}

func (l *Library) faceSuggestionPeople(ctx context.Context, id, sourcePerson int64, candidates []facePersonCandidate) (PeopleSuggestions, error) {
	out := PeopleSuggestions{People: []PersonSuggestion{}}
	ids := []int64{}
	args := []any{}
	thresholds := l.matchingThresholds()
	for index, candidate := range candidates {
		if candidate.score < thresholds.SuggestionSimilarity {
			break
		}
		if !reviewCandidateAllowed(candidates, index, thresholds.SuggestionMargin) {
			continue
		}
		ids = append(ids, candidate.person)
		args = append(args, candidate.face)
	}
	if len(ids) == 0 {
		return out, nil
	}
	// Counts and names must also respect fresh protection markers in other folders
	// belonging to a candidate, not just the matching reference's folder.
	if err := l.refreshPersonIDsVisibility(ctx, ids...); err != nil {
		return out, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT p.id,p.name,f.id,
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
	if current != sourcePerson {
		return out, ErrLabelConflict
	}
	for _, candidate := range candidates {
		if p, ok := people[candidate.person]; ok && p.FaceID == candidate.face {
			out.People = append(out.People, p)
		}
	}
	return out, nil
}

// Score each group once. Before the first hit, validate immediately; afterwards
// use small batches so interim updates do not turn every vector into a SQL call.
func (l *Library) streamNamedFaceCandidates(ctx context.Context, vector []float32, named []int64, emit func([]facePersonCandidate) error) ([]facePersonCandidate, error) {
	var best, batch []facePersonCandidate
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		slices.SortFunc(batch, compareFaceCandidates)
		checked, err := l.validateFacePersonCandidates(ctx, l.index.db, vector, batch, 20)
		batch = batch[:0]
		if err != nil {
			return err
		}
		previous := slices.Clone(best)
		best = append(best, checked...)
		slices.SortFunc(best, compareFaceCandidates)
		best = best[:min(20, len(best))]
		if !slices.Equal(previous, best) {
			return emit(best)
		}
		return nil
	}
	for index, person := range named {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		score := math.Inf(-1)
		for i, reference := range l.faceRuntime.graph.groups[person] {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			score = max(score, cosine(vector, reference))
		}
		candidate := facePersonCandidate{person: person, score: score}
		if score >= l.matchingThresholds().SuggestionSimilarity && (len(best) < 20 || compareFaceCandidates(candidate, best[len(best)-1]) < 0) {
			batch = append(batch, candidate)
		}
		if len(best) == 0 || (index+1)%32 == 0 {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return best, nil
}
