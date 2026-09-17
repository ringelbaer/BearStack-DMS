package photos

import (
	"context"
	"database/sql"
	"encoding/json"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

const labelColumns = `p.id,p.name,r.revision,(SELECT count(*) FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0),(SELECT min(id) FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0)`
const labelPreviewColumns = `p.id,p.name,r.revision,(SELECT count(*) FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0),` + personPortraitSQL
const labelFrom = ` FROM photo_people p JOIN photo_person_revisions r ON r.person_id=p.id `
const labelExists = ` EXISTS(SELECT 1 FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0) `

func scanLabel(row interface{ Scan(...any) error }) (LabelPerson, error) {
	var p LabelPerson
	err := row.Scan(&p.ID, &p.Name, &p.Revision, &p.Count, &p.FaceID)
	return p, err
}

// indexedLabelFace stays inside the photo module; paths never enter API JSON.
type indexedLabelFace struct {
	PersonID        int64
	Face            LabelFace
	Path            string
	Size, Modified  int64
	ContentRevision int64
}

func (s *photoIndexStore) labelIdentity(ctx context.Context, out *LabelSession) error {
	return s.db.QueryRowContext(ctx, `SELECT instance,dataset,(SELECT coalesce(max(id),0) FROM photo_people) FROM photo_labeling_identity WHERE id=1`).Scan(&out.Instance, &out.Dataset, &out.UpperID)
}

// Details and their count/revision use one read snapshot. No filesystem reads or
// display formatting occur while this transaction holds a connection.
func (s *photoIndexStore) labelPersonPage(ctx context.Context, id int64, offset, limit int, after int64) (LabelPerson, []indexedLabelFace, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LabelPerson{}, nil, err
	}
	defer tx.Rollback()
	person, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, id))
	if err != nil {
		return person, nil, err
	}
	person.Offset = offset
	rows, err := tx.QueryContext(ctx, `SELECT f.id,f.path,f.x,f.y,f.width,f.height,f.favorite,f.needs_review,f.source_revision,m.size_bytes,m.mod_time_unix_nano,coalesce((SELECT revision FROM photo_entities e WHERE e.id=f.entity_id),0) FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.ignored=0 AND f.id>? ORDER BY f.id LIMIT ? OFFSET ?`, id, after, limit, offset)
	if err != nil {
		return person, nil, err
	}
	defer rows.Close()
	faces := make([]indexedLabelFace, 0, limit)
	for rows.Next() {
		var row indexedLabelFace
		row.PersonID = id
		if err := rows.Scan(&row.Face.ID, &row.Path, &row.Face.Bounds.X, &row.Face.Bounds.Y, &row.Face.Bounds.Width, &row.Face.Bounds.Height, &row.Face.Favorite, &row.Face.NeedsReview, &row.Face.SourceRevision, &row.Size, &row.Modified, &row.ContentRevision); err != nil {
			return person, nil, err
		}
		faces = append(faces, row)
	}
	return person, faces, rows.Err()
}

func labelSuggestionFilter(q string, exact bool) (string, string) {
	if exact {
		name, _ := normalizedPersonName(q)
		return `p.name=?`, name
	}
	return `p.name_fold LIKE ? ESCAPE '\'`, searchtext.LikeContainsPattern(searchtext.GermanFold(q))
}

func (s *photoIndexStore) labelSuggestions(ctx context.Context, q string, exact bool) ([]LabelPerson, error) {
	filter, arg := labelSuggestionFilter(q, exact)
	rows, err := s.db.QueryContext(ctx, `SELECT `+labelPreviewColumns+labelFrom+` WHERE p.name<>'' AND `+filter+` AND `+labelExists+` ORDER BY p.name_fold,p.id LIMIT 20`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelPerson{}
	for rows.Next() {
		p, e := scanLabel(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *photoIndexStore) labelReceipt(ctx context.Context, actor, operation, dataset string) (LabelReceipt, error) {
	var out LabelReceipt
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT a.result FROM photo_labeling_actions a,photo_labeling_identity i WHERE a.actor=? AND a.operation_id=? AND i.dataset=?`, actor, operation, dataset).Scan(&encoded)
	if err == nil {
		err = json.Unmarshal([]byte(encoded), &out)
	}
	return out, err
}

type labelPeopleScope int

const (
	labelPeopleUnnamed labelPeopleScope = iota
	labelPeopleNamed
	labelPeopleAll
)

// Imported names can disappear when the name's source becomes private. Include
// these IDs in the preflight selection, then apply the requested scope again
// after checking visibility. Manual names outside the scope need no filesystem IO.
func (scope labelPeopleScope) predicates() (candidates, names string) {
	switch scope {
	case labelPeopleUnnamed:
		return `(p.name='' OR (p.manual_name=0 AND p.name_source<>''))`, `p.name=''`
	case labelPeopleNamed:
		return `p.name<>''`, `p.name<>''`
	default:
		return `1=1`, `1=1`
	}
}

func labelPeopleNameFilter(query string) (string, []any) {
	if query == "" {
		return "", nil
	}
	return ` AND p.name_fold LIKE ? ESCAPE '\'`, []any{searchtext.LikeContainsPattern(searchtext.GermanFold(query))}
}

func (s *photoIndexStore) labelPeopleIDs(ctx context.Context, after, upper int64, scope labelPeopleScope, query string) ([]int64, error) {
	candidates, _ := scope.predicates()
	filter, queryArgs := labelPeopleNameFilter(query)
	rows, err := s.db.QueryContext(ctx, `SELECT p.id FROM photo_people p WHERE `+candidates+` AND p.id>? AND p.id<=? AND `+labelExists+filter+` ORDER BY p.id LIMIT 21`, append([]any{after, upper}, queryArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0, 21)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *photoIndexStore) labelPeople(ctx context.Context, ids []int64, scope labelPeopleScope, query string, limit int) ([]LabelPerson, error) {
	_, names := scope.predicates()
	filter, queryArgs := labelPeopleNameFilter(query)
	args := make([]any, len(ids), len(ids)+len(queryArgs)+1)
	for i, id := range ids {
		args[i] = id
	}
	args = append(args, queryArgs...)
	args = append(args, limit)
	columns := labelPreviewColumns
	if scope == labelPeopleUnnamed {
		// Queue identities also seed naming searches; do not change their source face.
		columns = labelColumns
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+labelFrom+` WHERE p.id IN (`+sqlutil.Placeholders(len(ids))+`) AND `+names+` AND `+labelExists+filter+` ORDER BY p.id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	people := make([]LabelPerson, 0, limit)
	for rows.Next() {
		person, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		people = append(people, person)
	}
	return people, rows.Err()
}

func (s *photoIndexStore) labelPortraits(ctx context.Context, people []LabelPerson) ([]indexedLabelFace, error) {
	args := make([]any, len(people))
	for i, person := range people {
		args[i] = person.FaceID
	}
	rows, err := s.db.QueryContext(ctx, `SELECT f.person_id,f.id,f.path,f.x,f.y,f.width,f.height,f.favorite,f.needs_review,f.source_revision,m.size_bytes,m.mod_time_unix_nano,coalesce((SELECT revision FROM photo_entities e WHERE e.id=f.entity_id),0)
 FROM photo_faces f JOIN media_index m ON m.path=f.path
 WHERE f.id IN (`+sqlutil.Placeholders(len(args))+`) AND f.ignored=0 AND m.admin_only=0`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	faces := make([]indexedLabelFace, 0, len(people))
	for rows.Next() {
		var row indexedLabelFace
		if err := rows.Scan(&row.PersonID, &row.Face.ID, &row.Path, &row.Face.Bounds.X, &row.Face.Bounds.Y, &row.Face.Bounds.Width, &row.Face.Bounds.Height, &row.Face.Favorite, &row.Face.NeedsReview, &row.Face.SourceRevision, &row.Size, &row.Modified, &row.ContentRevision); err != nil {
			return nil, err
		}
		faces = append(faces, row)
	}
	return faces, rows.Err()
}
