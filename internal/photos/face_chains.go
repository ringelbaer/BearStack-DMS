package photos

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"slices"

	"bearstack/internal/sqlutil"
)

const MaxFaceChainHops = 5
const MaxFaceChainGroups = 1000
const MaxFaceChainExclusions = 10000
const FaceChainPageSize = 60

var ErrFaceChainLarge = errors.New("Die Kette umfasst mehr als 1.000 Gruppen. Bitte eine höhere Mindestähnlichkeit oder weniger Sprünge wählen.")

type FaceChainSearch struct {
	Similarity     *float64 `json:"similarity,omitempty"`
	Hops           int      `json:"hops"`
	After          int64    `json:"after"`
	ExcludedGroups []int64  `json:"excluded_groups"`
}

type FaceChainGroup struct {
	LabelGroupRef
	Depth int `json:"depth"`
}

type FaceChain struct {
	Dataset    string           `json:"dataset"`
	Groups     []FaceChainGroup `json:"groups"`
	After      int64            `json:"after"`
	HasMore    bool             `json:"has_more"`
	Similarity float64          `json:"similarity"`
}

// One seed per request keeps isolated groups resumable without building a
// collection-wide graph. All eligible faces participate, including faces outside
// the recognition reference limit. Only a small source-vector block is retained.
// No matching runs from a worker, page listing or application startup.
func (l *Library) NextFaceChain(ctx context.Context, request FaceChainSearch) (FaceChain, error) {
	out := FaceChain{Groups: []FaceChainGroup{}, After: request.After}
	if request.Hops < 1 || request.Hops > MaxFaceChainHops || request.After < 0 || len(request.ExcludedGroups) > MaxFaceChainExclusions {
		return out, ErrLabelInvalid
	}
	if v := request.Similarity; v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1) {
		return out, ErrLabelInvalid
	}
	excluded := make(map[int64]bool, len(request.ExcludedGroups))
	for _, id := range request.ExcludedGroups {
		if id <= 0 || excluded[id] {
			return out, ErrLabelInvalid
		}
		excluded[id] = true
	}
	// Leave the second index connection available for interactive edits and the
	// worker. Waiting requests remain cancelable and never hold a connection.
	select {
	case l.faceChainGate <- struct{}{}:
		defer func() { <-l.faceChainGate }()
	case <-ctx.Done():
		return out, ctx.Err()
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var model string
	if err := tx.QueryRowContext(ctx, `SELECT i.dataset,s.model,t.suggestion_similarity FROM photo_labeling_identity i CROSS JOIN photo_face_state s CROSS JOIN photo_face_thresholds t WHERE i.id=1 AND s.id=1 AND t.id=1`).Scan(&out.Dataset, &model, &out.Similarity); err != nil {
		return out, err
	}
	if request.Similarity != nil {
		out.Similarity = *request.Similarity
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.id,v.revision FROM photo_people p CROSS JOIN photo_person_revisions v ON v.person_id=p.id
 WHERE p.name='' AND p.id>? AND EXISTS(SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path
 WHERE f.person_id=p.id AND f.ignored=0 AND f.needs_review=0 AND f.embedding_current=1 AND f.drawn=0 AND f.model=? AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1)) ORDER BY p.id`, request.After, model)
	if err != nil {
		return out, err
	}
	var seed FaceChainGroup
	for rows.Next() {
		if err := rows.Scan(&seed.ID, &seed.Revision); err != nil {
			rows.Close()
			return out, err
		}
		if !excluded[seed.ID] {
			break
		}
		seed = FaceChainGroup{}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || seed.ID == 0 {
		return out, err
	}
	out.After, out.HasMore = seed.ID, true
	groups := map[int64]FaceChainGroup{seed.ID: seed}
	frontier := []int64{seed.ID}
	visibility := newFaceDirectoryVisibility(l.root)
	for depth := 1; depth <= request.Hops && len(frontier) > 0; depth++ {
		next, err := l.expandFaceChain(ctx, tx, model, frontier, groups, excluded, visibility, depth, out.Similarity)
		if err != nil {
			return out, err
		}
		frontier = next
	}
	if len(groups) < 2 {
		return out, nil
	}
	for _, group := range groups {
		out.Groups = append(out.Groups, group)
	}
	slices.SortFunc(out.Groups, func(a, b FaceChainGroup) int {
		if a.Depth < b.Depth {
			return -1
		}
		if a.Depth > b.Depth {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}

type chainVector struct {
	person int64
	vector []float32
}

const faceChainVectorsSQL = `SELECT f.person_id,f.embedding,f.path,v.revision
 FROM photo_people p CROSS JOIN photo_faces f ON f.person_id=p.id
 CROSS JOIN media_index m ON m.path=f.path CROSS JOIN photo_person_revisions v ON v.person_id=p.id
 WHERE p.name='' AND f.ignored=0 AND f.needs_review=0 AND f.embedding_current=1 AND f.drawn=0 AND f.model=? AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1)`

func (l *Library) expandFaceChain(ctx context.Context, tx *sql.Tx, model string, frontier []int64, groups map[int64]FaceChainGroup, excluded map[int64]bool, visibility *faceDirectoryVisibility, depth int, similarity float64) ([]int64, error) {
	args := []any{model}
	for _, id := range frontier {
		args = append(args, id)
	}
	sources, err := tx.QueryContext(ctx, faceChainVectorsSQL+` AND p.id IN (`+sqlutil.Placeholders(len(frontier))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer sources.Close()
	var next []int64
	block := make([]chainVector, 0, 64)
	// Cache only pairs for which a vector actually passes the similarity test.
	blocked := map[[2]int64]bool{}
	compare := func() error {
		rows, err := tx.QueryContext(ctx, faceChainVectorsSQL, model)
		if err != nil {
			return err
		}
		defer rows.Close()
		var lastPerson int64
		for rows.Next() {
			var id, revision int64
			var encoded []byte
			var path string
			if err := rows.Scan(&id, &encoded, &path, &revision); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, seen := groups[id]; seen || excluded[id] {
				continue
			}
			if id != lastPerson {
				clear(blocked)
				lastPerson = id
			}
			vector := decodeVector(encoded)
			if vector == nil || visibility.private(parentPath(path)) {
				continue
			}
			for _, source := range block {
				if cosine(source.vector, vector) < similarity {
					continue
				}
				key := [2]int64{source.person, id}
				veto, checked := blocked[key]
				if !checked {
					if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions WHERE rejected=1 AND ((source_id=? AND target_id=?) OR (source_id=? AND target_id=?)))
 OR EXISTS(SELECT 1 FROM photo_faces a JOIN photo_faces b ON b.path=a.path WHERE a.person_id=? AND b.person_id=? AND a.ignored=0 AND b.ignored=0)`, source.person, id, id, source.person, source.person, id).Scan(&veto); err != nil {
						return err
					}
					blocked[key] = veto
				}
				if veto {
					continue
				}
				if len(groups) >= MaxFaceChainGroups {
					return ErrFaceChainLarge
				}
				groups[id] = FaceChainGroup{LabelGroupRef: LabelGroupRef{ID: id, Revision: revision}, Depth: depth}
				next = append(next, id)
				break
			}
		}
		return rows.Err()
	}
	for sources.Next() {
		var id, revision int64
		var encoded []byte
		var path string
		if err := sources.Scan(&id, &encoded, &path, &revision); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if visibility.private(parentPath(path)) {
			continue
		}
		if vector := decodeVector(encoded); vector != nil {
			block = append(block, chainVector{id, vector})
		}
		if len(block) == cap(block) {
			if err := compare(); err != nil {
				return nil, err
			}
			block = block[:0]
		}
	}
	if err := sources.Err(); err != nil {
		return nil, err
	}
	if len(block) > 0 {
		if err := compare(); err != nil {
			return nil, err
		}
	}
	slices.Sort(next)
	return next, nil
}
