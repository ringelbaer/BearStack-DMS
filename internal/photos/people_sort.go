package photos

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// NormalizePeopleSort is shared by HTTP parsing and the library. Only these
// fixed expressions enter SQL; request values are never interpolated.
func NormalizePeopleSort(value string) (string, error) {
	switch value {
	case "":
		return "name_asc", nil
	case "name_asc", "name_desc", "count_asc", "count_desc", "folder_asc", "folder_desc", "date_asc", "date_desc":
		return value, nil
	default:
		return "", errors.New("ungültige Personensortierung")
	}
}

func peopleSortOption(options []string) (string, error) {
	if len(options) == 0 {
		return NormalizePeopleSort("")
	}
	return NormalizePeopleSort(options[0])
}

const personPhotoCountSQL = `(SELECT count(DISTINCT path) FROM photo_faces WHERE person_id=p.id AND ignored=0)`
const personPortraitSQL = `(SELECT min(id) FROM photo_faces WHERE person_id=p.id AND ignored=0)`

func peopleOverviewSQL(sorting, filter string) string {
	key, count := "p.name_fold", personPhotoCountSQL
	switch strings.Split(sorting, "_")[0] {
	case "count":
		key, count = personPhotoCountSQL, "p.sort_key"
	case "folder":
		key = `(SELECT bearstack_german_fold(CASE WHEN m.directory='' THEN 'Fotos' ELSE m.directory END)
 FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.id=` + personPortraitSQL + `)`
	case "date":
		// Deduplicate before reading timestamps: several detections of the same
		// person in one image must not multiply metadata lookups.
		key = `(SELECT max(bearstack_route_time(m.captured_at,m.mod_time_unix_nano))
 FROM (SELECT DISTINCT path FROM photo_faces WHERE person_id=p.id AND ignored=0) f
 JOIN media_index m ON m.path=f.path WHERE m.admin_only=0)`
	}
	direction := " ASC"
	if strings.HasSuffix(sorting, "_desc") {
		direction = " DESC"
	}
	return `WITH candidates AS (
 SELECT p.id,p.name,` + key + ` AS sort_key FROM photo_people p
 WHERE p.name_fold LIKE ? ESCAPE '\'` + filter + `
 AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=p.id AND ignored=0)),
 page AS MATERIALIZED (SELECT * FROM candidates ORDER BY sort_key` + direction + `,id` + direction + ` LIMIT 61 OFFSET ?)
 SELECT p.id,p.name,` + count + `,f.id,f.path,f.x,f.y,f.width,f.height
 FROM page p JOIN photo_faces f ON f.id=` + personPortraitSQL + `
 ORDER BY p.sort_key` + direction + `,p.id` + direction
}

func ignoredFacesSQL(sorting, filter string) string {
	from := ` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path`
	where := ` WHERE f.ignored=1 AND m.admin_only=0 AND p.name_fold LIKE ? ESCAPE '\'` + filter
	key, cte := "p.name_fold", ""
	switch strings.Split(sorting, "_")[0] {
	case "count":
		// Aggregate once per group, not once for every ignored face in it.
		cte = `WITH counts AS MATERIALIZED (SELECT f.person_id,count(DISTINCT f.path) AS photo_count` + from + where + ` GROUP BY f.person_id) `
		from += ` JOIN counts c ON c.person_id=p.id`
		where = ` WHERE f.ignored=1 AND m.admin_only=0`
		key = "c.photo_count"
	case "folder":
		key = `bearstack_german_fold(CASE WHEN m.directory='' THEN 'Fotos' ELSE m.directory END)`
	case "date":
		key = `bearstack_route_time(m.captured_at,m.mod_time_unix_nano)`
	}
	direction := " ASC"
	if strings.HasSuffix(sorting, "_desc") {
		direction = " DESC"
	}
	return cte + `SELECT ` + faceColumns + from + where + ` ORDER BY ` + key + direction + `,f.id` + direction + ` LIMIT 61 OFFSET ?`
}

type peopleSortReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (l *Library) peopleSortReader(ctx context.Context, sorting string) (peopleSortReader, func(), error) {
	if strings.HasPrefix(sorting, "name_") {
		return l.index.db, func() {}, nil
	}
	// Large aggregate sorts can spill to disk instead of competing with image
	// decoding for RAM. Restore the pool's settings, including on cancellation.
	return photoFileTempConn(ctx, l.index.db)
}
