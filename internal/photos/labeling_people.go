package photos

import (
	"context"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

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

// LabelNamedPeople walks IDs rather than offsets or mutable names. Visibility
// checks and face counts are limited to small batches, including when private
// directories remove candidates during the request. No full-list count is needed.
func (l *Library) LabelNamedPeople(ctx context.Context, after, upper int64, query ...string) (LabelCandidates, error) {
	filter := ""
	var queryArgs []any
	if len(query) > 0 && query[0] != "" {
		filter = ` AND p.name_fold LIKE ? ESCAPE '\'`
		queryArgs = []any{searchtext.LikeContainsPattern(searchtext.GermanFold(query[0]))}
	}
	return l.labelPeopleList(ctx, after, upper, labelPeopleNamed, filter, queryArgs)
}

func (l *Library) labelPeopleList(ctx context.Context, after, upper int64, scope labelPeopleScope, filter string, queryArgs []any) (LabelCandidates, error) {
	candidates, names := scope.predicates()
	out := LabelCandidates{People: []LabelPerson{}, Next: after}
	cursor := after
	for len(out.People) < 21 {
		rows, err := l.index.db.QueryContext(ctx, `SELECT p.id FROM photo_people p WHERE `+candidates+` AND p.id>? AND p.id<=? AND `+labelExists+filter+` ORDER BY p.id LIMIT 21`, append([]any{cursor, upper}, queryArgs...)...)
		if err != nil {
			return out, err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return out, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || len(ids) == 0 {
			return out, err
		}
		if err = l.refreshPersonIDsVisibility(ctx, ids...); err != nil {
			return out, err
		}
		// Restrict to the checked IDs, even if another client names a group now.
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		rows, err = l.index.db.QueryContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id IN (`+sqlutil.Placeholders(len(ids))+`) AND `+names+` AND `+labelExists+filter+` ORDER BY p.id`, append(args, queryArgs...)...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			p, err := scanLabel(rows)
			if err != nil {
				rows.Close()
				return out, err
			}
			out.People = append(out.People, p)
			if len(out.People) == 21 {
				out.HasNext = true
				out.People = out.People[:20]
				out.Next = out.People[19].ID
				rows.Close()
				return out, nil
			}
			out.Next = p.ID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		cursor = ids[len(ids)-1]
		if len(ids) < 21 {
			break
		}
	}
	return out, nil
}
