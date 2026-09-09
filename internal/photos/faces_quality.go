package photos

import (
	"context"
	"database/sql"
)

// Unknown quality preserves embeddings produced by older compatible services.
// Explicit favorites can override measured reference quality; privacy cannot.
func setupFaceQuality(ctx context.Context, db *sql.DB) error {
	for _, column := range []struct{ name, definition string }{
		{"reference_eligible", "INTEGER CHECK(reference_eligible IN (0,1))"},
		{"face_pixels", "REAL"},
		{"sharpness", "REAL"},
	} {
		if _, err := ensurePhotoColumn(ctx, db, "photo_faces", column.name, "ALTER TABLE photo_faces ADD COLUMN "+column.name+" "+column.definition); err != nil {
			return err
		}
	}
	_, err := db.ExecContext(ctx, `UPDATE photo_face_reference_settings SET pending=1,cursor=0 WHERE id=1`)
	return err
}
