package photos

import (
	"context"
	"fmt"
	"testing"
)

func traceBatchCount(trace *ListTrace, name string) int {
	count := 0
	for _, step := range trace.Snapshot().Steps {
		if step.Name == name {
			count++
		}
	}
	return count
}

func TestPostFilterLoadsFacesOnlyForPersonPredicates(t *testing.T) {
	l := routeTestLibrary(t)
	for _, statement := range []string{
		`INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at) VALUES('IM_a.jpg','IM_a.jpg','','image','image/jpeg',1,1,'2024-01-01',''),('IM_b.jpg','IM_b.jpg','','image','image/jpeg',1,2,'2024-01-02','')`,
		`INSERT INTO photo_people(id,name,name_fold,manual_name) VALUES(1,'Ada','ada',1)`,
		`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model) VALUES(1,'IM_a.jpg','',.1,.1,.2,.2,.99,zeroblob(512),'test')`,
	} {
		if _, err := l.index.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query         string
		want, batches int
	}{
		{"IM", 2, 0}, {"person:Ada", 1, 1}, {"-person:Ada", 1, 1},
	} {
		t.Run(tc.query, func(t *testing.T) {
			trace := NewListTrace()
			opts := indexMediaOptions{Query: tc.query, Limit: 96, LeanMetadata: true}
			items, total, err := l.indexMediaPostFilter(ContextWithListTrace(context.Background(), trace), opts, "", nil, false)
			if err != nil || total != tc.want || len(items) != tc.want {
				t.Fatalf("items=%v total=%d err=%v", items, total, err)
			}
			if got := traceBatchCount(trace, "photos.faces.batch"); got != tc.batches {
				t.Fatalf("face queries=%d want=%d", got, tc.batches)
			}
		})
	}
}

func TestContentStatesBatchAcrossGroupsPreservesDuplicatesAndReview(t *testing.T) {
	l := routeTestLibrary(t)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<205)
 INSERT INTO photo_entities(id,kind,path,cache_path,revision,missing_since)
 SELECT 10000+x,'image',printf('photo-%03d.jpg',x),'',3,CASE WHEN x=205 THEN 1 ELSE 0 END FROM n`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_people(id,name,name_fold) VALUES(1,'Ada','ada');
 INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model,entity_id,needs_review)
 VALUES(1,'photo-001.jpg','',.1,.1,.2,.2,.99,zeroblob(512),'test',10001,1)`); err != nil {
		t.Fatal(err)
	}
	groups := [][]Media{nil, {{Path: ""}}, {{Path: "absent.jpg"}}}
	for i := 1; i <= 205; i++ {
		groups = append(groups, []Media{{Path: fmt.Sprintf("photo-%03d.jpg", i)}, {Path: "photo-001.jpg"}})
	}
	trace := NewListTrace()
	if err := l.AddContentStates(ContextWithListTrace(context.Background(), trace), groups...); err != nil {
		t.Fatal(err)
	}
	if got := traceBatchCount(trace, "photos.identity.content_states_batch"); got != 2 {
		t.Fatalf("queries=%d, want 2", got)
	}
	for i := 1; i <= 205; i++ {
		group := groups[i+2]
		want := int64(10000 + i)
		if i == 205 {
			want = 0
		}
		if group[0].EntityID != want || (want != 0 && group[0].ContentRevision != 3) {
			t.Fatalf("identity %d: %+v", i, group[0])
		}
		if group[1].EntityID != 10001 || group[1].ContentRevision != 3 || !group[1].NeedsReview {
			t.Fatalf("duplicate %d: %+v", i, group[1])
		}
	}
	if groups[1][0].EntityID != 0 || groups[2][0].EntityID != 0 {
		t.Fatal("unknown preview gained identity")
	}
}
