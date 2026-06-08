// Datei buendelt Schreib-Fassaden und gemeinsame Serialisierungshelfer fuer den Fotoindex.
package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bearstack/internal/sqlutil"
)

func (l *Library) cachedMedia(base Media) (Media, bool) {
	if l == nil {
		return Media{}, false
	}
	return l.index.cachedMedia(base)
}

func cachedMediaFromRow(base Media, row cachedMediaRow) (Media, bool) {
	if row.SizeBytes != base.SizeBytes || row.ModTimeUnixNano != base.ModTime.UnixNano() || row.XMPFingerprint != base.XMPFingerprint || (row.AdminOnly != 0) != base.AdminOnly {
		return Media{}, false
	}
	media := mediaFromCachedRow(row)
	media.ModTime = base.ModTime
	return media, true
}

func (l *Library) saveMedia(media Media) {
	if err := l.saveMediaContext(context.Background(), media); err != nil {
		l.logWriteError("photo media cache update failed", media.Path, err)
	}
}

func (l *Library) saveMediaContext(ctx context.Context, media Media) error {
	if l == nil {
		return nil
	}
	return l.saveMediaBatch(ctx, []Media{media})
}

func (l *Library) saveMediaBatch(ctx context.Context, items []Media) error {
	return l.saveMediaBatchWithExisting(ctx, items, nil)
}

func (l *Library) saveMediaBatchWithExisting(ctx context.Context, items []Media, existing map[string]cachedMediaRow) error {
	if l == nil {
		return nil
	}
	return l.index.saveMediaBatchWithExisting(ctx, items, existing)
}

const (
	mediaWriteChunkSize  = 50
	searchWriteChunkSize = 200
)

type mediaSearchRow struct {
	RowID      int64
	Path       string
	SearchText string
	Tags       []string
	Replace    bool
}

var (
	mediaUpsertSQLCache  sync.Map
	searchInsertSQLCache sync.Map
)

func mediaUpsertSQL(items []Media, indexedAt string) (string, []any) {
	args := make([]any, 0, len(items)*23)
	for _, media := range items {
		capturedAt := media.ModTime.Format(time.RFC3339Nano)
		if media.CapturedAt != nil {
			capturedAt = media.CapturedAt.Format(time.RFC3339Nano)
		}
		args = append(args,
			media.Path,
			media.Name,
			media.Directory,
			media.Type,
			media.MIMEType,
			media.SizeBytes,
			media.ModTime.UnixNano(),
			capturedAt,
			media.Width,
			media.Height,
			media.Orientation,
			media.Camera,
			media.Lens,
			nullableFloat(media.Rating),
			nullableFloat(media.Latitude),
			nullableFloat(media.Longitude),
			stringSliceJSON(media.Keywords),
			tagsJSONString(media.Tags),
			facesJSONString(media.Faces),
			media.XMPFingerprint,
			boolInt(media.AdminOnly),
			stableHashKey(media.Path),
			indexedAt,
		)
	}
	return mediaUpsertSQLForCount(len(items)), args
}

func mediaUpsertSQLForCount(count int) string {
	if cached, ok := mediaUpsertSQLCache.Load(count); ok {
		return cached.(string)
	}
	var b strings.Builder
	b.Grow(640 + count*64)
	b.WriteString(`INSERT INTO media_index(path, name, directory, type, mime_type, size_bytes, mod_time_unix_nano,
		captured_at, width, height, orientation, camera, lens, rating, latitude, longitude, keywords, tags, faces, xmp_fingerprint, admin_only, random_hash, indexed_at) VALUES `)
	for i := 0; i < count; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}
	b.WriteString(` ON CONFLICT(path) DO UPDATE SET
		name = excluded.name,
		directory = excluded.directory,
		type = excluded.type,
		mime_type = excluded.mime_type,
		size_bytes = excluded.size_bytes,
		mod_time_unix_nano = excluded.mod_time_unix_nano,
		captured_at = excluded.captured_at,
		width = excluded.width,
		height = excluded.height,
		orientation = excluded.orientation,
		camera = excluded.camera,
		lens = excluded.lens,
		rating = excluded.rating,
		latitude = excluded.latitude,
		longitude = excluded.longitude,
		keywords = excluded.keywords,
		tags = CASE WHEN media_index.tags = '[]' OR media_index.tags = '' THEN excluded.tags ELSE media_index.tags END,
		faces = excluded.faces,
		xmp_fingerprint = excluded.xmp_fingerprint,
		admin_only = excluded.admin_only,
		random_hash = excluded.random_hash,
		indexed_at = excluded.indexed_at
	RETURNING path, rowid, tags`)
	sqlText := b.String()
	actual, _ := mediaUpsertSQLCache.LoadOrStore(count, sqlText)
	return actual.(string)
}

func replaceMediaSearchRows(ctx context.Context, tx *sql.Tx, rows []mediaSearchRow) error {
	deleteRows := make([]mediaSearchRow, 0, len(rows))
	for _, row := range rows {
		if row.Replace {
			deleteRows = append(deleteRows, row)
		}
	}
	for start := 0; start < len(deleteRows); start += searchWriteChunkSize {
		end := start + searchWriteChunkSize
		if end > len(deleteRows) {
			end = len(deleteRows)
		}
		args := make([]any, 0, end-start)
		for _, row := range deleteRows[start:end] {
			args = append(args, row.RowID)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_search WHERE rowid IN (`+sqlutil.Placeholders(len(args))+`)`, args...); err != nil {
			return err
		}
	}
	for start := 0; start < len(rows); start += searchWriteChunkSize {
		end := start + searchWriteChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		args := make([]any, 0, (end-start)*3)
		for _, row := range rows[start:end] {
			args = append(args, row.RowID, row.Path, row.SearchText)
		}
		if _, err := tx.ExecContext(ctx, searchInsertSQL(end-start), args...); err != nil {
			return err
		}
	}
	return nil
}

func searchInsertSQL(count int) string {
	if cached, ok := searchInsertSQLCache.Load(count); ok {
		return cached.(string)
	}
	var b strings.Builder
	b.Grow(80 + count*12)
	b.WriteString(`INSERT INTO media_search(rowid, path, search_text) VALUES `)
	for i := 0; i < count; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`(?, ?, ?)`)
	}
	sqlText := b.String()
	actual, _ := searchInsertSQLCache.LoadOrStore(count, sqlText)
	return actual.(string)
}

type cachedMediaRow struct {
	Path            string
	Name            string
	Directory       string
	Type            string
	MIMEType        string
	SizeBytes       int64
	ModTimeUnixNano int64
	CapturedAt      string
	Width           int
	Height          int
	Orientation     string
	Camera          string
	Lens            string
	Rating          sql.NullFloat64
	Latitude        sql.NullFloat64
	Longitude       sql.NullFloat64
	Keywords        string
	Tags            string
	Faces           string
	XMPFingerprint  string
	AdminOnly       int
}

type cachedBlogRow struct {
	Path            string
	Name            string
	Directory       string
	Date            string
	ModTimeUnixNano int64
	Text            string
	Tags            string
	AdminOnly       int
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func tagsJSONString(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	tags = cleanPhotoTags(tags)
	if len(tags) == 0 {
		return "[]"
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func stringSliceJSON(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func facesJSONString(faces []Face) string {
	if len(faces) == 0 {
		return "[]"
	}
	data, err := json.Marshal(faces)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (l *Library) refreshMediaSearch(ctx context.Context, path string) error {
	if l == nil {
		return nil
	}
	return l.index.refreshMediaSearch(ctx, path)
}

func scanIndexedMediaWithRowID(scanner mediaScanner) (Media, int64, error) {
	var row cachedMediaRow
	var rowID int64
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
		&rowID,
	); err != nil {
		return Media{}, 0, err
	}
	return mediaFromCachedRow(row), rowID, nil
}

func (l *Library) SetMediaTags(path string, tags []string) ([]string, error) {
	return l.SetMediaTagsContext(context.Background(), path, tags)
}

func (l *Library) SetMediaTagsContext(ctx context.Context, path string, tags []string) ([]string, error) {
	media, err := l.MediaContext(ctx, path)
	if err != nil {
		return nil, err
	}
	tags = cleanPhotoTags(tags)
	if l == nil {
		return tags, nil
	}
	if err := l.index.setMediaTags(ctx, media.Path, tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func (l *Library) SetFolderTags(path string, tags []string) ([]string, error) {
	return l.SetFolderTagsContext(context.Background(), path, tags)
}

func (l *Library) SetFolderTagsContext(ctx context.Context, path string, tags []string) ([]string, error) {
	rel, err := CleanPath(path)
	if err != nil {
		return nil, err
	}
	abs, err := l.Resolve(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}
	tags = cleanPhotoTags(tags)
	folder := Folder{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		Tags:      tags,
		AdminOnly: l.directoryAdminOnlyFromAbs(rel, abs),
		ModTime:   info.ModTime(),
	}
	if rel == "" {
		folder.Name = "Fotos"
	}
	if l == nil {
		return tags, nil
	}
	if err := l.index.setFolderTags(ctx, folder); err != nil {
		return nil, err
	}
	return tags, nil
}

func (l *Library) folderTags(path string) ([]string, bool) {
	if l == nil {
		return nil, false
	}
	return l.index.folderTags(path)
}

func (l *Library) blogTags(path string) ([]string, bool) {
	if l == nil {
		return nil, false
	}
	return l.index.blogTags(path)
}

func (l *Library) saveFolder(folder Folder) error {
	if l == nil {
		return nil
	}
	return l.index.saveFolder(folder)
}

func (l *Library) refreshFolderSearch(ctx context.Context, path string) error {
	if l == nil {
		return nil
	}
	return l.index.refreshFolderSearch(ctx, path)
}

func (l *Library) saveBlog(post BlogPost) {
	if l == nil {
		return
	}
	_ = l.saveBlogBatch(context.Background(), []BlogPost{post})
}

func (l *Library) saveBlogBatch(ctx context.Context, posts []BlogPost) error {
	if l == nil {
		return nil
	}
	return l.index.saveBlogBatch(ctx, posts)
}

func (l *Library) refreshBlogSearch(ctx context.Context, path string) error {
	if l == nil {
		return nil
	}
	return l.index.refreshBlogSearch(ctx, path)
}

func blogSearchText(post BlogPost) string {
	return strings.Join([]string{
		MediaTypeBlog,
		post.Path,
		post.Name,
		parentPath(post.Path),
		post.Text,
		strings.Join(post.Tags, " "),
	}, " ")
}
