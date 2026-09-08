package photos

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"strings"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

const faceColumns = `f.id,f.person_id,p.name,f.path,f.x,f.y,f.width,f.height,f.manual,f.ignored,f.favorite,p.name_source`

func scanFace(s interface{ Scan(...any) error }) (RecognizedFace, error) {
	var f RecognizedFace
	err := s.Scan(&f.ID, &f.PersonID, &f.Name, &f.Path, &f.X, &f.Y, &f.Width, &f.Height, &f.Manual, &f.Ignored, &f.Favorite, &f.nameSource)
	return f, err
}

func (l *Library) Face(ctx context.Context, id int64) (RecognizedFace, error) {
	f, err := scanFace(l.index.db.QueryRowContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.id=? AND m.admin_only=0`, id))
	if err != nil {
		return f, err
	}
	if err := l.refreshFaceSource(ctx, f.Path); err != nil {
		return f, err
	}
	var exists int
	if err = l.index.db.QueryRowContext(ctx, `SELECT 1 FROM photo_faces WHERE id=?`, id).Scan(&exists); err != nil {
		return f, err
	}
	return f, nil
}

// Refresh the fingerprint strictly before using stored face coordinates. A failed
// index write must not leave regions from a replaced source usable.
func (l *Library) refreshFaceSource(ctx context.Context, path string) error {
	private, err := l.MediaAdminOnly(path)
	if err != nil {
		return err
	}
	if private {
		return ErrAdminOnly()
	}
	// Surface index write failures: stale regions must not survive a replaced file.
	media, changed, err := l.mediaFromPathData(path)
	if err != nil {
		return err
	}
	if media.AdminOnly {
		return ErrAdminOnly()
	}
	if media.Type != MediaTypeImage {
		return sql.ErrNoRows
	}
	if changed {
		if err := l.saveMediaContext(ctx, media); err != nil {
			return err
		}
	}
	return nil
}

func (l *Library) AutomaticFaces(ctx context.Context, path string) ([]RecognizedFace, error) {
	private, err := l.MediaAdminOnly(path)
	if err != nil {
		return nil, err
	}
	if private {
		return nil, nil
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id WHERE f.path=? AND f.ignored=0 ORDER BY f.id`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	faces := []RecognizedFace{}
	for rows.Next() {
		f, e := scanFace(rows)
		if e != nil {
			return nil, e
		}
		faces = append(faces, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	l.sanitizeFaceNames(faces)
	return faces, nil
}

func (l *Library) People(ctx context.Context, id int64, page int, q string, knownOnly, unknownOnly bool) (PeoplePage, error) {
	out := PeoplePage{Query: q, PersonID: id, Page: max(1, page), People: []Person{}, KnownOnly: knownOnly && !unknownOnly && id == 0, UnknownOnly: unknownOnly && id == 0}
	out.HasPrev = out.Page > 1
	if err := l.refreshPeoplePageVisibility(ctx, id, q, out.KnownOnly, out.UnknownOnly); err != nil {
		return out, err
	}
	if id == 0 {
		pattern := searchtext.LikeContainsPattern(searchtext.GermanFold(q))
		knownFilter := ""
		if out.KnownOnly {
			knownFilter = " AND p.name <> ''"
		} else if out.UnknownOnly {
			// The folded-name equality uses the existing name/order index.
			knownFilter = " AND p.name_fold = '' AND p.name = ''"
		}
		var total int
		if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_people p WHERE p.name_fold LIKE ? ESCAPE '\'`+knownFilter+` AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=p.id AND ignored=0)`, pattern).Scan(&total); err != nil {
			return out, err
		}
		out.setTotal(total)
		// Materialize the page first: portrait geometry and directory need at most
		// 61 primary-key lookups, regardless of the number of groups or page offset.
		rows, err := l.index.db.QueryContext(ctx, `WITH page AS MATERIALIZED (
 SELECT p.id,p.name,p.name_fold,
 (SELECT count(DISTINCT path) FROM photo_faces WHERE person_id=p.id AND ignored=0) AS photo_count,
 (SELECT min(id) FROM photo_faces WHERE person_id=p.id AND ignored=0) AS face_id
 FROM photo_people p WHERE p.name_fold LIKE ? ESCAPE '\'`+knownFilter+`
 AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=p.id AND ignored=0)
 ORDER BY p.name_fold,p.id LIMIT 61 OFFSET ?)
 SELECT p.id,p.name,p.photo_count,f.id,f.path,f.x,f.y,f.width,f.height
 FROM page p JOIN photo_faces f ON f.id=p.face_id ORDER BY p.name_fold,p.id`, pattern, (out.Page-1)*60)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var p Person
			var source string
			p.Portrait = &FaceRegion{}
			if err = rows.Scan(&p.ID, &p.Name, &p.Count, &p.FaceID, &source, &p.Portrait.X, &p.Portrait.Y, &p.Portrait.Width, &p.Portrait.Height); err != nil {
				return out, err
			}
			p.Directory = path.Dir(source)
			if p.Directory == "." {
				p.Directory = "Fotos"
			}
			out.People = append(out.People, p)
		}
		if len(out.People) > 60 {
			out.HasNext = true
			out.People = out.People[:60]
		}
		return out, rows.Err()
	}
	err := l.index.db.QueryRowContext(ctx, `SELECT name FROM photo_people WHERE id=? AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=? AND ignored=0)`, id, id).Scan(&out.Name)
	if err != nil {
		return out, err
	}
	var total int
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.ignored=0 AND m.admin_only=0`, id).Scan(&total); err != nil {
		return out, err
	}
	out.setTotal(total)
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.ignored=0 AND m.admin_only=0 ORDER BY f.id LIMIT 61 OFFSET ?`, id, (out.Page-1)*60)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		f, e := scanFace(rows)
		if e != nil {
			return out, e
		}
		out.Faces = append(out.Faces, f)
	}
	if len(out.Faces) > 60 {
		out.HasNext = true
		out.Faces = out.Faces[:60]
	}
	return out, rows.Err()
}

// IgnoredFaces lists individual ignored detections, including groups with no
// active faces. Pagination happens in SQLite, using the partial ignored index.
func (l *Library) IgnoredFaces(ctx context.Context, page int, q string, knownOnly bool) (PeoplePage, error) {
	out := PeoplePage{Query: q, Page: max(1, page), People: []Person{}, Faces: []RecognizedFace{}, KnownOnly: knownOnly, IgnoredOnly: true}
	out.HasPrev = out.Page > 1
	if err := l.refreshPeoplePageVisibility(ctx, 0, q, knownOnly, false); err != nil {
		return out, err
	}
	knownFilter := ""
	if knownOnly {
		knownFilter = " AND p.name <> ''"
	}
	var total int
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.ignored=1 AND m.admin_only=0 AND p.name_fold LIKE ? ESCAPE '\'`+knownFilter, searchtext.LikeContainsPattern(searchtext.GermanFold(q))).Scan(&total); err != nil {
		return out, err
	}
	out.setTotal(total)
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.ignored=1 AND m.admin_only=0 AND p.name_fold LIKE ? ESCAPE '\'`+knownFilter+` ORDER BY f.id LIMIT 61 OFFSET ?`, searchtext.LikeContainsPattern(searchtext.GermanFold(q)), (out.Page-1)*60)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return out, err
		}
		out.Faces = append(out.Faces, f)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	if len(out.Faces) > 60 {
		out.HasNext = true
		out.Faces = out.Faces[:60]
	}
	l.sanitizeFaceNames(out.Faces)
	return out, nil
}

func (l *Library) RenamePerson(ctx context.Context, id int64, name string) error {
	name, err := normalizedPersonName(name)
	if err != nil {
		return err
	}
	if err = l.refreshPersonIDsVisibility(ctx, id); err != nil {
		return err
	}
	res, err := l.index.db.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=? AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=? AND ignored=0)`, name, searchtext.GermanFold(name), id, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// EditFaces is atomic: either all selected faces can be edited, or none are changed.
// target=0 creates a new group; ignore=true marks reversible false detections.

func (l *Library) EditFaces(ctx context.Context, ids []int64, target int64, ignore bool, name string) error {
	if len(ids) == 0 || len(ids) > 500 {
		return errors.New("1 bis 500 Gesichter auswählen")
	}
	name, err := normalizedPersonName(name)
	if err != nil {
		return err
	}
	args := make([]any, 1, len(ids)+1)
	args[0] = target
	for _, id := range ids {
		args = append(args, id)
	}
	if err = l.refreshPeopleVisibility(ctx, `p.id=? OR p.id IN (SELECT person_id FROM photo_faces WHERE id IN (`+sqlutil.Placeholders(len(ids))+`))`, args...); err != nil {
		return err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	affected := map[int64]bool{}
	for _, id := range ids {
		f, e := l.Face(ctx, id)
		if e != nil {
			return e
		}
		affected[f.PersonID] = true
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !ignore {
		if target == 0 {
			res, e := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
			if e != nil {
				return e
			}
			target, _ = res.LastInsertId()
		} else {
			var exists int
			if e := tx.QueryRowContext(ctx, `SELECT 1 FROM photo_people WHERE id=? AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=? AND ignored=0)`, target, target).Scan(&exists); e != nil {
				return e
			}
		}
		affected[target] = true
	}
	for _, id := range ids {
		if ignore {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE id=?`, id)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1,ignored=0 WHERE id=?`, target, id)
		}
		if err != nil {
			return err
		}
	}
	for p := range affected {
		if err = refreshFaceReferencesTx(ctx, tx, p); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	l.faceRuntime.graph = nil
	return nil
}

func (l *Library) MergePeople(ctx context.Context, source, target int64, additional ...int64) error {
	return l.mergePeople(ctx, source, target, nil, additional...)
}

// MergePeopleNamed atomically merges groups and gives the target a manual name.
func (l *Library) MergePeopleNamed(ctx context.Context, source, target int64, name string, additional ...int64) error {
	name, err := normalizedPersonName(name)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("Namen eingeben")
	}
	return l.mergePeople(ctx, source, target, &name, additional...)
}

func (l *Library) mergePeople(ctx context.Context, source, target int64, name *string, additional ...int64) error {
	sources := []int64{source}
	seen := map[int64]bool{source: true}
	for _, id := range additional {
		if !seen[id] {
			sources = append(sources, id)
			seen[id] = true
		}
	}
	if target <= 0 || len(sources) > 60 {
		return errors.New("höchstens 60 verschiedene Quellpersonen auswählen")
	}
	for _, id := range sources {
		if id <= 0 || id == target {
			return errors.New("verschiedene Personen auswählen")
		}
	}
	if err := l.refreshPersonIDsVisibility(ctx, append(append([]int64{}, sources...), target)...); err != nil {
		return err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range append(append([]int64{}, sources...), target) {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT 1 FROM photo_people WHERE id=? AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=? AND ignored=0)`, id, id).Scan(&exists); err != nil {
			return err
		}
	}
	if name != nil {
		if _, err = tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=?`, *name, searchtext.GermanFold(*name), target); err != nil {
			return err
		}
	}
	for _, id := range sources {
		// Preserve a source name when the explicitly selected target is unnamed.
		// Copy its provenance as well so imported names retain their visibility rules.
		if _, err = tx.ExecContext(ctx, `UPDATE photo_people SET (name,name_fold,manual_name,name_source)=(SELECT name,name_fold,manual_name,name_source FROM photo_people WHERE id=?) WHERE id=? AND name='' AND EXISTS(SELECT 1 FROM photo_people WHERE id=? AND name<>'')`, id, target, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE person_id=?`, target, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM photo_people WHERE id=?`, id); err != nil {
			return err
		}
		if err = refreshFaceReferencesTx(ctx, tx, id); err != nil {
			return err
		}
	}
	if err = refreshFaceReferencesTx(ctx, tx, target); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	l.faceRuntime.graph = nil
	return nil
}

// AddAutomaticFaces decorates only the returned page, not all search candidates.

func (l *Library) AddAutomaticFaces(ctx context.Context, items []Media) error {
	paths := make([]string, len(items))
	for i := range items {
		paths[i] = items[i].Path
	}
	auto, err := l.automaticFacesBatch(ctx, paths)
	if err != nil {
		return err
	}
	for i := range items {
		if len(auto[items[i].Path]) == 0 {
			continue
		}
		private, e := l.MediaAdminOnly(items[i].Path)
		if e != nil || private {
			continue
		}
		l.sanitizeFaceNames(auto[items[i].Path])
		items[i].AutomaticFaces = auto[items[i].Path]
	}
	return nil
}

func queryHasPerson(q string) bool {
	for _, g := range parseQueryExpression(q).Groups {
		for _, n := range g {
			if n.Term.Field == "person" {
				return true
			}
			for _, t := range n.NOfTerms {
				if t.Field == "person" {
					return true
				}
			}
		}
	}
	return false
}

func recognizedFaceNames(f []RecognizedFace) string {
	names := []string{}
	for _, v := range f {
		names = append(names, v.Name)
	}
	return strings.Join(names, " ")
}

func (l *Library) automaticFacesBatch(ctx context.Context, paths []string) (map[string][]RecognizedFace, error) {
	out := map[string][]RecognizedFace{}
	if l == nil || !l.index.available() {
		return out, nil
	}
	for start := 0; start < len(paths); start += 200 {
		end := min(start+200, len(paths))
		args := []any{}
		for _, p := range paths[start:end] {
			args = append(args, p)
		}
		rows, err := l.index.db.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE f.path IN (`+strings.TrimRight(strings.Repeat("?,", len(args)), ",")+`) AND f.ignored=0 AND m.admin_only=0 ORDER BY f.id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			f, e := scanFace(rows)
			if e != nil {
				rows.Close()
				return nil, e
			}
			out[f.Path] = append(out[f.Path], f)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (l *Library) sanitizeFaceNames(faces []RecognizedFace) {
	checked := map[string]bool{}
	for i := range faces {
		source := faces[i].nameSource
		if source == "" {
			continue
		}
		private, ok := checked[source]
		if !ok {
			var err error
			private, err = l.MediaAdminOnly(source)
			private = private || err != nil
			checked[source] = private
		}
		if private {
			faces[i].Name = ""
		}
	}
}
