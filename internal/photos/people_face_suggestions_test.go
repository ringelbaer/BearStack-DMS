package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSuggestPeopleForFaceRanksNamedGroupsAndProtectsSources(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "near/b.jpg", "far/c.jpg", "unknown/d.jpg")
	for i := 0; i < 4; i++ {
		finishFace(t, l, i)
	}
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "near/b.jpg")
	c, _ := l.AutomaticFaces(ctx, "far/c.jpg")
	for _, named := range []struct {
		id   int64
		name string
	}{{b[0].PersonID, "Near"}, {c[0].PersonID, "Far"}} {
		if err := l.RenamePerson(ctx, named.id, named.name); err != nil {
			t.Fatal(err)
		}
	}
	for i, score := range []float64{.8, .6, 1} {
		v := faceDetection(i + 1).Embedding
		v[0] = float32(score)
		v[i+1] = float32(math.Sqrt(1 - score*score))
		if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=?`, encodeVector(v), []int64{b[0].ID, c[0].ID, 4}[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.index.db.Exec(`UPDATE photo_face_state SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	got, err := l.SuggestPeopleForFace(ctx, a[0].ID)
	if err != nil || len(got.People) != 2 || got.People[0].Name != "Near" || got.People[1].Name != "Far" || got.People[0].FaceID != b[0].ID || got.HasNext {
		t.Fatalf("ranked=%+v %v", got, err)
	}
	if err := l.RenamePerson(ctx, a[0].PersonID, "Source"); err != nil {
		t.Fatal(err)
	}
	got, err = l.SuggestPeopleForFace(ctx, a[0].ID)
	if err != nil || len(got.People) != 2 {
		t.Fatalf("source suggested: %+v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "near", ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = l.SuggestPeopleForFace(ctx, a[0].ID)
	if err != nil || len(got.People) != 1 || got.People[0].Name != "Far" {
		t.Fatalf("private candidate leaked: %+v %v", got, err)
	}
	if err := l.EditFaces(ctx, []int64{a[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SuggestPeopleForFace(ctx, a[0].ID); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("ignored source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SuggestPeopleForFace(ctx, a[0].ID); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private source: %v", err)
	}
}

func TestSuggestPeopleForFaceEmptyAndCancelled(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	got, err := l.SuggestPeopleForFace(context.Background(), 1)
	if err != nil || len(got.People) != 0 || got.People == nil {
		t.Fatalf("empty=%+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = l.SuggestPeopleForFace(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSuggestPeopleForFaceLimitsNamedRanking(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<50)
 INSERT INTO photo_people(name,name_fold) SELECT CASE WHEN x>25 THEN printf('Named %d',x) ELSE '' END,CASE WHEN x>25 THEN printf('named %d',x) ELSE '' END FROM n;
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT f.path,f.directory,p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p CROSS JOIN photo_faces f WHERE f.id=1 AND p.id<>1;
 UPDATE photo_face_reference_settings SET pending=1,cursor=0 WHERE id=1;
 UPDATE photo_face_state SET revision=revision+1 WHERE id=1;`); err != nil {
		t.Fatal(err)
	}
	got, err := l.SuggestPeopleForFace(ctx, 1)
	if err != nil || len(got.People) != 20 {
		t.Fatalf("bounded named candidates: %+v %v", got, err)
	}
	var snapshots []PeopleSuggestions
	streamed, err := l.SuggestPeopleForFaceStream(ctx, 1, func(update PeopleSuggestions) error { snapshots = append(snapshots, update); return nil })
	if err != nil || !reflect.DeepEqual(streamed, got) || len(snapshots) < 2 || len(snapshots[0].People) != 1 {
		t.Fatalf("not incremental or ranking differs: snapshots=%+v result=%+v err=%v", snapshots, streamed, err)
	}
	stopped := errors.New("consumer stopped")
	calls := 0
	_, err = l.SuggestPeopleForFaceStream(ctx, 1, func(PeopleSuggestions) error { calls++; return stopped })
	if !errors.Is(err, stopped) || calls != 1 {
		t.Fatalf("consumer cancellation: %d %v", calls, err)
	}

	for i, p := range got.People {
		if p.Name == "" || p.ID != int64(27+i) {
			t.Fatalf("unstable ranking: %+v", got)
		}
	}
}
