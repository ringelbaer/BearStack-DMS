package photos

import "context"

// LabelNamedPeople walks stable IDs, with visibility checks only for the current
// candidate batches. Search strings are normalized by the index query layer.
func (l *Library) LabelNamedPeople(ctx context.Context, after, upper int64, query ...string) (LabelCandidates, error) {
	name := ""
	if len(query) > 0 {
		name = query[0]
	}
	return l.labelPeopleList(ctx, after, upper, labelPeopleNamed, name)
}

// LabelNamedPeopleWithFolders keeps exclusion-only named people reachable.
// Legacy clients retain the original nonempty list contract.
func (l *Library) LabelNamedPeopleWithFolders(ctx context.Context, after, upper int64, query string) (LabelCandidates, error) {
	return l.labelPeopleList(ctx, after, upper, labelPeopleNamedWithFolders, query)
}

func (l *Library) visiblePersonExclusion(ctx context.Context, id int64) (bool, error) {
	rows, err := l.index.db.QueryContext(ctx, `SELECT directory FROM person_folder_exclusions WHERE person_id=?`, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	visibility := newFaceDirectoryVisibility(l.root)
	for rows.Next() {
		var directory string
		if err := rows.Scan(&directory); err != nil {
			return false, err
		}
		if !visibility.private(directory) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (l *Library) labelPeopleList(ctx context.Context, after, upper int64, scope labelPeopleScope, query string) (LabelCandidates, error) {
	out := LabelCandidates{People: []LabelPerson{}, Next: after}
	cursor := after
	for len(out.People) < 21 {
		ids, err := l.index.labelPeopleIDs(ctx, cursor, upper, scope, query)
		if err != nil || len(ids) == 0 {
			return out, err
		}
		if err = l.refreshPersonIDsVisibility(ctx, ids...); err != nil {
			return out, err
		}
		// Read only IDs whose directories and imported name sources were checked.
		limit := 21 - len(out.People)
		if scope == labelPeopleNamedWithFolders {
			// Exclusion-only entries are filtered against live directory visibility
			// below. Read this entire bounded batch so hidden entries cannot make
			// the ID cursor skip unexamined visible people.
			limit = 21
		}
		people, err := l.index.labelPeople(ctx, ids, scope, query, limit)
		if err != nil {
			return out, err
		}
		for _, person := range people {
			if scope == labelPeopleNamedWithFolders && person.Count == 0 {
				visible, err := l.visiblePersonExclusion(ctx, person.ID)
				if err != nil {
					return out, err
				}
				if !visible {
					continue
				}
			}
			out.People = append(out.People, person)
			if len(out.People) == 21 {
				out.HasNext = true
				out.People = out.People[:20]
				out.Next = out.People[19].ID
				return out, nil
			}
			out.Next = person.ID
		}
		cursor = ids[len(ids)-1]
		if len(ids) < 21 {
			break
		}
	}
	return out, nil
}
