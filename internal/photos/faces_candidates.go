package photos

import (
	"context"
	"iter"
	"maps"
	"math"
	"slices"

	"bearstack/internal/sqlutil"
)

// Keep the reference cache's existing invalidation and incremental-update
// contract, without an approximate graph that can disconnect dense face groups.
type faceVectorIndex struct {
	vectors map[int64][]float32
	groups  map[int64][][]float32
}

func newFaceVectorIndex() *faceVectorIndex {
	return &faceVectorIndex{vectors: make(map[int64][]float32), groups: make(map[int64][][]float32)}
}

func (g *faceVectorIndex) Len() int { return len(g.vectors) }

type facePersonScope uint8

const (
	facePersonsAll facePersonScope = iota
	facePersonsNamed
	facePersonsUnnamed
)

type facePersonCandidate struct {
	person int64
	score  float64
	face   int64 // The currently valid reference that supplies score.
}

// Vectors are immutable after decoding. A suggestion snapshot can therefore
// validate its own references without borrowing the worker's mutable maps.
type faceCandidateReference struct {
	id, person, revision int64
	vector               []float32
}

type faceCandidateReferences interface {
	references(person int64) iter.Seq[faceCandidateReference]
}

func (rt *faceRuntime) references(person int64) iter.Seq[faceCandidateReference] {
	return func(yield func(faceCandidateReference) bool) {
		for _, id := range rt.nodes[person] {
			if !yield(faceCandidateReference{id: id, person: rt.people[id], vector: rt.faceReferenceVector(id)}) {
				return
			}
		}
	}
}

func faceCandidateValidationSQL(count int) string {
	// SQLite must start with the bounded ID set, even before ANALYZE has run.
	return `SELECT f.id,f.person_id,f.path,p.name_source,v.revision FROM photo_faces f
 CROSS JOIN photo_people p ON p.id=f.person_id CROSS JOIN media_index m ON m.path=f.path
 CROSS JOIN photo_person_revisions v ON v.person_id=p.id
 WHERE f.id IN (` + sqlutil.Placeholders(count) + `) AND f.ignored=0 AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1)`
}

func compareFaceCandidates(a, b facePersonCandidate) int {
	if a.score > b.score {
		return -1
	}
	if a.score < b.score {
		return 1
	}
	if a.person < b.person {
		return -1
	}
	if a.person > b.person {
		return 1
	}
	return 0
}

func (rt *faceRuntime) faceReferenceVector(id int64) []float32 {
	if v := rt.favorites[id]; v != nil {
		return v
	}
	return rt.graph.vectors[id]
}

// rankFacePersons scores every eligible cached reference, including every
// favorite. The maximum is an upper bound on that person's currently visible
// score: live transaction and filesystem checks may only remove references.
// Caller holds faceRuntime.mu and has called ensureFaceGraph.
func (rt *faceRuntime) rankFacePersons(ctx context.Context, v []float32, excluded map[int64]bool) ([]facePersonCandidate, error) {
	return rt.rankFacePersonsInScope(ctx, v, excluded, nil, facePersonsAll)
}

func (rt *faceRuntime) rankFacePersonsInScope(ctx context.Context, v []float32, excluded, named map[int64]bool, scope facePersonScope) ([]facePersonCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rt.graph == nil {
		return nil, nil
	}
	capacity := len(rt.nodes)
	persons := maps.Keys(rt.graph.groups)
	if scope == facePersonsNamed {
		capacity = min(capacity, len(named))
		persons = maps.Keys(named)
	}
	ranked := make([]facePersonCandidate, 0, capacity)
	checked := 0
	for person := range persons {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if excluded[person] || (scope == facePersonsUnnamed && named[person]) {
			continue
		}
		best := math.Inf(-1)
		for _, vector := range rt.graph.groups[person] {
			if checked%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			checked++
			best = max(best, cosine(v, vector))
		}
		if !math.IsInf(best, -1) {
			ranked = append(ranked, facePersonCandidate{person: person, score: best})
		}
	}
	slices.SortFunc(ranked, compareFaceCandidates)
	return ranked, ctx.Err()
}

// facePersonCandidates returns exact, distinct-person scores in descending
// order. Only groups that can enter the requested top results need SQL reads.
// If live privacy, deletion, reassignment or exclusion invalidates a leading
// reference, continue through the ranking instead of starving the result set.
// Caller holds faceRuntime.mu and has called ensureFaceGraph.
func (l *Library) facePersonCandidates(ctx context.Context, tx faceRowsQuery, v []float32, excluded map[int64]bool, limit int) ([]facePersonCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	rt := &l.faceRuntime
	ranked, err := rt.rankFacePersons(ctx, v, excluded)
	if err != nil {
		return nil, err
	}
	return l.validateFacePersonCandidates(ctx, tx, v, ranked, limit)
}

// ranked contains per-person upper bounds in descending order.
func (l *Library) validateFacePersonCandidates(ctx context.Context, tx faceRowsQuery, v []float32, ranked []facePersonCandidate, limit int) ([]facePersonCandidate, error) {
	return l.validateFaceCandidates(ctx, tx, &l.faceRuntime, v, ranked, limit)
}

func (l *Library) validateFaceCandidates(ctx context.Context, tx faceRowsQuery, refs faceCandidateReferences, v []float32, ranked []facePersonCandidate, limit int) ([]facePersonCandidate, error) {
	return l.validateFaceCandidatesInScope(ctx, tx, refs, v, ranked, limit, facePersonsAll)
}

func (l *Library) validateFaceCandidatesInScope(ctx context.Context, tx faceRowsQuery, refs faceCandidateReferences, v []float32, ranked []facePersonCandidate, limit int, scope facePersonScope) ([]facePersonCandidate, error) {
	if limit <= 0 {
		return nil, ctx.Err()
	}
	limit = min(limit, len(ranked))
	result := make([]facePersonCandidate, 0, limit)
	visibility := newFaceDirectoryVisibility(l.root)
	for offset := 0; offset < len(ranked); {
		if len(result) == limit && compareFaceCandidates(ranked[offset], result[limit-1]) >= 0 {
			break
		}
		// Validate enough groups to fill the result, sharing bounded SQL batches
		// across groups (including groups with unlimited favorite references).
		end := min(len(ranked), offset+max(1, limit-len(result)))
		scores := make(map[int64]facePersonCandidate, end-offset)
		witnesses := make(map[int64]faceCandidateReference, 512)
		validate := func(batch []any) error {
			query := faceCandidateValidationSQL(len(batch))
			if scope == facePersonsNamed {
				query += ` AND p.name<>''`
			} else if scope == facePersonsUnnamed {
				query += ` AND p.name=''`
			}
			rows, err := tx.QueryContext(ctx, query, batch...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id, person, revision int64
				var path, nameSource string
				if err := rows.Scan(&id, &person, &path, &nameSource, &revision); err != nil {
					rows.Close()
					return err
				}
				if err := ctx.Err(); err != nil {
					rows.Close()
					return err
				}
				ref := witnesses[id]
				if person != ref.person || (ref.revision != 0 && ref.revision != revision) || visibility.private(parentPath(path)) ||
					(nameSource != "" && visibility.private(parentPath(nameSource))) {
					continue
				}
				if vector := ref.vector; vector != nil {
					score := cosine(v, vector)
					if old, ok := scores[person]; !ok || score > old.score || (score == old.score && id < old.face) {
						scores[person] = facePersonCandidate{person: person, score: score, face: id}
					}
				}
			}
			err = rows.Err()
			rows.Close()
			clear(witnesses)
			return err
		}
		// Keep argument memory bounded even for a person with many favorites.
		args := make([]any, 0, 512)
		for _, candidate := range ranked[offset:end] {
			for ref := range refs.references(candidate.person) {
				args = append(args, ref.id)
				witnesses[ref.id] = ref
				if len(args) == cap(args) {
					if err := validate(args); err != nil {
						return nil, err
					}
					args = args[:0]
				}
			}
		}
		if len(args) > 0 {
			if err := validate(args); err != nil {
				return nil, err
			}
		}
		for _, candidate := range scores {
			result = append(result, candidate)
		}
		slices.SortFunc(result, compareFaceCandidates)
		result = result[:min(limit, len(result))]
		offset = end
	}
	return result, ctx.Err()
}
