package photos

import (
	"context"
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

type facePersonCandidate struct {
	person int64
	score  float64
	face   int64 // The currently valid reference that supplies score.
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rt.graph == nil {
		return nil, nil
	}
	ranked := make([]facePersonCandidate, 0, len(rt.nodes))
	checked := 0
	for person, references := range rt.graph.groups {
		if excluded[person] {
			continue
		}
		best := math.Inf(-1)
		for _, vector := range references {
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
		validate := func(batch []any) error {
			rows, err := tx.QueryContext(ctx, `SELECT f.id,f.person_id,f.path,p.name_source FROM photo_faces f
 JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path
 WHERE f.id IN (`+sqlutil.Placeholders(len(batch))+`) AND f.ignored=0 AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1)`, batch...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id, person int64
				var path, nameSource string
				if err := rows.Scan(&id, &person, &path, &nameSource); err != nil {
					rows.Close()
					return err
				}
				if err := ctx.Err(); err != nil {
					rows.Close()
					return err
				}
				if person != rt.people[id] || visibility.private(parentPath(path)) ||
					(nameSource != "" && visibility.private(parentPath(nameSource))) {
					continue
				}
				if vector := rt.faceReferenceVector(id); vector != nil {
					score := cosine(v, vector)
					if old, ok := scores[person]; !ok || score > old.score || (score == old.score && id < old.face) {
						scores[person] = facePersonCandidate{person: person, score: score, face: id}
					}
				}
			}
			err = rows.Err()
			rows.Close()
			return err
		}
		// Keep argument memory bounded even for a person with many favorites.
		args := make([]any, 0, 512)
		for _, candidate := range ranked[offset:end] {
			for _, id := range rt.nodes[candidate.person] {
				args = append(args, id)
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
