package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"
)

type FaceReconciliationProgress struct {
	Pending     bool  `json:"pending"`
	Cursor      int64 `json:"cursor"`
	Upper       int64 `json:"upper"`
	Processed   int64 `json:"processed"`
	Reassigned  int64 `json:"reassigned"`
	Suggestions int64 `json:"suggestions"`
}

func (l *Library) FaceReconciliationStatus(ctx context.Context) (FaceReconciliationProgress, error) {
	var out FaceReconciliationProgress
	err := l.index.db.QueryRowContext(ctx, `SELECT pending,cursor,upper_id,processed,reassigned,suggestions FROM photo_face_reconciliation WHERE id=1`).Scan(&out.Pending, &out.Cursor, &out.Upper, &out.Processed, &out.Reassigned, &out.Suggestions)
	return out, err
}

// ScheduleFaceReconciliation coalesces requests; it never interrupts an active pass.
func (l *Library) ScheduleFaceReconciliation(ctx context.Context) error {
	_, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_reconciliation SET pending=1,generation=generation+1 WHERE id=1`)
	return err
}

type faceReconcileRow struct {
	id, person int64
	path       string
	eligible   bool
}

type faceSuggestionEvidence struct {
	source, target, face, targetFace int64
	score                            float64
}

// ReconcileFacesBatch uses saved embeddings, with at most 100 source rows per
// transaction. Cursors and results commit together and survive cancellation or
// restart. No inference or thumbnail generation runs here.
func (l *Library) ReconcileFacesBatch(ctx context.Context, batchSize int) (FaceReconciliationProgress, error) {
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	batchSize = min(max(batchSize, 1), 100)
	state, err := l.FaceReconciliationStatus(ctx)
	if err != nil || !state.Pending {
		return state, err
	}
	if _, err = l.index.db.ExecContext(ctx, `UPDATE photo_face_reconciliation SET running=1,run_generation=generation,cursor=0,
 upper_id=(SELECT coalesce(max(id),0) FROM photo_faces),processed=0,reassigned=0,suggestions=0 WHERE id=1 AND pending=1 AND running=0`); err != nil {
		return state, err
	}
	state, err = l.FaceReconciliationStatus(ctx)
	if err != nil {
		return state, err
	}
	// Scan a bounded number of rows, including named groups. Filtering named
	// groups in SQL could otherwise scan the entire library in a single batch.
	rows, err := l.index.db.QueryContext(ctx, `SELECT f.id,f.person_id,f.path,
 p.name='' AND p.manual_name=0 AND f.needs_review=0 AND f.embedding_current=1 AND coalesce(f.reference_eligible,1)=1 AND f.model=(SELECT model FROM photo_face_state WHERE id=1)
 FROM photo_faces f INDEXED BY idx_face_reconcile_candidates CROSS JOIN photo_people p ON p.id=f.person_id
 WHERE f.id>? AND f.id<=? AND f.manual=0 AND f.ignored=0 AND f.favorite=0 ORDER BY f.id LIMIT ?`, state.Cursor, state.Upper, batchSize)
	if err != nil {
		return state, err
	}
	var work []faceReconcileRow
	for rows.Next() {
		var f faceReconcileRow
		if err = rows.Scan(&f.id, &f.person, &f.path, &f.eligible); err != nil {
			rows.Close()
			return state, err
		}
		work = append(work, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	visibility := newFaceDirectoryVisibility(l.root)
	skip := map[int64]bool{}
	checkedPaths := map[string]bool{}
	haveCandidates := false
	for _, f := range work {
		if err = ctx.Err(); err != nil {
			return state, err
		}
		if !f.eligible || visibility.private(parentPath(f.path)) {
			skip[f.id] = true
			continue
		}
		if failed, checked := checkedPaths[f.path]; checked {
			skip[f.id] = failed
			haveCandidates = haveCandidates || !failed
			continue
		}
		// Replaced originals must not retain a valid-looking stored embedding.
		if err = l.refreshFaceSource(ctx, f.path); err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrAdminOnly()) {
				skip[f.id] = true
				checkedPaths[f.path] = true
				continue
			}
			return state, err
		}
		checkedPaths[f.path] = false
		haveCandidates = true
	}
	var model string
	if err = l.index.db.QueryRowContext(ctx, `SELECT model FROM photo_face_state WHERE id=1`).Scan(&model); err != nil {
		return state, err
	}
	if haveCandidates {
		if err = l.ensureFaceGraph(ctx, model); err != nil {
			return state, err
		}
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return state, err
	}
	defer tx.Rollback()
	// Reserve the writer before checking model/revisions, including index writers
	// outside faceRuntime.mu.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return state, err
	}
	var revision int64
	var currentModel string
	if err = tx.QueryRowContext(ctx, `SELECT revision,model FROM photo_face_state WHERE id=1`).Scan(&revision, &currentModel); err != nil {
		return state, err
	}
	if haveCandidates && (revision != l.faceRuntime.revision || model != currentModel) {
		l.faceRuntime.graph = nil
		return state, ErrLabelConflict
	}
	affected := map[int64]bool{}
	thresholds := l.matchingThresholds()
	var named map[int64]bool
	if haveCandidates {
		named, err = faceNamedPeople(ctx, tx)
		if err != nil {
			return state, err
		}
	}
	var evidence []faceSuggestionEvidence
	var reassigned int64
	processed := 0
	deadline := time.Now().Add(250 * time.Millisecond)
	for _, f := range work {
		if processed > 0 && time.Now().After(deadline) {
			break
		}
		processed++
		if err = ctx.Err(); err != nil {
			return state, err
		}
		if skip[f.id] {
			continue
		}
		var encoded []byte
		var person int64
		var region Face
		var xmpJSON string
		err = tx.QueryRowContext(ctx, `SELECT f.person_id,f.embedding,f.x,f.y,f.width,f.height,m.faces FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path
 WHERE f.id=? AND f.model=? AND f.manual=0 AND f.favorite=0 AND f.ignored=0 AND f.needs_review=0 AND f.embedding_current=1 AND coalesce(f.reference_eligible,1)=1 AND p.name='' AND p.manual_name=0 AND m.admin_only=0`, f.id, model).Scan(&person, &encoded, &region.X, &region.Y, &region.Width, &region.Height, &xmpJSON)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return state, err
		}
		// An unnamed face overlapping an imported name may represent an explicit
		// XMP conflict. Reassociation must not bypass the initial import decision.
		var imported []Face
		if err := json.Unmarshal([]byte(xmpJSON), &imported); err != nil {
			return state, err
		}
		annotated := false
		for _, importedFace := range imported {
			if name, err := normalizedPersonName(importedFace.Name); err == nil && name != "" && overlap(region, importedFace) >= .5 {
				annotated = true
				break
			}
		}
		if annotated {
			continue
		}
		vector := decodeVector(encoded)
		if vector == nil {
			continue
		}
		if thresholds.ReconcileUnnamedGroups {
			target, moved, err := l.reconcileFaceGroup(ctx, tx, person, f.id, model, named)
			if err != nil {
				return state, err
			}
			if moved > 0 {
				affected[person], affected[target] = true, true
				reassigned += moved
				// Publish refreshed group references before the next comparison.
				// The scheduled pass regenerates any earlier, now stale evidence.
				evidence = nil
				break
			}
		}
		excluded, err := faceReconcileExclusions(ctx, tx, f.path, person)
		if err != nil {
			return state, err
		}
		ranked, err := l.faceRuntime.rankFacePersons(ctx, vector, excluded)
		if err != nil {
			return state, err
		}
		// Automatic reassignment keeps its global score and margin rules.
		candidates, err := l.validateFacePersonCandidates(ctx, tx, vector, ranked, 3)
		if err != nil {
			return state, err
		}
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		margin := best.score + 1
		if len(candidates) > 1 {
			margin = best.score - candidates[1].score
		}
		var trusted, rejected bool
		if err = tx.QueryRowContext(ctx, `SELECT name<>'' AND manual_name=1 FROM photo_people WHERE id=?`, best.person).Scan(&trusted); err != nil {
			return state, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions WHERE rejected=1 AND ((source_id=? AND target_id=?) OR (source_id=? AND target_id=?)))`, person, best.person, best.person, person).Scan(&rejected); err != nil {
			return state, err
		}
		if !thresholds.ReconcileUnnamedGroups && trusted && !rejected && best.score >= thresholds.ReconcileSimilarity && margin >= thresholds.ReconcileMargin {
			if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=? WHERE id=?`, best.person, f.id); err != nil {
				return state, err
			}
			affected[person], affected[best.person] = true, true
			reassigned++
			continue
		}
		suggestions, err := l.faceReconciliationSuggestions(ctx, tx, person, vector, ranked, named)
		if err != nil {
			return state, err
		}
		for _, candidate := range suggestions {
			evidence = append(evidence, faceSuggestionEvidence{source: person, target: candidate.person, face: f.id, targetFace: candidate.face, score: candidate.score})
		}
	}
	committedRevision := revision
	if len(affected) > 0 {
		committedRevision, err = refreshFaceMutationTx(ctx, tx, affected)
		if err != nil {
			return state, err
		}
	}
	var suggestions int64
	for _, item := range evidence {
		added, err := cacheFaceMergeSuggestion(ctx, tx, item, model)
		if err != nil {
			return state, err
		}
		if added {
			suggestions++
		}
	}
	cursor := state.Upper
	if processed < len(work) || len(work) == batchSize {
		cursor = work[processed-1].id
	}
	finished := cursor >= state.Upper
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_reconciliation SET cursor=?,processed=processed+?,reassigned=reassigned+?,suggestions=suggestions+?,
 running=?,pending=CASE WHEN ? THEN generation<>run_generation ELSE 1 END WHERE id=1`, cursor, processed, reassigned, suggestions, !finished, finished); err != nil {
		return state, err
	}
	if err = tx.Commit(); err != nil {
		return state, err
	}
	if len(affected) > 0 {
		l.syncFaceMutation(ctx, affected, revision, committedRevision)
	}
	return l.FaceReconciliationStatus(ctx)
}

// Reuse the automatic assignment's ranking without rescoring all references.
// Named people get the first three review slots. Only when none qualifies do we
// validate unnamed groups; margins compare distinct people in the same scope.
func (l *Library) faceReconciliationSuggestions(ctx context.Context, tx faceRowsQuery, source int64, vector []float32, ranked []facePersonCandidate, named map[int64]bool) ([]facePersonCandidate, error) {
	// Rejected pairs and groups sharing a photo cannot be merged. Filter them
	// before selecting the top three so they cannot block a valid fallback.
	rows, err := tx.QueryContext(ctx, `SELECT target_id FROM photo_face_merge_suggestions WHERE source_id=? AND rejected=1
 UNION SELECT source_id FROM photo_face_merge_suggestions WHERE target_id=? AND rejected=1
 UNION SELECT b.person_id FROM (SELECT DISTINCT path FROM photo_faces WHERE person_id=? AND ignored=0) a
 JOIN photo_faces b ON b.path=a.path AND b.ignored=0`, source, source, source)
	if err != nil {
		return nil, err
	}
	excluded := make(map[int64]bool)
	for rows.Next() {
		var person int64
		if err := rows.Scan(&person); err != nil {
			rows.Close()
			return nil, err
		}
		excluded[person] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	thresholds := l.matchingThresholds()
	for _, scope := range []facePersonScope{facePersonsNamed, facePersonsUnconfirmedUnnamed} {
		if scope == facePersonsNamed && len(named) == 0 {
			continue
		}
		scoped := make([]facePersonCandidate, 0, len(ranked))
		for _, candidate := range ranked {
			if !excluded[candidate.person] && named[candidate.person] == (scope == facePersonsNamed) {
				scoped = append(scoped, candidate)
			}
		}
		candidates, err := l.validateFaceCandidatesInScope(ctx, tx, &l.faceRuntime, vector, scoped, 3, scope)
		if err != nil {
			return nil, err
		}
		var result []facePersonCandidate
		for index, candidate := range candidates {
			if candidate.score < thresholds.SuggestionSimilarity {
				break
			}
			if reviewCandidateAllowed(candidates, index, thresholds.SuggestionMargin) {
				result = append(result, candidate)
			}
		}
		if len(result) > 0 {
			return result, nil
		}
	}
	return nil, nil
}

func faceReconcileExclusions(ctx context.Context, tx *sql.Tx, path string, source int64) (map[int64]bool, error) {
	excluded := map[int64]bool{source: true}
	if err := folderExcludedPeople(ctx, tx, parentPath(path), excluded); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT person_id FROM photo_faces WHERE path=? AND ignored=0`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var person int64
		if err := rows.Scan(&person); err != nil {
			return nil, err
		}
		excluded[person] = true
	}
	return excluded, rows.Err()
}
