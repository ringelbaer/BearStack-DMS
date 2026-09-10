package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
)

const groupPhotoStripSize = 32

type GroupPhotoPreview struct {
	Path        string `json:"path"`
	DisplayPath string `json:"display_path"`
	Remaining   int    `json:"remaining"`
}

type GroupPhotoStrip struct {
	Minimum     int                 `json:"minimum"`
	Photos      []GroupPhotoPreview `json:"photos"`
	Before      string              `json:"before"`
	After       string              `json:"after"`
	HasPrevious bool                `json:"has_previous"`
	HasNext     bool                `json:"has_next"`
}

// GroupPhotoPreviews returns a bounded window in the same path order and with
// the same eligibility and visibility checks as NextGroupPhoto. Only metadata
// is read; face rectangles, vectors and original image bytes are unnecessary.
func (l *Library) GroupPhotoPreviews(ctx context.Context, path, after, before string, minimum int) (GroupPhotoStrip, error) {
	out := GroupPhotoStrip{Minimum: minimum, Photos: []GroupPhotoPreview{}, Before: after, After: before}
	if minimum < 0 || minimum > MaxGroupPhotoMinimum || !validGroupPhotoPath(path, true) || !validGroupPhotoPath(after, true) || !validGroupPhotoPath(before, true) ||
		(path != "" && (after != "" || before != "")) || (after != "" && before != "") {
		return out, ErrLabelInvalid
	}
	var err error
	if path != "" {
		current, e := l.groupPhotoPreview(ctx, path)
		if e != nil {
			return out, e
		}
		var previous, next []GroupPhotoPreview
		previous, out.HasPrevious, err = l.groupPhotoPreviewSide(ctx, path, minimum, groupPhotoStripSize/2, true)
		if err != nil {
			return out, err
		}
		next, out.HasNext, err = l.groupPhotoPreviewSide(ctx, path, minimum, groupPhotoStripSize/2, false)
		out.Photos = append(append(previous, current), next...)
	} else if before != "" {
		out.Photos, out.HasPrevious, err = l.groupPhotoPreviewSide(ctx, before, minimum, groupPhotoStripSize, true)
		out.HasNext = true
	} else {
		out.Photos, out.HasNext, err = l.groupPhotoPreviewSide(ctx, after, minimum, groupPhotoStripSize, false)
		out.HasPrevious = after != ""
	}
	if len(out.Photos) > 0 {
		out.Before = out.Photos[0].Path
		out.After = out.Photos[len(out.Photos)-1].Path
	}
	return out, err
}

func (l *Library) groupPhotoPreview(ctx context.Context, path string) (GroupPhotoPreview, error) {
	out := GroupPhotoPreview{Path: path, DisplayPath: mediaDisplayPath(path)}
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return out, err
	}
	var count int
	err := l.index.db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN f.ignored=0 AND p.name='' THEN 1 ELSE 0 END),0)
 FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.path=? AND m.admin_only=0 AND m.type='image'`, path).Scan(&count, &out.Remaining)
	if err == nil && count == 0 {
		err = sql.ErrNoRows
	}
	return out, err
}

func (l *Library) groupPhotoPreviewSide(ctx context.Context, cursor string, minimum, limit int, backwards bool) ([]GroupPhotoPreview, bool, error) {
	out := []GroupPhotoPreview{}
	operator, order := ">", "ASC"
	if backwards {
		operator, order = "<", "DESC"
	}
	for len(out) <= limit {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		rows, err := l.index.db.QueryContext(ctx, `SELECT f.path
 FROM photo_faces f INDEXED BY idx_face_group_candidates
 CROSS JOIN photo_people p ON p.id=f.person_id CROSS JOIN media_index m ON m.path=f.path
 WHERE f.path`+operator+`? AND f.ignored=0 AND (p.name='' OR p.name_source<>'')
 AND m.admin_only=0 AND m.type='image'
 GROUP BY f.path HAVING count(*)>? ORDER BY f.path `+order+` LIMIT ?`, cursor, minimum, limit+1)
		if err != nil {
			return nil, false, err
		}
		paths := []string{}
		for rows.Next() {
			var p string
			if err = rows.Scan(&p); err != nil {
				break
			}
			paths = append(paths, p)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, false, err
		}
		for _, path := range paths {
			cursor = path
			photo, err := l.groupPhotoPreview(ctx, path)
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrAdminOnly()) || errors.Is(err, ErrPathEscapesRoot()) {
				continue
			}
			if err != nil {
				return nil, false, err
			}
			if photo.Remaining > minimum {
				out = append(out, photo)
			}
			if len(out) > limit {
				break
			}
		}
		if len(paths) < limit+1 {
			break
		}
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	if backwards {
		slices.Reverse(out)
	}
	return out, more, nil
}
