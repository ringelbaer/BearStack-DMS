package photos

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

var ErrCatalogDate = errors.New("invalid catalog date")
var ErrCatalogIndexUnavailable = errors.New("photo catalog index is not ready")

// CatalogDatePosition addresses the unfiltered, descending-date native stream.
// Page numbers use the same 96-item pages as /api/photos/v1/browse.
type CatalogDatePosition struct {
	Path string `json:"path"`
	Date string `json:"date"`
	Page int    `json:"page"`
}

type catalogDateKey struct {
	path, captured string
	modified       int64
}

func (k catalogDateKey) date() string {
	if len(k.captured) >= 10 {
		return k.captured[:10]
	}
	return time.Unix(0, k.modified).Format(time.DateOnly)
}

func catalogDateNano(t time.Time) int64 {
	if t.Before(time.Unix(0, math.MinInt64)) {
		return math.MinInt64
	}
	if t.After(time.Unix(0, math.MaxInt64)) {
		return math.MaxInt64
	}
	return t.UnixNano()
}

func (l *Library) CatalogDate(ctx context.Context, date string, includeAdminOnly bool) (CatalogDatePosition, error) {
	out := CatalogDatePosition{Page: 1}
	target, err := time.Parse(time.DateOnly, date)
	if err != nil || len(date) != 10 || target.Year() < 1 {
		return out, ErrCatalogDate
	}
	state, err := l.indexedListingPathState(ctx, "", includeAdminOnly, false)
	if err != nil {
		return out, err
	}
	if !state.Covered {
		return out, ErrCatalogIndexUnavailable
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	from := "media_index INDEXED BY idx_media_index_date WHERE "
	if !includeAdminOnly {
		from = "media_index INDEXED BY idx_media_index_admin_date WHERE admin_only=0 AND "
	}
	lookup := func(where, order string, args ...any) (catalogDateKey, error) {
		var key catalogDateKey
		err := tx.QueryRowContext(ctx, `SELECT path,captured_at,mod_time_unix_nano FROM `+from+where+
			` ORDER BY captured_at `+order+`,mod_time_unix_nano `+order+`,path `+order+` LIMIT 1`, args...).
			Scan(&key.path, &key.captured, &key.modified)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return key, err
	}
	// Four range seeks in existing covering indexes. The displayed capture day
	// retains its original timezone; missing EXIF uses the displayed modification day.
	local, _ := time.ParseInLocation(time.DateOnly, date, time.Local)
	queries := []struct {
		where, order string
		args         []any
	}{
		{"captured_at>'' AND captured_at<?", "DESC", []any{date + "~"}},
		{"captured_at>=?", "ASC", []any{date}},
		{"captured_at='' AND mod_time_unix_nano<=?", "DESC", []any{catalogDateNano(local.AddDate(0, 0, 1).Add(-time.Nanosecond))}},
		{"captured_at='' AND mod_time_unix_nano>=?", "ASC", []any{catalogDateNano(local)}},
	}
	bestDate := ""
	bestDistance := int64(math.MaxInt64)
	for _, q := range queries {
		key, err := lookup(q.where, q.order, q.args...)
		if err != nil {
			return out, err
		}
		if key.path == "" {
			continue
		}
		day, err := time.Parse(time.DateOnly, key.date())
		if err != nil {
			return out, err
		}
		distance := (day.Unix() - target.Unix()) / 86400
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance || (distance == bestDistance && key.date() < bestDate) {
			bestDistance, bestDate = distance, key.date()
		}
	}
	if bestDate == "" {
		return out, ctx.Err()
	}
	// Anchor to the first item of the winning day, including days spanning pages.
	key, err := lookup("captured_at>=? AND captured_at<?", "DESC", bestDate, bestDate+"~")
	if err != nil {
		return out, err
	}
	if key.path == "" {
		day, _ := time.ParseInLocation(time.DateOnly, bestDate, time.Local)
		key, err = lookup("captured_at='' AND mod_time_unix_nano>=? AND mod_time_unix_nano<=?", "DESC",
			catalogDateNano(day), catalogDateNano(day.AddDate(0, 0, 1).Add(-time.Nanosecond)))
		if err != nil {
			return out, err
		}
	}
	var preceding int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+from+
		`(captured_at,mod_time_unix_nano,path)>(?,?,?)`, key.captured, key.modified, key.path).Scan(&preceding)
	if err != nil {
		return out, err
	}
	out.Path, out.Date, out.Page = key.path, bestDate, preceding/96+1
	return out, nil
}
