package photos

import (
	"context"
	"database/sql"
)

// Older faces deliberately remain unknown: current group size cannot recover
// the original recognition decision after merges, moves, and deletions.
func setupFaceAssignmentStats(ctx context.Context, db *sql.DB) error {
	if _, err := ensurePhotoColumn(ctx, db, "photo_faces", "recognition_assignment", "ALTER TABLE photo_faces ADD COLUMN recognition_assignment TEXT NOT NULL DEFAULT '' CHECK(recognition_assignment IN ('','matched','new'))"); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_face_recognition_assignment ON photo_faces(recognition_assignment) WHERE ignored=0`)
	return err
}
