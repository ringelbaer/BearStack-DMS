package photos

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"
)

func setupPersonTags(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS person_tag_index (
 person_id INTEGER NOT NULL, tag TEXT NOT NULL, PRIMARY KEY(person_id,tag)) WITHOUT ROWID;
 CREATE INDEX IF NOT EXISTS idx_person_tags_tag ON person_tag_index(tag,person_id);
 CREATE TRIGGER IF NOT EXISTS photo_person_tags_delete AFTER DELETE ON photo_people BEGIN
 DELETE FROM person_tag_index WHERE person_id=old.id; END`)
	return err
}

func mergePersonTagsTx(ctx context.Context, tx *sql.Tx, source, target int64) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_tag_index(person_id,tag) SELECT ?,tag FROM person_tag_index WHERE person_id=?`, target, source)
	return err
}

func (l *Library) personTags(ctx context.Context, id int64) ([]string, error) {
	rows, err := l.index.db.QueryContext(ctx, `SELECT tag FROM person_tag_index WHERE person_id=? ORDER BY tag`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// Tags belong to the person group, independently of the tags on its photos.
func (l *Library) SetPersonTags(ctx context.Context, id int64, tags []string) ([]string, error) {
	if id <= 0 || len(tags) > 100 {
		return nil, errors.New("ungültige Personen-Tags")
	}
	tags = cleanPhotoTags(tags)
	for _, tag := range tags {
		if len(tag) > 4096 || !utf8.ValidString(tag) {
			return nil, errors.New("ungültiger Personen-Tag")
		}
	}
	if err := l.refreshPersonIDsVisibility(ctx, id); err != nil {
		return nil, err
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM photo_people p WHERE p.id=? AND EXISTS(
 SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=p.id AND f.ignored=0 AND m.admin_only=0)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if err := insertPhotoTagsTx(tx, tags); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM person_tag_index WHERE person_id=?`, id); err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO person_tag_index(person_id,tag) VALUES(?,?)`, id, tag); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tags, nil
}
