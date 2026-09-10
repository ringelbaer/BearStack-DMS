package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/searchtext"
)

func peopleSortFixture(t *testing.T) *Library {
	t.Helper()
	l := faceLibrary(t, "Zeta/old.jpg", "other/new.jpg", "Älbum/one.jpg", "Mitte/one.jpg", "Mitte/two.jpg", "Mitte/three.jpg", "root.jpg", "ignored/a.jpg", "ignored/b.jpg")
	for i, name := range []string{"Zoe", "Änne", "Bert", ""} {
		if _, err := l.index.db.Exec(`INSERT INTO photo_people(id,name,name_fold,manual_name) VALUES(?,?,?,1)`, i+1, name, searchtext.GermanFold(name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, photo := range []struct{ path, captured string }{
		{"Zeta/old.jpg", "2020-01-01T00:00:00Z"}, {"other/new.jpg", "2024-01-02T00:15:30+14:00"},
		{"Älbum/one.jpg", "2024-01-01T10:20:00Z"}, {"root.jpg", "2026-01-01T00:00:00Z"},
		{"Mitte/one.jpg", ""}, {"Mitte/two.jpg", ""}, {"Mitte/three.jpg", "invalid"},
		{"ignored/a.jpg", "2030-01-01T00:00:00Z"}, {"ignored/b.jpg", "2031-01-01T00:00:00Z"},
	} {
		if _, err := l.index.db.Exec(`UPDATE media_index SET captured_at=?,mod_time_unix_nano=? WHERE path=?`, photo.captured, time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(), photo.path); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []struct {
		person  int
		path    string
		ignored bool
	}{
		{1, "Zeta/old.jpg", false}, {1, "other/new.jpg", false}, {1, "other/new.jpg", false},
		{2, "Älbum/one.jpg", false}, {3, "Mitte/one.jpg", false}, {3, "Mitte/two.jpg", false}, {3, "Mitte/three.jpg", false},
		{4, "root.jpg", false}, {1, "ignored/a.jpg", true}, {1, "ignored/b.jpg", true}, {2, "ignored/a.jpg", true},
	} {
		dir := filepath.ToSlash(filepath.Dir(f.path))
		if dir == "." {
			dir = ""
		}
		if _, err := l.index.db.Exec(`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model,ignored) VALUES(?,?,?,.1,.1,.2,.2,.99,?,?,?)`, f.person, f.path, dir, encodeVector(faceDetection(0).Embedding), facerec.Model, f.ignored); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func personIDs(page PeoplePage) []int64 {
	ids := make([]int64, len(page.People))
	for i, p := range page.People {
		ids[i] = p.ID
	}
	return ids
}

func TestPeopleSortAllKeysDirectionsAndPhotoSemantics(t *testing.T) {
	l := peopleSortFixture(t)
	for _, tc := range []struct {
		key string
		ids []int64
	}{
		{"name", []int64{4, 2, 3, 1}}, {"count", []int64{2, 4, 1, 3}},
		{"folder", []int64{2, 4, 3, 1}}, {"date", []int64{3, 1, 2, 4}},
	} {
		for _, direction := range []string{"asc", "desc"} {
			t.Run(tc.key+"_"+direction, func(t *testing.T) {
				want := slices.Clone(tc.ids)
				if direction == "desc" {
					slices.Reverse(want)
				}
				page, err := l.People(context.Background(), 0, 1, "", false, false, tc.key+"_"+direction)
				if err != nil || !reflect.DeepEqual(personIDs(page), want) || page.Sort != tc.key+"_"+direction {
					t.Fatalf("got %v, want %v: %v", personIDs(page), want, err)
				}
				for _, person := range page.People {
					if person.ID == 1 && (person.Count != 2 || person.Directory != "Zeta" || person.FaceID != 1) {
						t.Fatalf("ignored/duplicate/portrait mismatch: %+v", person)
					}
				}
			})
		}
	}
	// Updating an indexed capture time changes the next result without any cache rebuild.
	if _, err := l.index.db.Exec(`UPDATE media_index SET captured_at='2040-01-01T00:00:00Z' WHERE path='other/new.jpg'`); err != nil {
		t.Fatal(err)
	}
	page, err := l.People(context.Background(), 0, 1, "", false, false, "date_desc")
	if err != nil || page.People[0].ID != 1 {
		t.Fatalf("date update: %+v %v", page, err)
	}
}

func TestPeopleSortFiltersVisibilityAndValidation(t *testing.T) {
	l := peopleSortFixture(t)
	ctx := context.Background()
	for _, sorting := range []string{"name_desc", "count_desc", "folder_desc", "date_desc"} {
		page, err := l.People(ctx, 0, 1, "Aenne", true, false, sorting)
		if err != nil || !reflect.DeepEqual(personIDs(page), []int64{2}) {
			t.Fatalf("search %s: %+v %v", sorting, page, err)
		}
		page, err = l.People(ctx, 0, 1, "", true, true, sorting)
		if err != nil || !reflect.DeepEqual(personIDs(page), []int64{4}) {
			t.Fatalf("unknown %s: %+v %v", sorting, page, err)
		}
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "Zeta/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "other/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, sorting := range []string{"name_desc", "count_desc", "folder_desc", "date_desc"} {
		page, err := l.People(ctx, 0, 1, "", false, false, sorting)
		if err != nil || slices.Contains(personIDs(page), int64(1)) {
			t.Fatalf("private data in %s: %+v %v", sorting, page, err)
		}
	}
	for _, sorting := range []string{"name; DROP TABLE photo_people", "DATE_DESC", "count", " date_asc"} {
		if _, err := l.People(ctx, 0, 1, "", false, false, sorting); err == nil {
			t.Fatalf("accepted %q", sorting)
		}
		if _, err := l.IgnoredFaces(ctx, 1, "", false, sorting); err == nil {
			t.Fatalf("ignored accepted %q", sorting)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.People(cancelled, 0, 1, "", false, false, "date_desc"); err == nil {
		t.Fatal("cancelled query succeeded")
	}
}

func TestPeopleSortPaginationIsGlobalAndStable(t *testing.T) {
	l := peopleSortFixture(t)
	ctx := context.Background()
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(10) UNION ALL SELECT x+1 FROM n WHERE x<134)
 INSERT INTO photo_people(id,name,name_fold,manual_name) SELECT x,printf('Person %03d',x),printf('person %03d',x),1 FROM n`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model)
 SELECT p.id,f.path,f.directory,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p CROSS JOIN photo_faces f WHERE p.id>=10 AND f.id=4`); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "count", "folder", "date"} {
		var ascending []int64
		for _, direction := range []string{"asc", "desc"} {
			var all []int64
			for pageNumber := 1; pageNumber <= 3; pageNumber++ {
				page, err := l.People(ctx, 0, pageNumber, "Person", true, false, key+"_"+direction)
				if err != nil || page.TotalPages != 3 || page.Page != pageNumber || len(page.People) > 60 {
					t.Fatalf("page: %+v %v", page, err)
				}
				all = append(all, personIDs(page)...)
			}
			if len(all) != 125 {
				t.Fatalf("%s: count=%d", key, len(all))
			}
			if direction == "asc" {
				ascending = all
			} else {
				slices.Reverse(ascending)
				if !reflect.DeepEqual(all, ascending) {
					t.Fatalf("unstable %s", key)
				}
			}
		}
		last, err := l.People(ctx, 0, 999, "Person", true, false, key+"_desc")
		if err != nil || last.Page != 3 || len(last.People) != 5 {
			t.Fatalf("last page: %+v %v", last, err)
		}
	}
}

func TestIgnoredFaceSortsAndPoolSettings(t *testing.T) {
	l := peopleSortFixture(t)
	ctx := context.Background()
	for _, key := range []string{"name", "count", "folder", "date"} {
		var ascending []int64
		for _, direction := range []string{"asc", "desc"} {
			page, err := l.IgnoredFaces(ctx, 1, "", false, key+"_"+direction)
			if err != nil || len(page.Faces) != 3 {
				t.Fatalf("%s: %+v %v", key, page, err)
			}
			ids := []int64{}
			for _, f := range page.Faces {
				ids = append(ids, f.ID)
			}
			if direction == "asc" {
				ascending = ids
			} else {
				slices.Reverse(ascending)
				if !reflect.DeepEqual(ids, ascending) {
					t.Fatalf("unstable %s", key)
				}
			}
			if key == "count" && direction == "desc" && page.Faces[0].PersonID != 1 {
				t.Fatalf("ignored photo count: %+v", page)
			}
		}
		page, err := l.IgnoredFaces(ctx, 1, "Aenne", true, key+"_asc")
		if err != nil || len(page.Faces) != 1 || page.Faces[0].PersonID != 2 {
			t.Fatalf("ignored search: %+v %v", page, err)
		}
	}
	// Every leased sorter restores MEMORY for unrelated pooled queries.
	var temp int
	if err := l.index.db.QueryRow(`PRAGMA temp_store`).Scan(&temp); err != nil || temp != 2 {
		t.Fatalf("temp_store=%d: %v", temp, err)
	}
}

func TestPeopleNameSortUsesExistingOrderIndex(t *testing.T) {
	l := peopleSortFixture(t)
	for _, direction := range []string{"asc", "desc"} {
		rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN `+peopleOverviewSQL("name_"+direction, ""), "%", 0)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var a, b, c int
			var detail string
			if err := rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&plan, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan.String(), "idx_photo_people_name") || !strings.Contains(plan.String(), "idx_photo_faces_person") {
			t.Fatalf("unindexed %s: %s", direction, plan.String())
		}
	}
}
