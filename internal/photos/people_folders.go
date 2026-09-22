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

// Hidden names are excluded from the physical photo tree. These opaque paths
// are only interpreted by List, never by the filesystem or media endpoints.
const PeopleFolderPath = ".people"

func IsPeopleFolder(path string) bool {
	return path == PeopleFolderPath || strings.HasPrefix(path, PeopleFolderPath+"/")
}

// IsPeopleDirectory identifies paginated lists of people, excluding the category
// root and individual photo galleries. List still validates the opaque path.
func IsPeopleDirectory(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 2 && parts[0] == PeopleFolderPath &&
		(parts[1] == "all" || strings.HasPrefix(parts[1], "t-") || strings.HasPrefix(parts[1], "f-"))
}

func PersonFolderPath(id int64) string { return PeopleFolderPath + "/all/" + strconv.FormatInt(id, 10) }

func personTagPath(tag string) string {
	return PeopleFolderPath + "/t-" + base64.RawURLEncoding.EncodeToString([]byte(tag))
}

const visiblePersonSQL = `EXISTS(SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=p.id AND f.ignored=0 AND m.admin_only=0)`

func (l *Library) peopleRootFolder(ctx context.Context, previews int) (Folder, error) {
	if err := l.refreshPeopleVisibility(ctx, ""); err != nil {
		return Folder{}, err
	}
	folders := []Folder{{Name: "Personen", DisplayName: "Personen", Path: PeopleFolderPath, Virtual: true}}
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_people p WHERE p.name<>'' AND `+visiblePersonSQL).Scan(&folders[0].DirCount); err != nil {
		return Folder{}, err
	}
	if err := l.peopleFolderPreviews(ctx, folders, previews); err != nil {
		return Folder{}, err
	}
	return folders[0], nil
}

// One batched query ranks distinct photos per person, then loads at most eight
// portrait IDs per returned folder. Original images are never decoded here.
func (l *Library) peopleFolderPreviews(ctx context.Context, folders []Folder, limit int) error {
	if len(folders) == 0 {
		return nil
	}
	values, args := make([]string, 0, len(folders)), make([]any, 0, len(folders)*3+1)
	for i, folder := range folders {
		values = append(values, "(?,?,?)")
		var tag any
		if len(folder.Tags) > 0 {
			tag = folder.Tags[0]
		}
		args = append(args, i, tag, folder.Path == PeopleFolderPath || folder.Path == PeopleFolderPath+"/all")
	}
	args = append(args, limit)
	reader, release, err := photoFileTempConn(ctx, l.index.db)
	if err != nil {
		return err
	}
	defer release()
	rows, err := reader.QueryContext(ctx, `WITH requested(slot,tag,named_only) AS (VALUES `+strings.Join(values, ",")+`),
 counts AS MATERIALIZED (SELECT f.person_id,count(DISTINCT f.path) AS n FROM photo_faces f
 JOIN media_index m ON m.path=f.path WHERE f.ignored=0 AND m.admin_only=0 GROUP BY f.person_id),
 ranked AS (SELECT r.slot,c.person_id,c.n,row_number() OVER(PARTITION BY r.slot ORDER BY c.n DESC,c.person_id) AS rank
 FROM (SELECT r.slot,c.person_id,c.n FROM requested r CROSS JOIN counts c WHERE r.tag IS NULL
 AND (r.named_only=0 OR EXISTS(SELECT 1 FROM photo_people p WHERE p.id=c.person_id AND p.name<>''))
 UNION ALL SELECT r.slot,c.person_id,c.n FROM requested r JOIN person_tag_index pt ON pt.tag=r.tag JOIN counts c ON c.person_id=pt.person_id) c JOIN requested r ON r.slot=c.slot)
 SELECT r.slot,p.name,`+personPortraitSQL+` FROM ranked r JOIN photo_people p ON p.id=r.person_id WHERE r.rank<=? ORDER BY r.slot,r.rank`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var slot int
		var preview Media
		if err := rows.Scan(&slot, &preview.Name, &preview.FaceID); err != nil {
			return err
		}
		preview.Type = MediaTypeImage
		preview.MIMEType = "image/jpeg"
		folders[slot].Previews = append(folders[slot].Previews, preview)
	}
	return rows.Err()
}

func (l *Library) listPeopleFolders(ctx context.Context, rel string, opts ListOptions) (Listing, error) {
	out := newListing(rel, opts)
	if !l.index.available() {
		return out, os.ErrNotExist
	}
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 && strings.HasPrefix(parts[1], "f-") {
		return l.listDirectoryPeople(ctx, rel, opts)
	}
	if len(parts) > 3 {
		return out, os.ErrNotExist
	}
	tag, groupName := "", "Alle"
	if len(parts) >= 2 && parts[1] != "all" {
		if !strings.HasPrefix(parts[1], "t-") {
			return out, os.ErrNotExist
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(parts[1], "t-"))
		if err != nil || len(decoded) == 0 || len(decoded) > 4096 || !utf8.Valid(decoded) {
			return out, os.ErrNotExist
		}
		tag = string(decoded)
		groupName = tag
		if personTagPath(tag) != PeopleFolderPath+"/"+parts[1] {
			return out, os.ErrNotExist
		}
	}
	out.Breadcrumbs = []Crumb{{Name: "Fotos", Path: ""}, {Name: "Personen", Path: PeopleFolderPath}}
	if len(parts) >= 2 {
		out.Breadcrumbs = append(out.Breadcrumbs, Crumb{Name: groupName, Path: PeopleFolderPath + "/" + parts[1]})
	}
	if len(parts) == 3 {
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || id <= 0 {
			return out, os.ErrNotExist
		}
		if err := l.refreshPersonIDsVisibility(ctx, id); err != nil {
			return out, err
		}
		var name string
		if err := l.index.db.QueryRowContext(ctx, `SELECT name FROM photo_people p WHERE p.id=? AND `+visiblePersonSQL+` AND (?='' OR EXISTS(SELECT 1 FROM person_tag_index WHERE person_id=p.id AND tag=?))`, id, tag, tag).Scan(&name); err != nil {
			return out, err
		}
		if name == "" {
			name = "Unbenannt"
		}
		out.Breadcrumbs = append(out.Breadcrumbs, Crumb{Name: name, Path: rel})
		if err := l.listPersonFolderMedia(ctx, &out, id, "", opts); err != nil {
			return out, err
		}

		return out, nil
	}
	visibilityFilter, visibilityArgs := "", []any{}
	if tag != "" {
		visibilityFilter = `p.id IN (SELECT person_id FROM person_tag_index WHERE tag=?)`
		visibilityArgs = append(visibilityArgs, tag)
	}
	if err := l.refreshPeopleVisibility(ctx, visibilityFilter, visibilityArgs...); err != nil {
		return out, err
	}
	if opts.SkipFolders {
		return out, nil
	}
	size := opts.FolderPageSize
	if size <= 0 {
		size = opts.PageSize
	}
	if len(parts) == 1 {
		// Only tags with active, visible people enter this one-level directory.
		if err := l.index.db.QueryRowContext(ctx, `SELECT 1+count(DISTINCT pt.tag) FROM person_tag_index pt JOIN photo_people p ON p.id=pt.person_id WHERE `+visiblePersonSQL).Scan(&out.FolderTotal); err != nil {
			return out, err
		}
		direction := "ASC"
		if opts.Sort == "descending_name" {
			direction = "DESC"
		}
		rows, err := l.index.db.QueryContext(ctx, `WITH entries AS (
 SELECT 'Alle' AS name,count(*) AS n,1 AS is_all FROM photo_people p WHERE p.name<>'' AND `+visiblePersonSQL+`
 UNION ALL SELECT pt.tag,count(*),0 FROM person_tag_index pt JOIN photo_people p ON p.id=pt.person_id WHERE `+visiblePersonSQL+` GROUP BY pt.tag)
 SELECT name,n,is_all FROM entries ORDER BY is_all DESC,bearstack_german_fold(name) `+direction+`,name `+direction+` LIMIT ? OFFSET ?`, size, (opts.Page-1)*size)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var name string
			var count int
			var all bool
			if err := rows.Scan(&name, &count, &all); err != nil {
				rows.Close()
				return out, err
			}
			folder := Folder{Name: name, DisplayName: name, Path: personTagPath(name), Tags: []string{name}, DirCount: count, Virtual: true}
			if all {
				folder.Path = PeopleFolderPath + "/all"
				folder.Tags = nil
			}
			out.Folders = append(out.Folders, folder)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		out.FolderHasNext = opts.Page*size < out.FolderTotal
		if err := l.peopleFolderPreviews(ctx, out.Folders, 2*opts.FolderPreviewSize); err != nil {
			return out, err
		}
	} else {
		filter := visiblePersonSQL + ` AND (?='' OR EXISTS(SELECT 1 FROM person_tag_index WHERE person_id=p.id AND tag=?)) AND p.name_fold LIKE ? ESCAPE '\'`
		if tag == "" {
			filter = `p.name<>'' AND ` + filter
		}
		args := []any{tag, tag, searchtext.LikeContainsPattern(searchtext.GermanFold(opts.Query))}
		if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_people p WHERE `+filter, args...).Scan(&out.FolderTotal); err != nil {
			return out, err
		}
		if tag != "" && out.FolderTotal == 0 && opts.Query == "" {
			return out, sql.ErrNoRows
		}
		direction := "ASC"
		if strings.HasPrefix(opts.Sort, "descending_") {
			direction = "DESC"
		}
		args = append(args, size, (opts.Page-1)*size)
		var reader peopleSortReader = l.index.db
		columns, count, order := "p.id,p.name,p.name_fold", personPhotoCountSQL, "p.name_fold "+direction+",p.id"
		if opts.Sort == "ascending_count" || opts.Sort == "descending_count" {
			// Count matching people before paging; portraits remain limited to the
			// returned page. The person/path index bounds each distinct-photo count.
			columns += "," + personPhotoCountSQL + " AS photo_count"
			count = "p.photo_count"
			order = "photo_count " + direction + ",p.name_fold,p.id"
			// Allow large aggregate sorts to spill to disk, like the people editor.
			conn, release, err := photoFileTempConn(ctx, l.index.db)
			if err != nil {
				return out, err
			}
			defer release()
			reader = conn
		}
		rows, err := reader.QueryContext(ctx, `WITH page AS MATERIALIZED (SELECT `+columns+` FROM photo_people p WHERE `+filter+` ORDER BY `+order+` LIMIT ? OFFSET ?)
 SELECT p.id,p.name,`+count+`,`+personPortraitSQL+` FROM page p ORDER BY `+order, args...)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, face int64
			var name string
			var count int
			if err := rows.Scan(&id, &name, &count, &face); err != nil {
				return out, err
			}
			if name == "" {
				name = "Unbenannt"
			}
			out.Folders = append(out.Folders, Folder{Name: name, DisplayName: name, Path: rel + "/" + strconv.FormatInt(id, 10), Virtual: true, MediaCount: count, Previews: []Media{{FaceID: face, Name: name, Type: MediaTypeImage, MIMEType: "image/jpeg"}}})
		}
		if err := rows.Err(); err != nil {
			return out, err
		}
		out.FolderHasNext = opts.Page*size < out.FolderTotal
	}
	// Browser pagination is shared with the ordinary gallery; native clients
	// keep their independent folder and media sections.
	if opts.FolderPageSize == 0 {
		out.Total = out.FolderTotal
		out.HasNext = out.FolderHasNext
		out.HasPrev = opts.Page > 1
	}
	return out, nil
}

// Both global and folder-scoped person galleries use the regular media planner.
func (l *Library) listPersonFolderMedia(ctx context.Context, out *Listing, id int64, directory string, opts ListOptions) error {
	if !opts.SkipMedia {
		plan := indexQueryPlanFor(opts.Query)
		condition := `mi.path IN (SELECT path FROM photo_faces WHERE person_id=? AND ignored=0)`
		if plan.ExpressionSQL != "" {
			condition = "(" + plan.ExpressionSQL + ") AND " + condition
		}
		plan.ExpressionSQL = condition
		plan.ExpressionArgs = append(plan.ExpressionArgs, id)
		var err error
		out.Media, out.Total, err = l.indexMedia(ctx, indexMediaOptions{Directory: directory, Subtree: directory != "", Query: opts.Query, Plan: plan, MediaType: opts.MediaType, GPSOnly: opts.GPSOnly, RequestSort: opts.Sort, Limit: opts.PageSize, Offset: (opts.Page - 1) * opts.PageSize, LeanMetadata: opts.LeanMetadata})
		if err != nil {
			return err
		}
	}
	if err := l.AddImageGroups(ctx, out.Media); err != nil {
		return err
	}
	out.HasPrev = opts.Page > 1
	out.HasNext = opts.Page*opts.PageSize < out.Total
	return nil
}
