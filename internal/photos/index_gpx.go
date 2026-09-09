package photos

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Inventory contains only file metadata. Parsing remains lazy and uses the
// shared byte/point-bounded GPX cache, so index scans never decode tracks.
type GPXFile struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Bytes     int64     `json:"bytes"`
	Modified  time.Time `json:"modified"`
	adminOnly bool
	id        int64
}

type GPXFilePage struct {
	Files          []GPXFile `json:"tracks"`
	Cursor         string    `json:"cursor"`
	PreviousCursor string    `json:"previous_cursor"`
	HasPrevious    bool      `json:"has_previous"`
	HasNext        bool      `json:"has_next"`
	Ready          bool      `json:"ready"`
}

func setupGPXIndex(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS gpx_index (
		id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL UNIQUE, directory TEXT NOT NULL, name TEXT NOT NULL,
        size_bytes INTEGER NOT NULL, mod_time_unix_nano INTEGER NOT NULL,
		admin_only INTEGER NOT NULL DEFAULT 0, seen INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_gpx_directory_id ON gpx_index(directory,id)`)
	return err
}

func (s *photoIndexStore) replaceGPXDirectory(ctx context.Context, directory string, files []GPXFile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE gpx_index SET seen=0 WHERE directory=?`, directory); err != nil {
		return err
	}
	if len(files) > 0 {
		insert, err := tx.PrepareContext(ctx, `INSERT INTO gpx_index(path,directory,name,size_bytes,mod_time_unix_nano,admin_only) VALUES(?,?,?,?,?,?)
            ON CONFLICT(path) DO UPDATE SET name=excluded.name,size_bytes=excluded.size_bytes,
            mod_time_unix_nano=excluded.mod_time_unix_nano,admin_only=excluded.admin_only,seen=1`)
		if err != nil {
			return err
		}
		defer insert.Close()
		for _, file := range files {
			if _, err = insert.ExecContext(ctx, file.Path, directory, file.Name, file.Bytes, file.Modified.UnixNano(), boolInt(file.adminOnly)); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gpx_index WHERE directory=? AND seen=0`, directory); err != nil {
		return err
	}
	return tx.Commit()
}

// GPXFiles uses an ID cursor, so deep pages do not scan or sort photo metadata.
// A second, current marker check prevents newly private folders leaking names.
func (l *Library) GPXFiles(ctx context.Context, path, cursor string, includeAdminOnly bool) (GPXFilePage, error) {
	return l.gpxFiles(ctx, path, cursor, includeAdminOnly, false)
}

func (l *Library) GPXFilesBefore(ctx context.Context, path, cursor string, includeAdminOnly bool) (GPXFilePage, error) {
	return l.gpxFiles(ctx, path, cursor, includeAdminOnly, true)
}

func (l *Library) gpxFiles(ctx context.Context, path, cursor string, includeAdminOnly, before bool) (GPXFilePage, error) {
	result := GPXFilePage{Files: []GPXFile{}}
	rel, err := CleanPath(path)
	if err != nil {
		return result, err
	}
	var after int64
	if cursor != "" {
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 0 {
			return result, ErrMapCursor
		}
	}
	private, err := l.FolderAdminOnly(rel)
	if err != nil {
		return result, err
	}
	if private && !includeAdminOnly {
		return result, errAdminOnly
	}
	if !l.index.available() {
		return result, ErrMapIndexUnavailable
	}
	coverage, err := l.indexedListingPathState(ctx, rel, includeAdminOnly, false)
	if err != nil {
		return result, err
	}
	where := "id > ?"
	order := "id"
	if before {
		where = "id < ?"
		order = "id DESC"
	}
	args := []any{after}
	scanWhere := "1=1"
	var scanArgs []any
	if rel != "" {
		start, end := prefixRange(rel + "/")
		where += " AND path>=? AND path<?"
		args = append(args, start, end)
		scanWhere = "(path=? OR (path>=? AND path<?))"
		scanArgs = []any{rel, start, end}
	}
	var pending int
	if err = l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_folder_scan WHERE gpx_scanned=0 AND `+scanWhere+`)`, scanArgs...).Scan(&pending); err != nil {
		return result, err
	}
	result.Ready = coverage.Covered && pending == 0
	if !includeAdminOnly {
		where += " AND admin_only=0"
	}
	// Read a bounded candidate batch. The cursor advances over newly hidden
	// files as well, allowing callers to continue without an unbounded scan.
	rows, err := l.index.db.QueryContext(ctx, `SELECT id,path,name,size_bytes,mod_time_unix_nano FROM gpx_index WHERE `+where+` ORDER BY `+order+` LIMIT 33`, args...)
	if err != nil {
		return result, err
	}
	var candidates []GPXFile
	for rows.Next() {
		var item GPXFile
		var modified int64
		if err = rows.Scan(&item.id, &item.Path, &item.Name, &item.Bytes, &modified); err != nil {
			rows.Close()
			return result, err
		}
		item.Modified = time.Unix(0, modified).UTC()
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if before {
		result.HasPrevious = len(candidates) > 32
		result.HasNext = cursor != ""
		candidates = candidates[:min(len(candidates), 32)]
		slices.Reverse(candidates)
	} else {
		result.HasNext = len(candidates) > 32
		result.HasPrevious = cursor != ""
		candidates = candidates[:min(len(candidates), 32)]
	}
	if len(candidates) > 0 {
		result.PreviousCursor = strconv.FormatInt(candidates[0].id, 10)
	}
	visibility := map[string]bool{}
	for _, item := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Cursor = strconv.FormatInt(item.id, 10)
		directory := parentPath(item.Path)
		hidden, known := visibility[directory]
		if !known {
			hidden, err = l.FolderAdminOnly(directory)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return result, err
				}
				hidden = true
			}
			visibility[directory] = hidden
		}
		if hidden && !includeAdminOnly {
			continue
		}
		if !strings.EqualFold(filepath.Ext(item.Path), ".gpx") {
			continue
		}
		result.Files = append(result.Files, item)
	}
	return result, nil
}

var ErrMapCursor = errors.New("invalid map cursor")
