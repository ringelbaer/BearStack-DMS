package photos

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFavoritePortraitsSharedByWebAndAndroid(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "first/a.jpg", "second/b.jpg", "third/c.jpg")
	for range 3 {
		finishFace(t, l, 0)
	}
	if err := l.RenamePerson(ctx, 1, "Anna"); err != nil {
		t.Fatal(err)
	}
	check := func(want int64, count int) {
		t.Helper()
		for _, sort := range []string{"name_asc", "name_desc", "count_asc", "count_desc", "folder_asc", "folder_desc", "date_asc", "date_desc"} {
			page, err := l.People(ctx, 0, 1, "Anna", true, false, sort)
			if err != nil || len(page.People) != 1 {
				t.Fatalf("overview: %+v %v", page, err)
			}
			p := page.People[0]
			f, err := l.Face(ctx, want)
			if err != nil || p.FaceID != want || p.SearchFaceID != 1 || p.Count != count || p.DisplayPath != mediaDisplayPath(f.Path) || p.Portrait == nil || *p.Portrait != (FaceRegion{f.X, f.Y, f.Width, f.Height}) {
				t.Fatalf("overview portrait: %+v %v", p, err)
			}
		}
		web, err := l.SuggestPeople(ctx, "Anna")
		if err != nil || len(web.People) != 1 || web.People[0].FaceID != want || web.People[0].Count != count {
			t.Fatalf("web picker: %+v %v", web, err)
		}
		for _, exact := range []bool{false, true} {
			app, err := l.LabelSuggestions(ctx, "Anna", exact)
			if err != nil || len(app) != 1 || app[0].FaceID != want || app[0].Count != int64(count) {
				t.Fatalf("app picker: %+v %v", app, err)
			}
		}
		named, err := l.LabelNamedPeople(ctx, 0, 1)
		if err != nil || len(named.People) != 1 || named.People[0].FaceID != want {
			t.Fatalf("app overview: %+v %v", named, err)
		}
	}
	check(1, 3)
	if _, err := l.SetFaceFavorite(ctx, 3, 1, true); err != nil {
		t.Fatal(err)
	}
	check(3, 3)
	if _, err := l.SetFaceFavorite(ctx, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	check(2, 3)
	// Detail pagination and the face used to start a naming search stay unchanged.
	detail, err := l.LabelPerson(ctx, 1, 0)
	if err != nil || detail.FaceID != 1 || len(detail.Faces) != 3 || detail.Faces[0].ID != 1 {
		t.Fatalf("detail changed: %+v %v", detail, err)
	}
	if _, err := l.SetFaceFavorite(ctx, 2, 1, false); err != nil {
		t.Fatal(err)
	}
	check(3, 3)
	if err := os.WriteFile(filepath.Join(l.Root(), "third/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	check(1, 2)
	if _, err := l.SetFaceFavorite(ctx, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	check(2, 2)
	if err := l.EditFaces(ctx, []int64{2}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	check(1, 1)
}

func TestFavoritePortraitBeyondDetailPageUsesSparseIndex(t *testing.T) {
	l, p, s := labelFixture(t)
	ctx := context.Background()
	if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<20000)
 INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model)
 SELECT f.person_id,f.path,f.directory,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM n CROSS JOIN photo_faces f WHERE f.id=1;
 UPDATE photo_faces SET favorite=1 WHERE id=(SELECT max(id) FROM photo_faces)`); err != nil {
		t.Fatal(err)
	}
	list, err := l.LabelNamedPeople(ctx, 0, s.UpperID)
	if err != nil || len(list.People) != 1 || list.People[0].FaceID != 20003 || list.People[0].Count != 20003 {
		t.Fatalf("late favorite: %+v %v", list, err)
	}
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT `+personPortraitSQL+` FROM photo_people p WHERE id=?`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if err := rows.Err(); err != nil || !strings.Contains(plan.String(), "idx_face_favorites") || strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Fatalf("portrait plan: %s %v", plan.String(), err)
	}
}

func TestFavoriteSuggestionPortraitDoesNotChangeMatching(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a-source.jpg", "b-witness.jpg", "c-star.jpg", "d-other.jpg")
	for axis := range 4 {
		finishFace(t, l, axis)
	}
	if err := l.MergePeople(ctx, 3, 2); err != nil {
		t.Fatal(err)
	}
	for id, name := range map[int64]string{2: "Near", 4: "Far"} {
		if err := l.RenamePerson(ctx, id, name); err != nil {
			t.Fatal(err)
		}
	}
	for id, score := range map[int64]float64{2: .9, 4: .6} {
		v := faceDetection(int(id)).Embedding
		v[0] = float32(score)
		v[id] = float32(math.Sqrt(1 - score*score))
		if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=?`, encodeVector(v), id); err != nil {
			t.Fatal(err)
		}
	}
	// The favorite has zero similarity to the source. A nonfavorite must still
	// supply the winning score; only the portrait in the response may change.
	if _, err := l.SetFaceFavorite(ctx, 3, 2, true); err != nil {
		t.Fatal(err)
	}
	beforeRefs := selectedReferences(t, l)
	detail, err := l.LabelPerson(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertResult := func(out PeopleSuggestions) {
		t.Helper()
		if len(out.People) != 2 || out.People[0].ID != 2 || out.People[0].FaceID != 3 || out.People[1].ID != 4 {
			t.Fatalf("changed matching or wrong portrait: %+v", out)
		}
	}
	out, err := l.SuggestPeopleForFace(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(out)
	streamed, err := l.SuggestPeopleForFaceStream(ctx, 1, func(update PeopleSuggestions) error {
		for _, p := range update.People {
			if p.ID == 2 && p.FaceID != 3 {
				t.Fatalf("nonfavorite streaming portrait: %+v", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(streamed)
	after, err := l.LabelPerson(ctx, 2, 0)
	if err != nil || !reflect.DeepEqual(after, detail) || !reflect.DeepEqual(selectedReferences(t, l), beforeRefs) {
		t.Fatal("presentation mutated groups or matching references")
	}
}
