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
		people, err := l.index.labelPeople(ctx, ids, scope, query, 21-len(out.People))
		if err != nil {
			return out, err
		}
		for _, person := range people {
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
