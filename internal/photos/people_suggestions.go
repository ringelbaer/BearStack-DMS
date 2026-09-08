package photos

import (
	"context"

	"bearstack/internal/searchtext"
)

type PersonSuggestion struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	FaceID int64  `json:"face_id"`
}

type PeopleSuggestions struct {
	People  []PersonSuggestion `json:"people"`
	HasNext bool               `json:"has_next"`
}

// SuggestPeople serves the web picker with the same names, photo counts, portraits and
// ordering as the known-people overview. One extra row replaces its global count;
// current marker/name-source checks still run before querying visible groups.
func (l *Library) SuggestPeople(ctx context.Context, q string) (PeopleSuggestions, error) {
	out := PeopleSuggestions{People: []PersonSuggestion{}}
	if err := l.refreshPeoplePageVisibility(ctx, 0, q, true, false); err != nil {
		return out, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT p.id,p.name,
 (SELECT count(DISTINCT path) FROM photo_faces WHERE person_id=p.id AND ignored=0),
 (SELECT min(id) FROM photo_faces WHERE person_id=p.id AND ignored=0)
 FROM photo_people p WHERE p.name<>'' AND p.name_fold LIKE ? ESCAPE '\'
 AND EXISTS(SELECT 1 FROM photo_faces WHERE person_id=p.id AND ignored=0)
 ORDER BY p.name_fold,p.id LIMIT 61`, searchtext.LikeContainsPattern(searchtext.GermanFold(q)))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var person PersonSuggestion
		if err := rows.Scan(&person.ID, &person.Name, &person.Count, &person.FaceID); err != nil {
			return out, err
		}
		out.People = append(out.People, person)
	}
	if len(out.People) > 60 {
		out.HasNext = true
		out.People = out.People[:60]
	}
	return out, rows.Err()
}
