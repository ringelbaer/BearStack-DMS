// Datei stellt die oeffentliche Tag-API und gemeinsame Tag-Hilfsfunktionen fuer Fotos bereit.
package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"bearstack/internal/tagutil"
)

const defaultPhotoTagColor = tagutil.DefaultColor

func tagsFromJSON(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" || value == "null" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return nil
	}
	return cleanPhotoTags(tags)
}

func cleanPhotoTags(values []string) []string {
	return tagutil.Normalize(values)
}

func (l *Library) ListTags(ctx context.Context, includeAdminOnly ...bool) ([]Tag, error) {
	if l == nil {
		return nil, nil
	}
	includeRestricted := len(includeAdminOnly) > 0 && includeAdminOnly[0]
	return l.index.listTags(ctx, includeRestricted)
}

func (l *Library) SaveTag(ctx context.Context, name string, colorValues ...string) (Tag, error) {
	name, err := cleanSinglePhotoTag(name)
	if err != nil {
		return Tag{}, err
	}
	color := defaultPhotoTagColor
	updateColor := len(colorValues) > 0
	if updateColor {
		color = tagutil.NormalizeColorOr(colorValues[0], defaultPhotoTagColor)
	}
	if l == nil {
		return Tag{Name: name, Color: color}, nil
	}
	return l.index.saveTag(ctx, name, color, updateColor)
}

func (l *Library) GetTag(ctx context.Context, name string) (Tag, error) {
	name, err := cleanSinglePhotoTag(name)
	if err != nil {
		return Tag{}, err
	}
	if l == nil {
		return Tag{Name: name, Color: defaultPhotoTagColor}, nil
	}
	return l.index.getTag(ctx, name)
}

func (l *Library) RenameTag(ctx context.Context, oldName, newName string, colorValues ...string) (Tag, error) {
	oldName, err := cleanSinglePhotoTag(oldName)
	if err != nil {
		return Tag{}, err
	}
	newName, err = cleanSinglePhotoTag(newName)
	if err != nil {
		return Tag{}, err
	}
	if oldName == newName {
		return l.SaveTag(ctx, newName, colorValues...)
	}
	color := defaultPhotoTagColor
	updateColor := len(colorValues) > 0
	if updateColor {
		color = tagutil.NormalizeColorOr(colorValues[0], defaultPhotoTagColor)
	}
	if l == nil {
		return Tag{Name: newName, Color: color}, nil
	}
	tag, refresh, err := l.index.renameTag(ctx, oldName, newName, color, updateColor)
	if err != nil {
		return Tag{}, err
	}
	l.refreshPhotoTagSearchPaths(ctx, refresh)
	return tag, nil
}

type photoTagSearchRefresh struct {
	media  []string
	folder []string
	blog   []string
}

func (l *Library) DeleteTag(ctx context.Context, name string) (Tag, error) {
	name, err := cleanSinglePhotoTag(name)
	if err != nil {
		return Tag{}, err
	}
	if l == nil {
		return Tag{Name: name}, nil
	}
	tag, refresh, err := l.index.deleteTag(ctx, name)
	if err != nil {
		return Tag{}, err
	}
	l.refreshPhotoTagSearchPaths(ctx, refresh)
	return tag, nil
}

func (l *Library) refreshPhotoTagSearchPaths(ctx context.Context, refresh photoTagSearchRefresh) {
	for _, path := range refresh.media {
		if err := l.refreshMediaSearch(ctx, path); err != nil {
			l.logWriteError("photo media search refresh failed", path, err)
		}
	}
	for _, path := range refresh.folder {
		if err := l.refreshFolderSearch(ctx, path); err != nil {
			l.logWriteError("photo folder search refresh failed", path, err)
		}
	}
	for _, path := range refresh.blog {
		if err := l.refreshBlogSearch(ctx, path); err != nil {
			l.logWriteError("photo blog search refresh failed", path, err)
		}
	}
}

func cleanSinglePhotoTag(name string) (string, error) {
	tags := cleanPhotoTags([]string{name})
	if len(tags) == 0 {
		return "", errors.New("Foto-Tagname fehlt")
	}
	return tags[0], nil
}

func photoTagExistsTx(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(1) FROM photo_tags WHERE name = ?) +
			(SELECT COUNT(1) FROM media_tag_index WHERE tag = ?) +
			(SELECT COUNT(1) FROM folder_tag_index WHERE tag = ?) +
			(SELECT COUNT(1) FROM blog_tag_index WHERE tag = ?)`,
		name, name, name, name,
	).Scan(&count)
	return count > 0, err
}

func renamePhotoTagValuesTx(ctx context.Context, tx *sql.Tx, table, pathColumn, oldName, newName string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+pathColumn+", tags FROM "+table+" WHERE tags <> '' AND tags <> '[]'")
	if err != nil {
		return nil, err
	}
	type update struct {
		path string
		tags string
	}
	var updates []update
	for rows.Next() {
		var path, raw string
		if err := rows.Scan(&path, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		next, changed := renamePhotoTagJSON(raw, oldName, newName)
		if changed {
			updates = append(updates, update{path: path, tags: next})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET tags = ? WHERE "+pathColumn+" = ?", update.tags, update.path); err != nil {
			return nil, err
		}
	}
	paths := make([]string, len(updates))
	for i, update := range updates {
		paths[i] = update.path
	}
	return paths, nil
}

func removePhotoTagValuesTx(ctx context.Context, tx *sql.Tx, table, pathColumn, name string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+pathColumn+", tags FROM "+table+" WHERE tags <> '' AND tags <> '[]'")
	if err != nil {
		return nil, err
	}
	type update struct {
		path string
		tags string
	}
	var updates []update
	for rows.Next() {
		var path, raw string
		if err := rows.Scan(&path, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		next, changed := removePhotoTagJSON(raw, name)
		if changed {
			updates = append(updates, update{path: path, tags: next})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET tags = ? WHERE "+pathColumn+" = ?", update.tags, update.path); err != nil {
			return nil, err
		}
	}
	paths := make([]string, len(updates))
	for i, update := range updates {
		paths[i] = update.path
	}
	return paths, nil
}

func renamePhotoTagJSON(raw, oldName, newName string) (string, bool) {
	tags := tagsFromJSON(raw)
	changed := false
	for i, tag := range tags {
		if tag == oldName {
			tags[i] = newName
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	return tagsJSONString(tags), true
}

func removePhotoTagJSON(raw, name string) (string, bool) {
	tags := tagsFromJSON(raw)
	next := make([]string, 0, len(tags))
	changed := false
	for _, tag := range tags {
		if tag == name {
			changed = true
			continue
		}
		next = append(next, tag)
	}
	if !changed {
		return raw, false
	}
	return tagsJSONString(next), true
}

func renamePhotoTagIndexTx(ctx context.Context, tx *sql.Tx, table, pathColumn, oldName, newName string) error {
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO "+table+"("+pathColumn+", tag) SELECT "+pathColumn+", ? FROM "+table+" WHERE tag = ?", newName, oldName); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE tag = ?", oldName)
	return err
}

func syncTagIndexTx(ctx context.Context, tx *sql.Tx, table, pathColumn, path string, tags []string) error {
	tags = cleanPhotoTags(tags)
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE "+pathColumn+" = ?", path); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	if err := insertPhotoTagsTx(tx, tags); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO "+table+"("+pathColumn+", tag) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, tag := range tags {
		if _, err := stmt.ExecContext(ctx, path, tag); err != nil {
			return err
		}
	}
	return nil
}

func insertPhotoTagsTx(tx *sql.Tx, tags []string) error {
	tags = cleanPhotoTags(tags)
	if len(tags) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.Prepare(`INSERT INTO photo_tags(name, created_at, updated_at) VALUES (?, ?, ?) ON CONFLICT(name) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, tag := range tags {
		if _, err := stmt.Exec(tag, now, now); err != nil {
			return err
		}
	}
	return nil
}
