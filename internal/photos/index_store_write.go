// Datei enthaelt Store-Schreibpfade fuer Medien, Ordner, Blogs und deren Suchindexeintraege.
package photos

import (
	"context"
	"strings"
	"time"
)

func (s *photoIndexStore) cachedMedia(base Media) (Media, bool) {
	if !s.available() {
		return Media{}, false
	}
	var row cachedMediaRow
	err := s.db.QueryRow(`SELECT `+mediaIndexColumns(``)+`
		FROM media_index
		WHERE path = ?`, base.Path).Scan(
		&row.Path,
		&row.Name,
		&row.Directory,
		&row.Type,
		&row.MIMEType,
		&row.SizeBytes,
		&row.ModTimeUnixNano,
		&row.CapturedAt,
		&row.Width,
		&row.Height,
		&row.Orientation,
		&row.Camera,
		&row.Lens,
		&row.Rating,
		&row.Latitude,
		&row.Longitude,
		&row.Keywords,
		&row.Tags,
		&row.Faces,
		&row.XMPFingerprint,
		&row.AdminOnly,
	)
	if err != nil || row.SizeBytes != base.SizeBytes || row.ModTimeUnixNano != base.ModTime.UnixNano() {
		return Media{}, false
	}
	return cachedMediaFromRow(base, row)
}

func (s *photoIndexStore) saveMediaBatchWithExisting(ctx context.Context, items []Media, existing map[string]cachedMediaRow) error {
	if !s.available() || len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	replaceUnknown := existing == nil
	byPath := make(map[string]*Media, len(items))
	for i := range items {
		byPath[items[i].Path] = &items[i]
	}
	searchRows := make([]mediaSearchRow, 0, len(items))
	for start := 0; start < len(items); start += mediaWriteChunkSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + mediaWriteChunkSize
		if end > len(items) {
			end = len(items)
		}
		sqlText, args := mediaUpsertSQL(items[start:end], now)
		rows, err := tx.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var path string
			var rowID int64
			var rawTags string
			if err := rows.Scan(&path, &rowID, &rawTags); err != nil {
				_ = rows.Close()
				return err
			}
			media, ok := byPath[path]
			if !ok {
				continue
			}
			media.Tags = tagsFromJSON(rawTags)
			_, replace := existing[path]
			searchRows = append(searchRows, mediaSearchRow{
				RowID:      rowID,
				Path:       media.Path,
				SearchText: searchText(*media),
				Tags:       media.Tags,
				Replace:    replaceUnknown || replace,
			})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	if err := replaceMediaSearchRows(ctx, tx, searchRows); err != nil {
		return err
	}
	for _, row := range searchRows {
		if len(row.Tags) == 0 {
			continue
		}
		if err := syncTagIndexTx(ctx, tx, "media_tag_index", "media_path", row.Path, row.Tags); err != nil {
			return err
		}
	}
	if err := s.queueChangedMediaThumbnailsTx(ctx, tx, items, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *photoIndexStore) refreshMediaSearch(ctx context.Context, path string) error {
	if !s.available() {
		return nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaIndexColumns(``)+`, rowid FROM media_index WHERE path = ?`, path)
	media, rowID, err := scanIndexedMediaWithRowID(row)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM media_search WHERE rowid = ?`, rowID); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO media_search(rowid, path, search_text) VALUES (?, ?, ?)`, rowID, media.Path, searchText(media))
	return err
}

func (s *photoIndexStore) setMediaTags(ctx context.Context, path string, tags []string) error {
	if !s.available() {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE media_index SET tags = ?, indexed_at = ? WHERE path = ?`, tagsJSONString(tags), time.Now().UTC().Format(time.RFC3339Nano), path); err != nil {
		return err
	}
	s.syncMediaTags(path, tags)
	return s.refreshMediaSearch(ctx, path)
}

func (s *photoIndexStore) setFolderTags(ctx context.Context, folder Folder) error {
	if !s.available() {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO folder_index(path, parent, name, media_count, public_media_count, recursive_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count, dir_count, mod_time_unix_nano, order_mode, tags, admin_only, indexed_at)
		VALUES (?, ?, ?, 0, 0, 0, 0, 0, 0, 0, ?, '', ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			tags = excluded.tags,
			admin_only = excluded.admin_only,
			indexed_at = excluded.indexed_at`,
		folder.Path,
		parentPath(folder.Path),
		folder.Name,
		folder.ModTime.UnixNano(),
		tagsJSONString(folder.Tags),
		boolInt(folder.AdminOnly),
		now,
	); err != nil {
		return err
	}
	s.syncFolderTags(folder.Path, folder.Tags)
	return s.refreshFolderSearch(ctx, folder.Path)
}

func (s *photoIndexStore) folderTags(path string) ([]string, bool) {
	if !s.available() {
		return nil, false
	}
	var rawTags string
	if err := s.db.QueryRow(`SELECT tags FROM folder_index WHERE path = ?`, path).Scan(&rawTags); err != nil {
		return nil, false
	}
	return tagsFromJSON(rawTags), true
}

func (s *photoIndexStore) blogTags(path string) ([]string, bool) {
	if !s.available() {
		return nil, false
	}
	var rawTags string
	if err := s.db.QueryRow(`SELECT tags FROM blog_index WHERE path = ?`, path).Scan(&rawTags); err != nil {
		return nil, false
	}
	return tagsFromJSON(rawTags), true
}

func (s *photoIndexStore) saveFolder(folder Folder) error {
	if !s.available() || folder.Path == "" {
		return nil
	}
	directMediaCount := folder.DirectMediaCount
	if directMediaCount == 0 {
		directMediaCount = folder.MediaCount
	}
	publicMediaCount := directMediaCount
	publicRecursiveMediaCount := directMediaCount
	if folder.AdminOnly {
		publicMediaCount = 0
		publicRecursiveMediaCount = 0
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO folder_index(path, parent, name, media_count, public_media_count, recursive_media_count, public_recursive_media_count, recursive_blog_count, public_recursive_blog_count, dir_count, mod_time_unix_nano, order_mode, tags, admin_only, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, '', ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			parent = excluded.parent,
			name = excluded.name,
			media_count = excluded.media_count,
			public_media_count = excluded.public_media_count,
			recursive_media_count = CASE WHEN excluded.recursive_media_count > 0 THEN excluded.recursive_media_count ELSE folder_index.recursive_media_count END,
			public_recursive_media_count = CASE WHEN excluded.public_recursive_media_count > 0 OR excluded.admin_only = 1 THEN excluded.public_recursive_media_count ELSE folder_index.public_recursive_media_count END,
			public_recursive_blog_count = CASE WHEN excluded.admin_only = 1 THEN 0 ELSE folder_index.public_recursive_blog_count END,
			dir_count = excluded.dir_count,
			mod_time_unix_nano = excluded.mod_time_unix_nano,
			tags = excluded.tags,
			admin_only = excluded.admin_only,
			indexed_at = excluded.indexed_at`,
		folder.Path,
		parentPath(folder.Path),
		folder.Name,
		directMediaCount,
		publicMediaCount,
		directMediaCount,
		publicRecursiveMediaCount,
		folder.DirCount,
		folder.ModTime.UnixNano(),
		tagsJSONString(folder.Tags),
		boolInt(folder.AdminOnly),
		now,
	)
	if err != nil {
		return err
	}
	s.syncFolderTags(folder.Path, folder.Tags)
	return s.refreshFolderSearch(context.Background(), folder.Path)
}

func (s *photoIndexStore) refreshFolderSearch(ctx context.Context, path string) error {
	if !s.available() {
		return nil
	}
	var rowID int64
	var name, tags string
	if err := s.db.QueryRowContext(ctx, `SELECT rowid, name, tags FROM folder_index WHERE path = ?`, path).Scan(&rowID, &name, &tags); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM folder_search WHERE rowid = ?`, rowID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO folder_search(rowid, path, search_text) VALUES (?, ?, ?)`, rowID, path, strings.Join([]string{path, name, strings.Join(tagsFromJSON(tags), " ")}, " "))
	return err
}

func (s *photoIndexStore) saveBlogBatch(ctx context.Context, posts []BlogPost) error {
	if !s.available() || len(posts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	upsert, err := tx.PrepareContext(ctx, `
		INSERT INTO blog_index(path, name, directory, date, mod_time_unix_nano, text, tags, admin_only, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			name = excluded.name,
			directory = excluded.directory,
			date = excluded.date,
			mod_time_unix_nano = excluded.mod_time_unix_nano,
			text = excluded.text,
			tags = CASE WHEN blog_index.tags = '[]' OR blog_index.tags = '' THEN excluded.tags ELSE blog_index.tags END,
			admin_only = excluded.admin_only,
			indexed_at = excluded.indexed_at
		RETURNING rowid, tags`)
	if err != nil {
		return err
	}
	defer upsert.Close()
	deleteSearch, err := tx.PrepareContext(ctx, `DELETE FROM blog_search WHERE rowid = ?`)
	if err != nil {
		return err
	}
	defer deleteSearch.Close()
	insertSearch, err := tx.PrepareContext(ctx, `INSERT INTO blog_search(rowid, path, search_text) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insertSearch.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, post := range posts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if post.Path == "" {
			continue
		}
		dateValue := ""
		if post.Date != nil {
			dateValue = post.Date.Format("2006-01-02")
		}
		var rowID int64
		var rawTags string
		if err := upsert.QueryRowContext(ctx,
			post.Path,
			post.Name,
			parentPath(post.Path),
			dateValue,
			post.ModTime.UnixNano(),
			post.Text,
			tagsJSONString(post.Tags),
			boolInt(post.AdminOnly),
			now,
		).Scan(&rowID, &rawTags); err != nil {
			return err
		}
		post.Tags = tagsFromJSON(rawTags)
		if _, err := deleteSearch.ExecContext(ctx, rowID); err != nil {
			return err
		}
		if _, err := insertSearch.ExecContext(ctx, rowID, post.Path, blogSearchText(post)); err != nil {
			return err
		}
		if err := syncTagIndexTx(ctx, tx, "blog_tag_index", "blog_path", post.Path, post.Tags); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *photoIndexStore) refreshBlogSearch(ctx context.Context, path string) error {
	if !s.available() {
		return nil
	}
	var rowID int64
	var name, directory, text, tags string
	if err := s.db.QueryRowContext(ctx, `SELECT rowid, name, directory, text, tags FROM blog_index WHERE path = ?`, path).Scan(&rowID, &name, &directory, &text, &tags); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM blog_search WHERE rowid = ?`, rowID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO blog_search(rowid, path, search_text) VALUES (?, ?, ?)`, rowID, path, strings.Join([]string{MediaTypeBlog, path, name, directory, text, strings.Join(tagsFromJSON(tags), " ")}, " "))
	return err
}
