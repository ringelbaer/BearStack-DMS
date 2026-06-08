// Datei prueft, ob der Fotoindex einen Pfad fuer Such- und Listing-Fallbacks abdeckt.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

type indexedListingPathState struct {
	Covered bool
	Order   string
}

var errFilesystemContentExceedsIndex = errors.New("filesystem content exceeds photo index")

func (l *Library) indexedListingPathState(ctx context.Context, rel string, includeAdminOnly bool, allowContentFallback bool) (indexedListingPathState, error) {
	if l == nil || !l.index.available() {
		return indexedListingPathState{}, nil
	}
	if rel == "" {
		var order string
		err := l.index.db.QueryRowContext(ctx, `SELECT order_mode FROM photo_folder_scan WHERE path = ''`).Scan(&order)
		if errors.Is(err, sql.ErrNoRows) {
			if !allowContentFallback {
				return indexedListingPathState{}, nil
			}
			covered, err := l.indexHasIndexedContentForPath(ctx, rel, includeAdminOnly)
			if err != nil || !covered {
				return indexedListingPathState{}, err
			}
			covered, err = l.indexCoversFilesystemSearchContent(ctx, rel, includeAdminOnly)
			if err != nil || !covered {
				return indexedListingPathState{}, err
			}
			return indexedListingPathState{Covered: true, Order: "ascending_date"}, nil
		}
		if err != nil {
			return indexedListingPathState{}, err
		}
		return indexedListingPathState{Covered: true, Order: normalizeDirectoryOrderMode(order)}, nil
	}
	var adminOnly int
	var order string
	var scannedPath sql.NullString
	err := l.index.db.QueryRowContext(ctx, `
		SELECT fi.admin_only, COALESCE(NULLIF(pfs.order_mode, ''), fi.order_mode), pfs.path
		FROM folder_index fi
		LEFT JOIN photo_folder_scan pfs ON pfs.path = fi.path
		WHERE fi.path = ?`, rel).Scan(&adminOnly, &order, &scannedPath)
	if errors.Is(err, sql.ErrNoRows) {
		return indexedListingPathState{}, nil
	}
	if err != nil {
		return indexedListingPathState{}, err
	}
	if !includeAdminOnly && adminOnly != 0 {
		return indexedListingPathState{}, errAdminOnly
	}
	if !scannedPath.Valid {
		if !allowContentFallback {
			return indexedListingPathState{}, nil
		}
		covered, err := l.indexHasIndexedContentForPath(ctx, rel, includeAdminOnly)
		if err != nil || !covered {
			return indexedListingPathState{}, err
		}
		covered, err = l.indexCoversFilesystemSearchContent(ctx, rel, includeAdminOnly)
		if err != nil || !covered {
			return indexedListingPathState{}, err
		}
	}
	return indexedListingPathState{Covered: true, Order: normalizeDirectoryOrderMode(order)}, nil
}

func (l *Library) indexHasIndexedContentForPath(ctx context.Context, rel string, includeAdminOnly bool) (bool, error) {
	adminFilter := ""
	if !includeAdminOnly {
		adminFilter = " AND admin_only = 0"
	}
	var one int
	var err error
	if rel == "" {
		err = l.index.db.QueryRowContext(ctx, `
			SELECT 1
			WHERE EXISTS (SELECT 1 FROM media_index WHERE 1 = 1`+adminFilter+` LIMIT 1)
				OR EXISTS (SELECT 1 FROM blog_index WHERE 1 = 1`+adminFilter+` LIMIT 1)
				OR EXISTS (SELECT 1 FROM folder_index WHERE 1 = 1`+adminFilter+` LIMIT 1)`).Scan(&one)
	} else {
		start, end := prefixRange(rel + "/")
		err = l.index.db.QueryRowContext(ctx, `
			SELECT 1
			WHERE EXISTS (SELECT 1 FROM media_index WHERE (directory = ? OR (directory >= ? AND directory < ?))`+adminFilter+` LIMIT 1)
				OR EXISTS (SELECT 1 FROM blog_index WHERE (directory = ? OR (directory >= ? AND directory < ?))`+adminFilter+` LIMIT 1)
				OR EXISTS (SELECT 1 FROM folder_index WHERE (path = ? OR (path >= ? AND path < ?))`+adminFilter+` LIMIT 1)`,
			rel,
			start,
			end,
			rel,
			start,
			end,
			rel,
			start,
			end,
		).Scan(&one)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (l *Library) indexCoversFilesystemSearchContent(ctx context.Context, rel string, includeAdminOnly bool) (bool, error) {
	if l == nil || !l.index.available() {
		return false, nil
	}
	indexed, err := l.indexedSearchContentPaths(ctx, rel, includeAdminOnly)
	if err != nil {
		return false, err
	}
	abs, err := l.Resolve(rel)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	err = walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{
		Root:             abs,
		IncludeAdminOnly: includeAdminOnly,
	}, func(path string, _ os.DirEntry, kind string) error {
		if !isMediaKind(kind) && kind != MediaTypeBlog {
			return nil
		}
		childRel, err := filepath.Rel(l.root, path)
		if err != nil {
			return nil
		}
		if _, ok := indexed[filepath.ToSlash(childRel)]; !ok {
			return errFilesystemContentExceedsIndex
		}
		return nil
	})
	if errors.Is(err, errFilesystemContentExceedsIndex) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (l *Library) indexedSearchContentPaths(ctx context.Context, rel string, includeAdminOnly bool) (map[string]struct{}, error) {
	paths := map[string]struct{}{}
	if l == nil || !l.index.available() {
		return paths, nil
	}
	adminFilter := ""
	if !includeAdminOnly {
		adminFilter = " AND admin_only = 0"
	}
	var rows *sql.Rows
	var err error
	if rel == "" {
		rows, err = l.index.db.QueryContext(ctx, `
			SELECT path FROM media_index WHERE 1 = 1`+adminFilter+`
			UNION
			SELECT path FROM blog_index WHERE 1 = 1`+adminFilter)
	} else {
		start, end := prefixRange(rel + "/")
		rows, err = l.index.db.QueryContext(ctx, `
			SELECT path FROM media_index WHERE (directory = ? OR (directory >= ? AND directory < ?))`+adminFilter+`
			UNION
			SELECT path FROM blog_index WHERE (directory = ? OR (directory >= ? AND directory < ?))`+adminFilter,
			rel, start, end,
			rel, start, end,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths[path] = struct{}{}
	}
	return paths, rows.Err()
}
