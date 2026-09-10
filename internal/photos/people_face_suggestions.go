package photos

import (
	"context"
	"math"
	"slices"
	"time"

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
	var person, revision int64
	if err = l.index.db.QueryRowContext(ctx, `SELECT f.embedding,f.model,f.person_id,v.revision FROM photo_faces f
 CROSS JOIN photo_person_revisions v ON v.person_id=f.person_id WHERE f.id=? AND f.ignored=0`, id).Scan(&encoded, &model, &person, &revision); err != nil {
		return out, err
	}
	if person != source.PersonID {
		return out, ErrLabelConflict
	}
	vector := decodeVector(encoded)
	if vector == nil {
		return out, ErrLabelInvalid
	}
	snapshot, err := l.namedFaceReferences(ctx, model)
	if err != nil {
		return out, err
	}
	thresholds, err := l.FaceThresholds(ctx)
	if err != nil {
		return out, err
	}
	verify := func() error {
		current, err := l.Face(ctx, id)
		if err != nil {
			return err
		}
		if current.Ignored || current.PersonID != source.PersonID {
			return ErrLabelConflict
		}
		var currentRevision int64
		var currentModel string
		var limit int
		if err := l.index.db.QueryRowContext(ctx, `SELECT v.revision,s.model,r.reference_limit
 FROM photo_person_revisions v CROSS JOIN photo_face_state s CROSS JOIN photo_face_reference_settings r
 WHERE v.person_id=? AND s.id=1 AND r.id=1`, source.PersonID).Scan(&currentRevision, &currentModel, &limit); err != nil {
			return err
		}
		if currentRevision != revision || currentModel != model || limit != snapshot.key.limit {
			return ErrLabelConflict
		}
		currentThresholds, err := l.FaceThresholds(ctx)
		if err != nil {
			return err
		}
		if currentThresholds != thresholds {
			return ErrLabelConflict
		}
		return ctx.Err()
	}
	return l.rankNamedFaceSuggestions(ctx, snapshot, vector, source.PersonID, thresholds, func(ranking []facePersonCandidate) (PeopleSuggestions, error) {
		current, err := l.faceSuggestionPeople(ctx, id, source.PersonID, ranking, thresholds)
		if err != nil {
			return current, err
		}
		if err := snapshot.checkPeople(ctx, l.index.db, current.People); err != nil {
			return out, err
		}
		if err := verify(); err != nil {
			return out, err
		}
		return current, nil
	}, emit)
}

func (l *Library) faceSuggestionPeople(ctx context.Context, id, sourcePerson int64, candidates []facePersonCandidate, thresholds FaceThresholds) (PeopleSuggestions, error) {
	out := PeopleSuggestions{People: []PersonSuggestion{}}
	ids := []int64{}
	args := []any{}
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
 FROM photo_faces f CROSS JOIN photo_people p ON p.id=f.person_id
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

const faceSuggestionUpdateInterval = 100 * time.Millisecond

// Score every eligible named reference. Keep upper bounds for all groups so
// invalidated leading witnesses never hide a lower currently valid candidate.
// SQL validation and serialization happen only for the first hit, timed updates
// and the final ranking, rather than after every 32 groups.
func (l *Library) rankNamedFaceSuggestions(ctx context.Context, snapshot *faceSuggestionSnapshot, vector []float32, source int64, thresholds FaceThresholds, assemble func([]facePersonCandidate) (PeopleSuggestions, error), emit func(PeopleSuggestions) error) (PeopleSuggestions, error) {
	out := PeopleSuggestions{People: []PersonSuggestion{}}
	ranked := make([]facePersonCandidate, 0, len(snapshot.groups))
	var lastUpdate time.Time
	var previous []PersonSuggestion
	publish := func(ranking []facePersonCandidate, final bool) (PeopleSuggestions, error) {
		slices.SortFunc(ranking, compareFaceCandidates)
		checked, err := l.validateFaceCandidates(ctx, l.index.db, snapshot, vector, ranking, 20)
		if err != nil {
			return out, err
		}
		current, err := assemble(checked)
		if err != nil {
			return out, err
		}
		if !final && len(current.People) > 0 && !slices.Equal(previous, current.People) {
			if err := emit(current); err != nil {
				return out, err
			}
			previous = slices.Clone(current.People)
			lastUpdate = time.Now()
		}
		return current, nil
	}
	streaming := emit != nil && thresholds.SuggestionMargin == 0
	for _, group := range snapshot.groups {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if group.id == source {
			continue
		}
		score := math.Inf(-1)
		for i, ref := range snapshot.faces[group.id] {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return out, err
				}
			}
			score = max(score, cosine(vector, ref.vector))
		}
		if math.IsInf(score, -1) || (score < thresholds.SuggestionSimilarity && thresholds.SuggestionMargin == 0) {
			continue
		}
		candidate := facePersonCandidate{person: group.id, score: score}
		ranked = append(ranked, candidate)
		if streaming && lastUpdate.IsZero() {
			if _, err := publish([]facePersonCandidate{candidate}, false); err != nil {
				return out, err
			}
		} else if streaming && time.Since(lastUpdate) >= faceSuggestionUpdateInterval {
			// Mark attempts too, so unchanged rankings do not trigger repeated SQL.
			lastUpdate = time.Now()
			if _, err := publish(ranked, false); err != nil {
				return out, err
			}
		}
	}
	return publish(ranked, true)
}
