// Datei enthaelt Medienabfragen und SQL-Planung aus dem Fotoindex.
package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/searchtext"
)

const (
	negatedTagFastPathMaxExcluded  = 5000
	indexPostFilterCandidateMin    = 1000
	indexPostFilterCandidateFactor = 20
)

var indexPostFilterCandidateMax = 10000

type indexMediaOptions struct {
	Directory        string
	ExactDir         bool
	Subtree          bool
	Query            string
	Plan             indexQueryPlan
	MediaType        string
	GPSOnly          bool
	Order            string
	RequestSort      string
	Limit            int
	Offset           int
	LeanMetadata     bool
	IncludeAdminOnly bool
}

func (l *Library) indexMedia(ctx context.Context, opts indexMediaOptions) ([]Media, int, error) {
	if queryHasPerson(opts.Query) {
		if err := l.RefreshFaceVisibility(ctx); err != nil {
			return nil, 0, err
		}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultPageSize
	}
	finishFastPath := StartListTraceStep(ctx, "photos.index.media_negated_tag_fast", ListTraceBool("eligible", opts.Plan.onlyNegatedTagTerm()))
	if media, total, ok, err := l.indexMediaNegatedTagFast(ctx, opts); ok || err != nil {
		finishFastPath(
			ListTraceBool("used", ok),
			ListTraceInt("count", len(media)),
			ListTraceInt("total", total),
		)
		return media, total, err
	}
	finishFastPath(ListTraceBool("used", false))
	where, args, joinSearch := indexWhere(opts)
	if opts.Plan.PostFilter {
		return l.indexMediaPostFilter(ctx, opts, where, args, joinSearch)
	}
	var total int
	finishTotal := StartListTraceStep(ctx, "photos.index.media_total", ListTraceBool("fast_candidate", true))
	if fastTotal, ok := l.indexFastTotal(ctx, opts); ok {
		total = fastTotal
		finishTotal(ListTraceBool("fast", true), ListTraceInt("total", total))
	} else {
		from := indexMediaFrom(opts, joinSearch)
		countSQL := `SELECT COUNT(*) FROM ` + from + where
		if err := l.index.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
			finishTotal(ListTraceBool("fast", false), ListTraceString("error", err.Error()))
			return nil, 0, err
		}
		finishTotal(ListTraceBool("fast", false), ListTraceInt("total", total), ListTraceBool("join_search", joinSearch))
	}
	from := indexMediaFrom(opts, joinSearch)
	queryArgs := append(append([]any(nil), args...), opts.Limit, opts.Offset)
	includeMetadata := !opts.LeanMetadata
	finishQuery := StartListTraceStep(ctx, "photos.index.media_page_query",
		ListTraceInt("limit", opts.Limit),
		ListTraceInt("offset", opts.Offset),
		ListTraceBool("metadata", includeMetadata),
		ListTraceBool("join_search", joinSearch),
	)
	if total == 0 || opts.Offset >= total {
		finishQuery(
			ListTraceBool("skipped", true),
			ListTraceInt("count", 0),
		)
		return []Media{}, total, nil
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+mediaIndexColumnsWithMetadata(`mi`, includeMetadata)+` FROM `+from+where+indexOrderSQL(opts.Order, opts.RequestSort)+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		finishQuery(ListTraceString("error", err.Error()))
		return nil, 0, err
	}
	defer rows.Close()
	media, err := scanIndexedMediaRowsWithMetadata(rows, includeMetadata)
	finishQuery(ListTraceInt("count", len(media)))
	return media, total, err
}

func (l *Library) indexMediaPostFilter(ctx context.Context, opts indexMediaOptions, where string, args []any, joinSearch bool) ([]Media, int, error) {
	from := `media_index mi`
	if joinSearch {
		from += ` JOIN media_search ON media_search.rowid = mi.rowid`
	}
	includeMetadata := !opts.LeanMetadata
	candidateLimit := postFilterCandidateLimit(opts)
	finishTrace := StartListTraceStep(ctx, "photos.index.media_post_filter",
		ListTraceInt("candidate_limit", candidateLimit),
		ListTraceInt("limit", opts.Limit),
		ListTraceInt("offset", opts.Offset),
		ListTraceBool("metadata", includeMetadata),
		ListTraceBool("join_search", joinSearch),
	)
	queryArgs := append(append([]any(nil), args...), candidateLimit+1)
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+mediaIndexColumns(`mi`)+` FROM `+from+where+indexOrderSQL(opts.Order, opts.RequestSort)+` LIMIT ?`, queryArgs...)
	if err != nil {
		finishTrace(ListTraceString("error", err.Error()))
		return nil, 0, err
	}
	defer rows.Close()
	page := make([]Media, 0, opts.Limit)
	total := 0
	scanned := 0
	query := compileMediaQuery(opts.Query)
	candidates := []cachedMediaRow{}
	for rows.Next() {
		row, e := scanCachedMediaRow(rows)
		if e != nil {
			rows.Close()
			return nil, 0, e
		}
		candidates = append(candidates, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	paths := make([]string, len(candidates))
	for i := range candidates {
		paths[i] = candidates[i].Path
	}
	auto, err := l.automaticFacesBatch(ctx, paths)
	if err != nil {
		return nil, 0, err
	}
	for _, row := range candidates {
		scanned++
		if scanned > candidateLimit {
			finishTrace(ListTraceInt("scanned", scanned), ListTraceString("error", errPhotoSearchTooBroad.Error()))
			return nil, 0, errPhotoSearchTooBroad
		}
		original := row
		if queryHasPerson(opts.Query) {
			var faces []Face
			_ = json.Unmarshal([]byte(row.Faces), &faces)
			for _, f := range auto[row.Path] {
				faces = append(faces, Face{Name: f.Name})
			}
			b, _ := json.Marshal(faces)
			row.Faces = string(b)
		}
		if query.matchesRow(row) {
			if total >= opts.Offset && len(page) < opts.Limit {
				page = append(page, mediaFromCachedRowWithMetadata(original, includeMetadata))
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		finishTrace(ListTraceInt("scanned", scanned), ListTraceString("error", err.Error()))
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		finishTrace(ListTraceInt("scanned", scanned), ListTraceString("error", err.Error()))
		return nil, 0, err
	}
	finishTrace(ListTraceInt("scanned", scanned), ListTraceInt("count", len(page)), ListTraceInt("total", total))
	return page, total, nil
}

func postFilterCandidateLimit(opts indexMediaOptions) int {
	limit := opts.Offset + opts.Limit*indexPostFilterCandidateFactor
	if limit < indexPostFilterCandidateMin {
		limit = indexPostFilterCandidateMin
	}
	if limit > indexPostFilterCandidateMax {
		return indexPostFilterCandidateMax
	}
	return limit
}

func (l *Library) indexMediaNegatedTagFast(ctx context.Context, opts indexMediaOptions) ([]Media, int, bool, error) {
	if !opts.Plan.onlyNegatedTagTerm() || opts.Directory != "" || opts.MediaType != "" || opts.GPSOnly || opts.ExactDir || !opts.usesDateOrder() {
		return nil, 0, false, nil
	}
	tags := cleanPhotoTags([]string{opts.Plan.SQLTerms[0].Value})
	if len(tags) != 1 {
		return nil, 0, false, nil
	}
	total, ok := l.indexFastTotal(ctx, opts)
	if !ok {
		return nil, 0, false, nil
	}
	excludedRows, err := l.index.db.QueryContext(ctx, `SELECT media_path FROM media_tag_index WHERE tag = ?`, tags[0])
	if err != nil {
		return nil, 0, true, err
	}
	defer excludedRows.Close()
	excluded := make(map[string]struct{})
	for excludedRows.Next() {
		var path string
		if err := excludedRows.Scan(&path); err != nil {
			return nil, 0, true, err
		}
		excluded[path] = struct{}{}
		if len(excluded) > negatedTagFastPathMaxExcluded {
			return nil, 0, false, nil
		}
	}
	if err := excludedRows.Err(); err != nil {
		return nil, 0, true, err
	}

	fetchLimit := opts.Offset + opts.Limit + len(excluded)
	if fetchLimit < opts.Limit {
		return nil, 0, false, nil
	}
	from := `media_index AS mi INDEXED BY idx_media_index_date`
	where := ``
	args := make([]any, 0, 1)
	if !opts.IncludeAdminOnly {
		from = `media_index AS mi INDEXED BY idx_media_index_admin_date`
		where = ` WHERE mi.admin_only = 0`
	}
	args = append(args, fetchLimit)
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+mediaIndexColumns(`mi`)+` FROM `+from+where+indexOrderSQL(opts.Order, opts.RequestSort)+` LIMIT ?`, args...)
	if err != nil {
		return nil, 0, true, err
	}
	defer rows.Close()
	page := make([]Media, 0, opts.Limit)
	seen := 0
	for rows.Next() {
		item, err := scanIndexedMedia(rows)
		if err != nil {
			return nil, 0, true, err
		}
		if _, skip := excluded[item.Path]; skip {
			continue
		}
		if seen >= opts.Offset && len(page) < opts.Limit {
			page = append(page, item)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, true, err
	}
	return page, total, true, ctx.Err()
}

func indexMediaFrom(opts indexMediaOptions, joinSearch bool) string {
	from := `media_index mi`
	if !joinSearch {
		if opts.useGlobalGPSDateIndex() {
			from = `media_index AS mi INDEXED BY idx_media_index_gps_date`
		} else if opts.useGlobalDateIndex() {
			if opts.IncludeAdminOnly {
				from = `media_index AS mi INDEXED BY idx_media_index_date`
			} else {
				from = `media_index AS mi INDEXED BY idx_media_index_admin_date`
			}
		}
	}
	if joinSearch {
		from += ` JOIN media_search ON media_search.rowid = mi.rowid`
	}
	return from
}

func (opts indexMediaOptions) useGlobalGPSDateIndex() bool {
	if !opts.IncludeAdminOnly || opts.Directory != "" || opts.MediaType != "" || opts.ExactDir {
		return false
	}
	if opts.GPSOnly {
		return true
	}
	return opts.Plan.onlyGPSTerm()
}

func (opts indexMediaOptions) useGlobalDateIndex() bool {
	if opts.Plan.ExpressionSQL != "" {
		return false
	}
	if opts.Directory != "" || opts.MediaType != "" || opts.GPSOnly || opts.ExactDir || !opts.usesDateOrder() {
		return false
	}
	if len(opts.Plan.SQLTerms) > 0 && !opts.Plan.onlyNegatedTagTerm() {
		return false
	}
	return true
}

func (opts indexMediaOptions) usesDateOrder() bool {
	order := opts.Order
	if opts.RequestSort != "" {
		order = opts.RequestSort
	}
	switch order {
	case "", "ascending_date", "descending_date":
		return true
	default:
		return false
	}
}

func (p indexQueryPlan) onlyGPSTerm() bool {
	return p.FTSQuery == "" && !p.PostFilter && len(p.SQLTerms) == 1 && p.SQLTerms[0].Field == "gps" && truthyPhotoValue(p.SQLTerms[0].Value)
}

func (p indexQueryPlan) onlyNegatedTagTerm() bool {
	return p.FTSQuery == "" && !p.PostFilter && len(p.SQLTerms) == 1 && p.SQLTerms[0].Field == "tag" && p.SQLTerms[0].Negated
}

func (l *Library) indexFastTotal(ctx context.Context, opts indexMediaOptions) (int, bool) {
	if opts.Plan.ExpressionSQL != "" {
		return 0, false
	}
	if opts.ExactDir && opts.Query == "" && opts.MediaType == "" && !opts.GPSOnly && len(opts.Plan.SQLTerms) == 0 {
		var total int
		var err error
		if opts.Directory == "" {
			key := "root_media_count"
			if !opts.IncludeAdminOnly {
				key = "root_public_media_count"
			}
			err = l.index.db.QueryRowContext(ctx, `SELECT value FROM photo_stats WHERE key = ?`, key).Scan(&total)
		} else if opts.IncludeAdminOnly {
			err = l.index.db.QueryRowContext(ctx, `SELECT media_count FROM folder_index WHERE path = ?`, opts.Directory).Scan(&total)
		} else {
			err = l.index.db.QueryRowContext(ctx, `SELECT public_media_count FROM folder_index WHERE path = ?`, opts.Directory).Scan(&total)
			if err == nil && total < 0 {
				return 0, false
			}
		}
		return total, err == nil
	}
	if opts.Directory == "" && opts.MediaType == "" && !opts.GPSOnly && opts.Plan.onlyGPSTerm() {
		if !opts.IncludeAdminOnly {
			return 0, false
		}
		var total int
		err := l.index.db.QueryRowContext(ctx, `SELECT value FROM photo_stats WHERE key = 'gps_media_count'`).Scan(&total)
		return total, err == nil
	}
	if opts.Directory == "" && opts.MediaType == "" && !opts.GPSOnly && opts.Plan.onlyNegatedTagTerm() {
		if !opts.IncludeAdminOnly {
			hasAdminOnly, err := l.indexHasAdminOnlyMedia(ctx)
			if err != nil || hasAdminOnly {
				return 0, false
			}
		}
		tags := cleanPhotoTags([]string{opts.Plan.SQLTerms[0].Value})
		if len(tags) == 0 {
			return 0, true
		}
		var mediaCount int
		if err := l.index.db.QueryRowContext(ctx, `SELECT value FROM photo_stats WHERE key = 'media_count'`).Scan(&mediaCount); err != nil {
			return 0, false
		}
		var tagCount int
		if err := l.index.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_tag_index WHERE tag = ?`, tags[0]).Scan(&tagCount); err != nil {
			return 0, false
		}
		return mediaCount - tagCount, true
	}
	return 0, false
}

func indexWhere(opts indexMediaOptions) (string, []any, bool) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if !opts.IncludeAdminOnly {
		where = append(where, "mi.admin_only = 0")
	}
	if opts.ExactDir {
		where = append(where, "mi.directory = ?")
		args = append(args, opts.Directory)
	} else if opts.Subtree && opts.Directory != "" {
		start, end := prefixRange(opts.Directory + "/")
		where = append(where, "(mi.directory = ? OR (mi.directory >= ? AND mi.directory < ?))")
		args = append(args, opts.Directory, start, end)
	}
	mediaType := normalizeMediaType(opts.MediaType)
	if mediaType != "" {
		where = append(where, "mi.type = ?")
		args = append(args, mediaType)
	}
	if opts.GPSOnly {
		where = append(where, "mi.latitude IS NOT NULL AND mi.longitude IS NOT NULL")
	}
	if opts.Plan.ExpressionSQL != "" {
		where = append(where, opts.Plan.ExpressionSQL)
		args = append(args, opts.Plan.ExpressionArgs...)
	}
	for _, term := range opts.Plan.SQLTerms {
		appendIndexTermWhere(&where, &args, term)
	}
	joinSearch := opts.Plan.FTSQuery != ""
	if joinSearch {
		where = append(where, "media_search MATCH ?")
		args = append(args, opts.Plan.FTSQuery)
	}
	if len(where) == 0 {
		return "", args, joinSearch
	}
	return " WHERE " + strings.Join(where, " AND "), args, joinSearch
}

func appendIndexTermWhere(where *[]string, args *[]any, term queryTerm) {
	add := func(condition string, values ...any) {
		if term.Negated {
			condition = "NOT (" + condition + ")"
		}
		*where = append(*where, condition)
		*args = append(*args, values...)
	}
	switch term.Field {
	case "person":
		pattern := searchtext.LikeContainsPattern(searchtext.GermanFold(term.Value))
		add(`mi.path IN (SELECT path FROM photo_xmp_people WHERE bearstack_german_fold(name) LIKE ? ESCAPE '\' UNION SELECT f.path FROM photo_people p CROSS JOIN photo_faces f INDEXED BY idx_photo_faces_person ON f.person_id=p.id WHERE p.name_fold LIKE ? ESCAPE '\' AND f.ignored=0)`, pattern, pattern)
	case "type":
		if v := normalizeMediaType(term.Value); v != "" {
			add("mi.type = ?", v)
		}
	case "gps":
		if falseyPhotoValue(term.Value) {
			add("(mi.latitude IS NULL OR mi.longitude IS NULL)")
		} else {
			add("mi.latitude IS NOT NULL AND mi.longitude IS NOT NULL")
		}
	case "directory":
		value := strings.Trim(strings.TrimSpace(term.Value), "/")
		if value != "" {
			add("bearstack_german_fold(mi.directory) LIKE ? ESCAPE '\\'", searchtext.LikeContainsPattern(searchtext.GermanFold(value)))
		}
	case "file_name":
		add("bearstack_german_fold(mi.name) LIKE ? ESCAPE '\\'", searchtext.LikeContainsPattern(searchtext.GermanFold(term.Value)))
	case "date":
		if y, ok := yearPrefix(term.Value); ok {
			add("mi.captured_at >= ? AND mi.captured_at < ?", y+"-01-01T00:00:00Z", nextYear(y)+"-01-01T00:00:00Z")
		} else {
			add("mi.captured_at LIKE ?", term.Value+"%")
		}
	case "orientation":
		add("mi.orientation = ?", strings.ToLower(term.Value))
	case "resolution":
		if condition, values, ok := resolutionSQLCondition(term.Value); ok {
			add(condition, values...)
		}
	case "tag":
		tags := cleanPhotoTags([]string{term.Value})
		if term.Negated && len(tags) == 0 {
			*where = append(*where, "0")
			return
		}
		for _, tag := range tags {
			if term.Negated {
				*where = append(*where, "NOT EXISTS (SELECT 1 FROM media_tag_index mti WHERE mti.media_path = mi.path AND mti.tag = ?)")
			} else {
				*where = append(*where, "mi.path IN (SELECT media_path FROM media_tag_index WHERE tag = ?)")
			}
			*args = append(*args, tag)
		}
	}
}

func resolutionSQLCondition(value string) (string, []any, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil, false
	}
	op := ">="
	for _, candidate := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(value, candidate) {
			op = candidate
			value = strings.TrimSpace(strings.TrimPrefix(value, candidate))
			break
		}
	}
	threshold, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "", nil, false
	}
	return "mi.width > 0 AND mi.height > 0 AND ((mi.width * mi.height) / 1000000.0) " + op + " ?", []any{threshold}, true
}

func mediaIndexColumns(alias string) string {
	return mediaIndexColumnsWithMetadata(alias, true)
}

func mediaIndexColumnsWithMetadata(alias string, includeMetadata bool) string {
	if alias != "" {
		alias += "."
	}
	keywords := `'[]'`
	tags := `'[]'`
	faces := `'[]'`
	if includeMetadata {
		keywords = alias + "keywords"
		tags = alias + "tags"
		faces = alias + "faces"
	}
	return alias + `path, ` + alias + `name, ` + alias + `directory, ` + alias + `type, ` + alias + `mime_type,
			` + alias + `size_bytes, ` + alias + `mod_time_unix_nano, ` + alias + `captured_at, ` + alias + `width,
			` + alias + `height, ` + alias + `orientation, ` + alias + `camera, ` + alias + `lens,
			` + alias + `rating, ` + alias + `latitude, ` + alias + `longitude, ` + keywords + `, ` + tags + `, ` + faces + `,
			` + alias + `xmp_fingerprint,
			` + alias + `admin_only`
}

func indexOrderSQL(folderOrder, requestSort string) string {
	order := folderOrder
	if requestSort != "" {
		order = requestSort
	}
	switch order {
	case "ascending_name":
		return " ORDER BY mi.name COLLATE NOCASE ASC, mi.path ASC"
	case "descending_name":
		return " ORDER BY mi.name COLLATE NOCASE DESC, mi.path DESC"
	case "ascending_date":
		return " ORDER BY mi.captured_at ASC, mi.mod_time_unix_nano ASC, mi.path ASC"
	case "descending_date":
		return " ORDER BY mi.captured_at DESC, mi.mod_time_unix_nano DESC, mi.path DESC"
	case "random":
		return " ORDER BY mi.random_hash ASC, mi.path ASC"
	default:
		return " ORDER BY mi.captured_at ASC, mi.mod_time_unix_nano ASC, mi.path ASC"
	}
}

func scanIndexedMediaRowsWithMetadata(rows *sql.Rows, includeMetadata bool) ([]Media, error) {
	media := make([]Media, 0)
	for rows.Next() {
		item, err := scanIndexedMediaWithMetadata(rows, includeMetadata)
		if err != nil {
			return nil, err
		}
		media = append(media, item)
	}
	return media, rows.Err()
}

type mediaScanner interface {
	Scan(dest ...any) error
}

func scanCachedMediaRow(scanner mediaScanner) (cachedMediaRow, error) {
	var row cachedMediaRow
	if err := scanner.Scan(
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
	); err != nil {
		return cachedMediaRow{}, err
	}
	return row, nil
}

func scanIndexedMedia(scanner mediaScanner) (Media, error) {
	return scanIndexedMediaWithMetadata(scanner, true)
}

func scanIndexedMediaWithMetadata(scanner mediaScanner, includeMetadata bool) (Media, error) {
	row, err := scanCachedMediaRow(scanner)
	if err != nil {
		return Media{}, err
	}
	return mediaFromCachedRowWithMetadata(row, includeMetadata), nil
}

func scanIndexedMediaWithPrefixAndMetadata(scanner mediaScanner, prefix *string, includeMetadata bool) (Media, error) {
	var row cachedMediaRow
	if err := scanner.Scan(
		prefix,
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
	); err != nil {
		return Media{}, err
	}
	return mediaFromCachedRowWithMetadata(row, includeMetadata), nil
}

func mediaFromCachedRow(row cachedMediaRow) Media {
	return mediaFromCachedRowWithMetadata(row, true)
}

func mediaFromCachedRowWithMetadata(row cachedMediaRow, includeMetadata bool) Media {
	media := Media{
		Name:           row.Name,
		Path:           row.Path,
		Directory:      row.Directory,
		Type:           row.Type,
		MIMEType:       row.MIMEType,
		SizeBytes:      row.SizeBytes,
		ModTime:        time.Unix(0, row.ModTimeUnixNano),
		Width:          row.Width,
		Height:         row.Height,
		Orientation:    row.Orientation,
		Camera:         row.Camera,
		Lens:           row.Lens,
		XMPFingerprint: row.XMPFingerprint,
		AdminOnly:      row.AdminOnly != 0,
	}
	if row.Rating.Valid {
		rating := row.Rating.Float64
		media.Rating = &rating
	}
	if row.CapturedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, row.CapturedAt); err == nil {
			media.CapturedAt = &parsed
		}
	}
	if row.Latitude.Valid {
		lat := row.Latitude.Float64
		media.Latitude = &lat
	}
	if row.Longitude.Valid {
		lon := row.Longitude.Float64
		media.Longitude = &lon
	}
	if includeMetadata {
		_ = json.Unmarshal([]byte(row.Keywords), &media.Keywords)
		media.Tags = tagsFromJSON(row.Tags)
		_ = json.Unmarshal([]byte(row.Faces), &media.Faces)
	}
	return media
}
