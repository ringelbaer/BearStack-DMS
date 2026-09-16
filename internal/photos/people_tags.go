package photos

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"bearstack/internal/sqlutil"
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

// AddPeopleTags augments an entire selection atomically. Repeating a request is
// harmless; validation and writes share a transaction after visibility refresh.
func (l *Library) AddPeopleTags(ctx context.Context, ids []int64, tags []string) (int, error) {
	if len(ids) == 0 || len(ids) > 500 || len(tags) > 100 {
		return 0, errors.New("ungültige Personen- oder Tag-Auswahl")
	}
	tags = cleanPhotoTags(tags)
	if len(tags) == 0 {
		return 0, errors.New("Bitte mindestens einen Tag auswählen")
	}
	for _, tag := range tags {
		if len(tag) > 4096 || !utf8.ValidString(tag) {
			return 0, errors.New("ungültiger Personen-Tag")
		}
	}
	unique := make([]int64, 0, len(ids))
	args := make([]any, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("ungültige Personen-ID")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
			args = append(args, id)
		}
	}
	if err := l.refreshPersonIDsVisibility(ctx, unique...); err != nil {
		return 0, err
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	filter := `p.id IN (` + sqlutil.Placeholders(len(unique)) + `)`
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM photo_people p WHERE `+filter+` AND `+visiblePersonSQL, args...).Scan(&count); err != nil {
		return 0, err
	}
	if count != len(unique) {
		return 0, sql.ErrNoRows
	}
	// Bound each resulting selection as well as the incoming request.
	limitArgs := append([]any{}, args...)
	for _, tag := range tags {
		limitArgs = append(limitArgs, tag)
	}
	limitArgs = append(limitArgs, len(tags))
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM photo_people p WHERE `+filter+` AND
 (SELECT count(*) FROM person_tag_index WHERE person_id=p.id AND tag NOT IN (`+sqlutil.Placeholders(len(tags))+`))+?>100`, limitArgs...).Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, errors.New("Höchstens 100 Tags pro Person möglich")
	}
	if err := insertPhotoTagsTx(tx, tags); err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO person_tag_index(person_id,tag) SELECT p.id,? FROM photo_people p WHERE `+filter)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, tag := range tags {
		if _, err := stmt.ExecContext(ctx, append([]any{tag}, args...)...); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(unique), nil
}
