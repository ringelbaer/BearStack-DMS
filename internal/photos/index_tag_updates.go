package photos

import (
	"context"
	"database/sql"

	"bearstack/internal/sqlutil"
	"bearstack/internal/tagutil"
)

// UpdateMediaTagsContext applies additions or removals to the current tags.
// Validate files before taking the database writer lock; never carry tag
// snapshots from filesystem/metadata reads into the mutation transaction.
func (l *Library) UpdateMediaTagsContext(ctx context.Context, paths, tags []string, add bool) (int, error) {
	cleanPaths := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		media, err := l.MediaContext(ctx, path)
		if err != nil {
			return 0, err
		}
		if _, ok := seen[media.Path]; ok {
			continue
		}
		seen[media.Path] = struct{}{}
		cleanPaths = append(cleanPaths, media.Path)
	}
	return l.index.updateMediaTags(ctx, cleanPaths, cleanPhotoTags(tags), add)
}

func (s *photoIndexStore) updateMediaTags(ctx context.Context, paths, tags []string, add bool) (int, error) {
	if !s.available() || len(paths) == 0 || len(tags) == 0 {
		return 0, nil
	}
	tx, err := s.beginTagWrite(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	updated := 0
	for start := 0; start < len(paths); start += searchWriteChunkSize {
		end := min(start+searchWriteChunkSize, len(paths))
		args := make([]any, 0, end-start)
		for _, path := range paths[start:end] {
			args = append(args, path)
		}
		rows, err := tx.QueryContext(ctx, `SELECT path, tags FROM media_index WHERE path IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
		if err != nil {
			return 0, err
		}
		current := make(map[string][]string, len(args))
		for rows.Next() {
			var path, raw string
			if err := rows.Scan(&path, &raw); err != nil {
				_ = rows.Close()
				return 0, err
			}
			current[path] = tagsFromJSON(raw)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
		for _, path := range paths[start:end] {
			previous, ok := current[path]
			if !ok {
				return 0, sql.ErrNoRows
			}
			next := tagutil.Merge(previous, tags)
			if !add {
				next = tagutil.Remove(previous, tags)
			}
			if tagutil.EqualNormalized(previous, next) {
				continue
			}
			if err := setMediaTagsTx(ctx, tx, path, next); err != nil {
				return 0, err
			}
			updated++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}
