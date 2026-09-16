package photos

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"bearstack/internal/searchtext"
)

// A separate virtual path keeps selection, pagination and navigation scoped in
// both clients without treating any virtual directory as a filesystem path.
func DirectoryPeoplePath(directory string) string {
	return PeopleFolderPath + "/f-" + base64.RawURLEncoding.EncodeToString([]byte(directory))
}

func directoryPeopleRange(column, directory string) (string, []any) {
	if directory == "" {
		return "1=1", nil
	}
	// '/' is immediately followed by '0': this exact prefix range uses the path
	// index even for folder names containing SQL wildcard characters.
	return column + ">=? AND " + column + "<?", []any{directory + "/", directory + "0"}
}

func (l *Library) listDirectoryPeople(ctx context.Context, rel string, opts ListOptions) (Listing, error) {
	out := newListing(rel, opts)
	parts := strings.Split(rel, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return out, os.ErrNotExist
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(parts[1], "f-"))
	if err != nil || !utf8.Valid(decoded) || len(decoded) > 4096 {
		return out, os.ErrNotExist
	}
	directory := string(decoded)
	clean, err := CleanPath(directory)
	if err != nil || clean != directory || IsPeopleFolder(directory) || DirectoryPeoplePath(directory) != strings.Join(parts[:2], "/") {
		return out, os.ErrNotExist
	}
	private, err := l.FolderAdminOnly(directory)
	if err != nil {
		return out, err
	}
	if private {
		return out, ErrAdminOnly()
	}
	scope, args := directoryPeopleRange("f.path", directory)
	dirs, dirArgs := directoryPeopleRange("directory", directory)
	if directory != "" {
		dirs = "directory=? OR (" + dirs + ")"
		dirArgs = append([]any{directory}, dirArgs...)
	}
	visibility := `SELECT directory FROM photo_face_directories WHERE ` + dirs + `
 UNION SELECT m.directory FROM photo_people p JOIN media_index m ON m.path=p.name_source
 WHERE p.manual_name=0 AND p.id IN(SELECT f.person_id FROM photo_faces f WHERE ` + scope + `)`
	if err := l.refreshFaceDirectories(ctx, visibility, append(dirArgs, args...)...); err != nil {
		return out, err
	}
	out.Breadcrumbs = breadcrumbs(directory)
	out.Breadcrumbs = append(out.Breadcrumbs, Crumb{Name: "Personen im Ordner", Path: DirectoryPeoplePath(directory)})
	decorateListingDisplay(&out)
	out.ParentPath = directory
	if len(parts) == 3 {
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || id <= 0 {
			return out, os.ErrNotExist
		}
		var name string
		queryArgs := append([]any{id}, args...)
		err = l.index.db.QueryRowContext(ctx, `SELECT p.name FROM photo_people p WHERE p.id=? AND EXISTS(
 SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=p.id AND f.ignored=0 AND m.admin_only=0 AND `+scope+`)`, queryArgs...).Scan(&name)
		if err != nil {
			return out, err
		}
		if name == "" {
			name = "Unbenannt"
		}
		out.ParentPath = DirectoryPeoplePath(directory)
		out.Breadcrumbs = append(out.Breadcrumbs, Crumb{Name: name, Path: rel})
		err = l.listPersonFolderMedia(ctx, &out, id, directory, opts)
		return out, err
	}
	if opts.SkipFolders {
		return out, nil
	}
	size := opts.FolderPageSize
	if size <= 0 {
		size = opts.PageSize
	}
	reader, release, err := photoFileTempConn(ctx, l.index.db)
	if err != nil {
		return out, err
	}
	defer release()
	// Aggregate once per matching person, including distinct photo counts and
	// portraits from this subtree, preferring its starred faces.
	cte := `WITH scoped AS MATERIALIZED (SELECT f.person_id,count(DISTINCT f.path) AS n,
 coalesce(min(CASE WHEN f.favorite=1 THEN f.id END),min(f.id)) AS face_id
 FROM photo_faces f JOIN photo_people p ON p.id=f.person_id AND p.name<>''
 JOIN media_index m ON m.path=f.path WHERE f.ignored=0 AND m.admin_only=0 AND ` + scope + `
 GROUP BY f.person_id), matched AS (SELECT p.id,p.name,p.name_fold,s.n,s.face_id FROM scoped s JOIN photo_people p ON p.id=s.person_id WHERE p.name_fold LIKE ? ESCAPE '\') `
	args = append(args, searchtext.LikeContainsPattern(searchtext.GermanFold(opts.Query)))
	// Window total avoids repeating the subtree aggregation for count and page.
	direction := "ASC"
	if strings.HasPrefix(opts.Sort, "descending_") {
		direction = "DESC"
	}
	rows, err := reader.QueryContext(ctx, cte+`SELECT id,name,n,face_id,count(*) OVER() FROM matched ORDER BY name_fold `+direction+`,id LIMIT ? OFFSET ?`, append(append([]any{}, args...), size, (opts.Page-1)*size)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, face int64
		var name string
		var count int
		if err := rows.Scan(&id, &name, &count, &face, &out.FolderTotal); err != nil {
			rows.Close()
			return out, err
		}
		if name == "" {
			name = "Unbenannt"
		}
		out.Folders = append(out.Folders, Folder{Name: name, DisplayName: name, Path: rel + "/" + strconv.FormatInt(id, 10), Virtual: true, MediaCount: count, Previews: []Media{{FaceID: face, Name: name, Type: MediaTypeImage, MIMEType: "image/jpeg"}}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Folders) == 0 && opts.Page > 1 {
		if err := reader.QueryRowContext(ctx, cte+`SELECT count(*) FROM matched`, args...).Scan(&out.FolderTotal); err != nil && err != sql.ErrNoRows {
			return out, err
		}
	}
	out.FolderHasNext = opts.Page*size < out.FolderTotal
	if opts.FolderPageSize == 0 {
		out.Total = out.FolderTotal
		out.HasPrev = opts.Page > 1
		out.HasNext = out.FolderHasNext
	}
	return out, nil
}
