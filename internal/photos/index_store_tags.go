// Datei enthaelt Store-Operationen fuer Foto-Tags, Tag-Umbenennung und Tag-Index-Synchronisierung.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *photoIndexStore) listTags(ctx context.Context, includeRestricted bool) ([]Tag, error) {
	if !s.available() {
		return nil, nil
	}
	manualTags := `SELECT name AS tag, 0 AS count FROM photo_tags`
	mediaTags := `SELECT tag, COUNT(DISTINCT media_path) AS count FROM media_tag_index GROUP BY tag`
	folderTags := `SELECT tag, COUNT(DISTINCT folder_path) AS count FROM folder_tag_index GROUP BY tag`
	blogTags := `SELECT tag, COUNT(DISTINCT blog_path) AS count FROM blog_tag_index GROUP BY tag`
	if !includeRestricted {
		manualTags = `SELECT pt.name AS tag, 0 AS count
			FROM photo_tags pt
			WHERE NOT EXISTS (SELECT 1 FROM media_tag_index mti WHERE mti.tag = pt.name)
				AND NOT EXISTS (SELECT 1 FROM folder_tag_index fti WHERE fti.tag = pt.name)
				AND NOT EXISTS (SELECT 1 FROM blog_tag_index bti WHERE bti.tag = pt.name)`
		mediaTags = `SELECT mti.tag, COUNT(DISTINCT mti.media_path) AS count
			FROM media_tag_index mti
			JOIN media_index mi ON mi.path = mti.media_path
			WHERE mi.admin_only = 0
			GROUP BY mti.tag`
		folderTags = `SELECT fti.tag, COUNT(DISTINCT fti.folder_path) AS count
			FROM folder_tag_index fti
			JOIN folder_index fi ON fi.path = fti.folder_path
			WHERE fi.admin_only = 0
			GROUP BY fti.tag`
		blogTags = `SELECT bti.tag, COUNT(DISTINCT bti.blog_path) AS count
			FROM blog_tag_index bti
			JOIN blog_index bi ON bi.path = bti.blog_path
			WHERE bi.admin_only = 0
			GROUP BY bti.tag`
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH tag_counts AS (
			SELECT tag, SUM(count) AS count
			FROM (
			`+manualTags+`
			UNION ALL
			`+mediaTags+`
			UNION ALL
			`+folderTags+`
			UNION ALL
			`+blogTags+`
			)
			GROUP BY tag
		)
		SELECT tag_counts.tag, COALESCE(photo_tags.color, ?), tag_counts.count
		FROM tag_counts
		LEFT JOIN photo_tags ON photo_tags.name = tag_counts.tag
		ORDER BY tag_counts.tag`, defaultPhotoTagColor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Color, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *photoIndexStore) saveTag(ctx context.Context, name, color string, updateColor bool) (Tag, error) {
	if !s.available() {
		return Tag{Name: name, Color: color}, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if updateColor {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO photo_tags(name, color, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET color = excluded.color, updated_at = excluded.updated_at`,
			name, color, now, now,
		); err != nil {
			return Tag{}, err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO photo_tags(name, color, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET updated_at = excluded.updated_at`,
			name, color, now, now,
		); err != nil {
			return Tag{}, err
		}
	}
	return s.getTag(ctx, name)
}

func (s *photoIndexStore) getTag(ctx context.Context, name string) (Tag, error) {
	if !s.available() {
		return Tag{Name: name, Color: defaultPhotoTagColor}, nil
	}
	return getPhotoTag(ctx, s.db, name)
}

func getPhotoTag(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string) (Tag, error) {
	var tag Tag
	err := query.QueryRowContext(ctx, `
		WITH tag_counts AS (
			SELECT tag, SUM(count) AS count
			FROM (
			SELECT name AS tag, 0 AS count FROM photo_tags WHERE name = ?
			UNION ALL
			SELECT tag, COUNT(DISTINCT media_path) AS count FROM media_tag_index WHERE tag = ? GROUP BY tag
			UNION ALL
			SELECT tag, COUNT(DISTINCT folder_path) AS count FROM folder_tag_index WHERE tag = ? GROUP BY tag
			UNION ALL
			SELECT tag, COUNT(DISTINCT blog_path) AS count FROM blog_tag_index WHERE tag = ? GROUP BY tag
			)
			GROUP BY tag
		)
		SELECT tag_counts.tag, COALESCE(photo_tags.color, ?), tag_counts.count
		FROM tag_counts
		LEFT JOIN photo_tags ON photo_tags.name = tag_counts.tag`,
		name, name, name, name, defaultPhotoTagColor,
	).Scan(&tag.Name, &tag.Color, &tag.Count)
	if err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func (s *photoIndexStore) renameTag(ctx context.Context, oldName, newName, color string, updateColor bool) (Tag, error) {
	if !s.available() {
		return Tag{Name: newName, Color: color}, nil
	}
	tx, err := s.beginTagWrite(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer tx.Rollback()

	exists, err := photoTagExistsTx(ctx, tx, oldName)
	if err != nil {
		return Tag{}, err
	}
	if !exists {
		return Tag{}, sql.ErrNoRows
	}

	if !updateColor {
		if err := tx.QueryRowContext(ctx, `SELECT color FROM photo_tags WHERE name = ?`, oldName).Scan(&color); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Tag{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if updateColor {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO photo_tags(name, color, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET color = excluded.color, updated_at = excluded.updated_at`,
			newName, color, now, now,
		); err != nil {
			return Tag{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO photo_tags(name, color, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET updated_at = excluded.updated_at`,
			newName, color, now, now,
		); err != nil {
			return Tag{}, err
		}
	}

	mediaPaths, err := renamePhotoTagValuesTx(ctx, tx, "media_index", "media_tag_index", "media_path", oldName, newName)
	if err != nil {
		return Tag{}, err
	}
	folderPaths, err := renamePhotoTagValuesTx(ctx, tx, "folder_index", "folder_tag_index", "folder_path", oldName, newName)
	if err != nil {
		return Tag{}, err
	}
	blogPaths, err := renamePhotoTagValuesTx(ctx, tx, "blog_index", "blog_tag_index", "blog_path", oldName, newName)
	if err != nil {
		return Tag{}, err
	}
	for _, index := range []struct {
		table      string
		pathColumn string
	}{
		{"media_tag_index", "media_path"},
		{"folder_tag_index", "folder_path"},
		{"blog_tag_index", "blog_path"},
	} {
		if err := renamePhotoTagIndexTx(ctx, tx, index.table, index.pathColumn, oldName, newName); err != nil {
			return Tag{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_tags WHERE name = ?`, oldName); err != nil {
		return Tag{}, err
	}
	if err := refreshPhotoTagSearchPathsTx(ctx, tx, photoTagSearchRefresh{media: mediaPaths, folder: folderPaths, blog: blogPaths}); err != nil {
		return Tag{}, err
	}
	tag, err := getPhotoTag(ctx, tx, newName)
	if err != nil {
		return Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func (s *photoIndexStore) deleteTag(ctx context.Context, name string) (Tag, error) {
	if !s.available() {
		return Tag{Name: name}, nil
	}
	tx, err := s.beginTagWrite(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer tx.Rollback()
	tag, err := getPhotoTag(ctx, tx, name)
	if err != nil {
		return Tag{}, err
	}

	mediaPaths, err := removePhotoTagValuesTx(ctx, tx, "media_index", "media_tag_index", "media_path", name)
	if err != nil {
		return Tag{}, err
	}
	folderPaths, err := removePhotoTagValuesTx(ctx, tx, "folder_index", "folder_tag_index", "folder_path", name)
	if err != nil {
		return Tag{}, err
	}
	blogPaths, err := removePhotoTagValuesTx(ctx, tx, "blog_index", "blog_tag_index", "blog_path", name)
	if err != nil {
		return Tag{}, err
	}
	for _, index := range []struct {
		table string
	}{
		{"media_tag_index"},
		{"folder_tag_index"},
		{"blog_tag_index"},
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+index.table+" WHERE tag = ?", name); err != nil {
			return Tag{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_tags WHERE name = ?`, name); err != nil {
		return Tag{}, err
	}
	if err := refreshPhotoTagSearchPathsTx(ctx, tx, photoTagSearchRefresh{media: mediaPaths, folder: folderPaths, blog: blogPaths}); err != nil {
		return Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

// Reserve the SQLite writer before reading tags. A deferred read transaction
// cannot safely upgrade after another writer commits (SQLITE_BUSY_SNAPSHOT).
// This zero-row UPDATE takes the database write lock without changing rows;
// it coordinates other connections and the index worker as well as HTTP writes.
func (s *photoIndexStore) beginTagWrite(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_tags SET name = name WHERE 0`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}
