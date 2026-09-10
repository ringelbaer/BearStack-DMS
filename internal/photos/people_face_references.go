package photos

import (
	"context"
	"database/sql"
	"iter"
	"slices"
	"sync"

	"bearstack/internal/sqlutil"
)

type faceSuggestionGroup struct{ id, revision int64 }
type faceSuggestionKey struct {
	revision int64
	model    string
	limit    int
	pending  bool
}

// Published snapshots and their vector slices are immutable. Only one new
// snapshot is built at a time; waiting does not retain a database connection.
type faceSuggestionSnapshot struct {
	key    faceSuggestionKey
	groups []faceSuggestionGroup
	faces  map[int64][]faceCandidateReference
}

func (s *faceSuggestionSnapshot) references(person int64) iter.Seq[faceCandidateReference] {
	return slices.Values(s.faces[person])
}

func (s *faceSuggestionSnapshot) checkPeople(ctx context.Context, db *sql.DB, people []PersonSuggestion) error {
	if len(people) == 0 {
		return ctx.Err()
	}
	args := make([]any, 0, len(people))
	for _, p := range people {
		args = append(args, p.ID)
	}
	rows, err := db.QueryContext(ctx, `SELECT person_id,revision FROM photo_person_revisions WHERE person_id IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var person, revision int64
		if err := rows.Scan(&person, &revision); err != nil {
			return err
		}
		refs := s.faces[person]
		if len(refs) == 0 || refs[0].revision != revision {
			return ErrLabelConflict
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(people) {
		return ErrLabelConflict
	}
	return nil
}

type faceSuggestionCache struct {
	mu       sync.Mutex
	loading  chan struct{}
	snapshot *faceSuggestionSnapshot
	epoch    uint64
}

func (c *faceSuggestionCache) clear() {
	c.mu.Lock()
	c.snapshot = nil
	c.epoch++
	c.mu.Unlock()
}

func (l *Library) namedFaceReferences(ctx context.Context, model string) (*faceSuggestionSnapshot, error) {
	c := &l.faceSuggestions
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if wait := c.loading; wait != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}
		wait, previous := make(chan struct{}), c.snapshot
		epoch := c.epoch
		c.loading = wait
		c.mu.Unlock()
		next, err := l.loadNamedFaceReferences(ctx, model, previous)
		c.mu.Lock()
		if err == nil && epoch == c.epoch {
			c.snapshot = next
		}
		c.loading = nil
		close(wait)
		c.mu.Unlock()
		return next, err
	}
}

func (l *Library) loadNamedFaceReferences(ctx context.Context, model string, previous *faceSuggestionSnapshot) (*faceSuggestionSnapshot, error) {
	conn, release, err := photoFileTempConn(ctx, l.index.db)
	if err != nil {
		return nil, err
	}
	defer release()
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	s := &faceSuggestionSnapshot{faces: make(map[int64][]faceCandidateReference)}
	if err := tx.QueryRowContext(ctx, `SELECT s.revision,s.model,r.reference_limit,r.pending
 FROM photo_face_state s CROSS JOIN photo_face_reference_settings r WHERE s.id=1 AND r.id=1`).Scan(&s.key.revision, &s.key.model, &s.key.limit, &s.key.pending); err != nil {
		return nil, err
	}
	if s.key.model != model {
		return nil, ErrLabelConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.id,v.revision FROM photo_people p
 CROSS JOIN photo_person_revisions v ON v.person_id=p.id WHERE p.name<>'' ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g faceSuggestionGroup
		if err := rows.Scan(&g.id, &g.revision); err != nil {
			rows.Close()
			return nil, err
		}
		s.groups = append(s.groups, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if previous != nil && previous.key == s.key && slices.Equal(previous.groups, s.groups) {
		return previous, ctx.Err()
	}
	// Per-person revisions track every supported face edit. If only the global
	// revision changed (e.g. reference maintenance), reload conservatively.
	reuse := previous != nil && previous.key.model == model && previous.key.limit == s.key.limit && previous.key.pending == s.key.pending &&
		(previous.key.revision == s.key.revision || !slices.Equal(previous.groups, s.groups))
	old := map[int64]int64{}
	if reuse {
		for _, g := range previous.groups {
			old[g.id] = g.revision
		}
	}
	var changed []faceSuggestionGroup
	for _, g := range s.groups {
		if old[g.id] == g.revision {
			s.faces[g.id] = previous.faces[g.id]
		} else {
			changed = append(changed, g)
		}
	}
	// A ready worker cache can donate immutable vectors. TryLock never makes
	// interactive requests wait for inference, reconciliation or face edits.
	if !s.key.pending && l.faceRuntime.mu.TryLock() {
		rt := &l.faceRuntime
		if rt.graph != nil && rt.revision == s.key.revision && rt.model == model && rt.referenceLimit == s.key.limit {
			for _, g := range changed {
				for ref := range rt.references(g.id) {
					ref.revision = g.revision
					s.faces[g.id] = append(s.faces[g.id], ref)
				}
			}
			changed = nil
		}
		rt.mu.Unlock()
	}
	for start := 0; start < len(changed); start += 128 {
		batch := changed[start:min(start+128, len(changed))]
		args, revisions := make([]any, 0, len(batch)+2), make(map[int64]int64, len(batch))
		for _, g := range batch {
			args = append(args, g.id)
			revisions[g.id] = g.revision
		}
		query := `SELECT r.person_id,f.id,f.embedding FROM photo_face_references r INDEXED BY idx_face_references_person
 CROSS JOIN photo_faces f ON f.id=r.face_id CROSS JOIN media_index m ON m.path=f.path
 WHERE r.person_id IN (` + sqlutil.Placeholders(len(batch)) + `) AND f.model=? AND f.ignored=0 AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1) ORDER BY r.person_id,r.face_id`
		args = append(args, model)
		if s.key.pending {
			// Select current references only for these named groups. The existing
			// worker continues the resumable global refresh in the background.
			query = namedPendingReferencesSQL(len(batch))
			args = append(args, s.key.limit)
		}
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ref faceCandidateReference
			var encoded []byte
			if err := rows.Scan(&ref.person, &ref.id, &encoded); err != nil {
				rows.Close()
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			ref.vector, ref.revision = decodeVector(encoded), revisions[ref.person]
			if ref.vector != nil {
				s.faces[ref.person] = append(s.faces[ref.person], ref)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return s, ctx.Err()
}

func namedPendingReferencesSQL(n int) string {
	// Identical favorite/quality/folder rules to refreshFaceReferencesTx, without
	// writing the shared selection or loading vectors for unselected faces.
	return `WITH ranked AS (
 SELECT f.id,f.person_id,f.favorite,f.manual,f.confidence,
 row_number() OVER (PARTITION BY f.person_id,f.directory ORDER BY f.favorite DESC,f.manual DESC,f.confidence DESC,f.id) AS directory_rank
 FROM photo_faces f CROSS JOIN media_index m ON m.path=f.path
 WHERE f.person_id IN (` + sqlutil.Placeholders(n) + `) AND f.model=? AND f.ignored=0 AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1) AND f.drawn=0),
 selected AS (SELECT *,row_number() OVER (PARTITION BY person_id ORDER BY favorite DESC,directory_rank,manual DESC,confidence DESC,id) AS reference_rank FROM ranked)
 SELECT s.person_id,f.id,f.embedding FROM selected s CROSS JOIN photo_faces f ON f.id=s.id
 WHERE s.favorite=1 OR s.reference_rank<=? ORDER BY s.person_id,f.id`
}
