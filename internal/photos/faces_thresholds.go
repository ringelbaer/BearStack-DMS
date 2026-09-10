package photos

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

// FaceThresholds keeps recognition, retrospective moves and manual review independent.
type FaceThresholds struct {
	AssignmentSimilarity float64 `json:"assignment_similarity"`
	AssignmentMargin     float64 `json:"assignment_margin"`
	ReconcileSimilarity  float64 `json:"reconcile_similarity"`
	ReconcileMargin      float64 `json:"reconcile_margin"`
	SuggestionSimilarity float64 `json:"suggestion_similarity"`
	SuggestionMargin     float64 `json:"suggestion_margin"`
}

func DefaultFaceThresholds() FaceThresholds {
	return FaceThresholds{0.55, 0.08, 0.62, 0.10, 0.45, 0}
}

func (v FaceThresholds) Validate() error {
	for _, x := range []float64{v.AssignmentSimilarity, v.ReconcileSimilarity, v.SuggestionSimilarity} {
		if math.IsNaN(x) || math.IsInf(x, 0) || x < .4 || x > .7 {
			return errors.New("Ähnlichkeit muss zwischen 0,4 und 0,7 liegen")
		}
	}
	for _, x := range []float64{v.AssignmentMargin, v.ReconcileMargin, v.SuggestionMargin} {
		if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 || x > .2 {
			return errors.New("Mindestabstand muss zwischen 0,0 und 0,2 liegen")
		}
	}
	return nil
}

func setupFaceThresholds(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_face_thresholds (
 id INTEGER PRIMARY KEY CHECK(id=1),
 assignment_similarity REAL NOT NULL DEFAULT .55 CHECK(assignment_similarity BETWEEN .4 AND .7),
 assignment_margin REAL NOT NULL DEFAULT .08 CHECK(assignment_margin BETWEEN 0 AND .2),
 reconcile_similarity REAL NOT NULL DEFAULT .62 CHECK(reconcile_similarity BETWEEN .4 AND .7),
 reconcile_margin REAL NOT NULL DEFAULT .10 CHECK(reconcile_margin BETWEEN 0 AND .2),
 suggestion_similarity REAL NOT NULL DEFAULT .45 CHECK(suggestion_similarity BETWEEN .4 AND .7),
 suggestion_margin REAL NOT NULL DEFAULT 0 CHECK(suggestion_margin BETWEEN 0 AND .2)
 ); INSERT OR IGNORE INTO photo_face_thresholds(id) VALUES(1)`)
	return err
}

func (l *Library) FaceThresholds(ctx context.Context) (FaceThresholds, error) {
	var v FaceThresholds
	err := l.index.db.QueryRowContext(ctx, `SELECT assignment_similarity,assignment_margin,reconcile_similarity,reconcile_margin,suggestion_similarity,suggestion_margin FROM photo_face_thresholds WHERE id=1`).Scan(&v.AssignmentSimilarity, &v.AssignmentMargin, &v.ReconcileSimilarity, &v.ReconcileMargin, &v.SuggestionSimilarity, &v.SuggestionMargin)
	return v, err
}

// Settings and cache invalidation commit together. Rejected pairs remain durable.
// The worker rebuilds proposals in bounded batches using existing embeddings.
func (l *Library) SetFaceThresholds(ctx context.Context, v FaceThresholds) error {
	if err := v.Validate(); err != nil {
		return err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE photo_face_thresholds SET assignment_similarity=?,assignment_margin=?,reconcile_similarity=?,reconcile_margin=?,suggestion_similarity=?,suggestion_margin=? WHERE id=1 AND (assignment_similarity<>? OR assignment_margin<>? OR reconcile_similarity<>? OR reconcile_margin<>? OR suggestion_similarity<>? OR suggestion_margin<>?)`, v.AssignmentSimilarity, v.AssignmentMargin, v.ReconcileSimilarity, v.ReconcileMargin, v.SuggestionSimilarity, v.SuggestionMargin, v.AssignmentSimilarity, v.AssignmentMargin, v.ReconcileSimilarity, v.ReconcileMargin, v.SuggestionSimilarity, v.SuggestionMargin)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed > 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM photo_face_merge_suggestions WHERE rejected=0`); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE photo_face_reconciliation SET pending=1,running=0,generation=generation+1,cursor=0 WHERE id=1`); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	l.faceRuntime.thresholds = v
	return nil
}

// Called with faceRuntime.mu held, including synthetic matching fixtures.
func (l *Library) matchingThresholds() FaceThresholds {
	if l.faceRuntime.thresholds.AssignmentSimilarity == 0 {
		return DefaultFaceThresholds()
	}
	return l.faceRuntime.thresholds
}

// A zero review margin allows alternatives. A positive margin requires the best
// candidate to beat the runner-up, so ambiguous groups stay out of the queue.
func reviewCandidateAllowed(candidates []facePersonCandidate, index int, margin float64) bool {
	if margin == 0 {
		return true
	}
	if index != 0 {
		return false
	}
	return len(candidates) < 2 || candidates[0].score-candidates[1].score >= margin
}
