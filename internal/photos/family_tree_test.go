package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func saveTreeRoots(t *testing.T, l *Library, ids ...int64) {
	t.Helper()
	settings, err := l.FamilyTreeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SetFamilyTreeRoots(context.Background(), ids, settings.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestFamilyTreesTraverseAllRelationsAndDeduplicate(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	if enabled, err := l.FamilyTreeEnabled(ctx); err != nil || enabled {
		t.Fatalf("default: %v %v", enabled, err)
	}
	if _, err := l.FamilyTrees(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unset: %v", err)
	}
	if err := l.SetPersonParents(ctx, ids[0], ids[1], 0); err != nil {
		t.Fatal(err)
	}
	in := detailsInput(t, l, ids[1])
	in.SiblingIDs = []int64{ids[2]}
	saveDetails(t, l, ids[1], in)
	in = detailsInput(t, l, ids[2])
	in.BirthDate = "1940-01-02"
	in.DeathDate = "2020-03-04"
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[3], WeddingDate: "1960-01-01", DivorceDate: "1970-02-01"}, {SpouseID: ids[3], WeddingDate: "1980-01-01"}}
	saveDetails(t, l, ids[2], in)
	if _, err := l.SetPersonTags(ctx, ids[2], []string{"family"}); err != nil {
		t.Fatal(err)
	}
	saveTreeRoots(t, l, ids[0], ids[3], ids[4])
	page, err := l.FamilyTrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Trees) != 2 || len(page.Trees[0].People) != 4 || len(page.Trees[0].Relations) != 4 || len(page.Trees[1].People) != 1 || !reflect.DeepEqual(page.Trees[0].Roots, []int64{ids[0], ids[3]}) {
		t.Fatalf("components: %+v", page)
	}
	person := page.Trees[0].People[2]
	if person.ID != ids[2] || person.BirthDate != "1940-01-02" || person.DeathDate != "2020-03-04" || person.FaceID == 0 || !reflect.DeepEqual(person.Tags, []string{"family"}) {
		t.Fatalf("master data: %+v", person)
	}
	divorced := 0
	for _, e := range page.Trees[0].Relations {
		if e.DivorceDate != "" {
			divorced++
		}
	}
	if divorced != 1 {
		t.Fatal("divorced or repeated marriage lost")
	}
	again, err := l.FamilyTrees(ctx)
	if err != nil || !reflect.DeepEqual(page, again) {
		t.Fatal("unstable graph output")
	}
	// Traversal starting at an ancestor reaches descendants and both relationship directions.
	saveTreeRoots(t, l, ids[1])
	page, err = l.FamilyTrees(ctx)
	if err != nil || len(page.Trees) != 1 || len(page.Trees[0].People) != 4 {
		t.Fatalf("reverse traversal: %+v %v", page, err)
	}
	in = detailsInput(t, l, ids[0])
	in.SiblingIDs = []int64{ids[2]}
	saveDetails(t, l, ids[0], in)
	page, err = l.FamilyTrees(ctx)
	if err != nil || len(page.Trees[0].People) != 4 || len(page.Trees[0].Relations) != 5 {
		t.Fatalf("relation cycle: %+v %v", page, err)
	}
}

func TestFamilyTreeLiveVisibilityDoesNotBridgeHiddenPeople(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	if err := l.SetPersonParents(ctx, ids[0], ids[1], 0); err != nil {
		t.Fatal(err)
	}
	in := detailsInput(t, l, ids[1])
	in.SiblingIDs = []int64{ids[2]}
	saveDetails(t, l, ids[1], in)
	saveTreeRoots(t, l, ids[0], ids[2])
	if err := os.WriteFile(filepath.Join(l.root, "b/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	page, err := l.FamilyTrees(ctx)
	if err != nil || len(page.Trees) != 2 {
		t.Fatalf("hidden bridge joined families: %+v %v", page, err)
	}
	for _, tree := range page.Trees {
		if len(tree.People) != 1 || len(tree.Relations) != 0 {
			t.Fatalf("hidden metadata leaked: %+v", tree)
		}
	}
	if err := os.WriteFile(filepath.Join(l.root, "a/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := l.FamilyTreeSettings(ctx)
	if err != nil || settings.Roots[0].Name != "" || settings.Roots[0].Available {
		t.Fatalf("settings leaked root: %+v %v", settings, err)
	}
	if err := l.SetFamilyTreeRoots(ctx, []int64{ids[0], ids[2]}, settings.Revision); err != nil {
		t.Fatal("cannot retain unavailable root", err)
	}
	page, err = l.FamilyTrees(ctx)
	if err != nil || len(page.Trees) != 1 || page.Trees[0].People[0].ID != ids[2] {
		t.Fatalf("hidden root: %+v %v", page, err)
	}
}

func TestFamilyTreeSettingsAtomicRevisionAndMergePersistence(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	saveTreeRoots(t, l, ids[0], ids[4])
	before, err := l.FamilyTreeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]int64{{ids[0], ids[0]}, {-1}, {999999}, make([]int64, MaxFamilyTreeRoots+1)} {
		if err := l.SetFamilyTreeRoots(ctx, selection, before.Revision); !errors.Is(err, ErrFamilyTreeSettings) {
			t.Fatalf("invalid: %v", err)
		}
	}
	after, _ := l.FamilyTreeSettings(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed save changed settings")
	}
	if err := l.SetFamilyTreeRoots(ctx, nil, before.Revision+1); !errors.Is(err, ErrFamilyTreeConflict) {
		t.Fatalf("revision: %v", err)
	}
	// A failed family merge must roll back the already attempted root transfer.
	in := detailsInput(t, l, ids[0])
	in.BirthDate = "1940-01-01"
	saveDetails(t, l, ids[0], in)
	in = detailsInput(t, l, ids[4])
	in.BirthDate = "1941-01-01"
	saveDetails(t, l, ids[4], in)
	if err := l.MergePeople(ctx, ids[0], ids[4]); err == nil {
		t.Fatal("expected conflicting merge")
	}
	after, _ = l.FamilyTreeSettings(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed merge changed roots")
	}
	in = detailsInput(t, l, ids[4])
	in.BirthDate = ""
	saveDetails(t, l, ids[4], in)
	if err := l.MergePeople(ctx, ids[0], ids[4]); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(l.root, l.cacheDir, l.dbPath, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	settings, err := reopened.FamilyTreeSettings(ctx)
	if err != nil || len(settings.Roots) != 1 || settings.Roots[0].ID != ids[4] {
		t.Fatalf("persisted merge: %+v %v", settings, err)
	}
	if err := reopened.SetFamilyTreeRoots(ctx, nil, settings.Revision); err != nil {
		t.Fatal(err)
	}
	if enabled, err := reopened.FamilyTreeEnabled(ctx); err != nil || enabled {
		t.Fatal("clearing roots did not disable trees")
	}
}

func TestFamilyTreeMigrationAndLargeConnectedFamily(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	_, err := l.index.db.Exec(`DROP TABLE photo_family_tree_roots; DROP TABLE photo_family_tree_settings; DROP TRIGGER family_tree_person_delete; UPDATE schema_migrations SET version=38 WHERE component='photos'`)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(l.root, l.cacheDir, l.dbPath, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	l = reopened
	var version int
	if err := l.index.db.QueryRow(`SELECT version FROM schema_migrations WHERE component='photos'`).Scan(&version); err != nil || version != photoSchemaVersion {
		t.Fatalf("migration %d %v", version, err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE photo_people SET name='Person 1',name_fold='person 1',manual_name=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 1200; i++ {
		if _, err = tx.Exec(`INSERT INTO photo_people(id,name,name_fold,manual_name) VALUES(?,?,?,1)`, i, fmt.Sprintf("Person %d", i), fmt.Sprintf("person %d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=1`, i); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO person_siblings VALUES(?,?)`, i-1, i); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	saveTreeRoots(t, l, 1, 600, 1200)
	page, err := l.FamilyTrees(ctx)
	if err != nil || len(page.Trees) != 1 || len(page.Trees[0].People) != 1200 || len(page.Trees[0].Relations) != 1199 {
		t.Fatalf("long cyclic-capable chain: trees=%d err=%v", len(page.Trees), err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.FamilyTrees(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestFamilyTreeLimitReturnsNoPartialTrees(t *testing.T) {
	l, ids := parentFixture(t)
	saveTreeRoots(t, l, ids[0], ids[4])
	// An excessive frontier is rejected before loading thousands of portraits or
	// returning an apparently complete, but truncated, family.
	_, err := l.index.db.Exec(`WITH RECURSIVE people(id) AS (
 SELECT 1000 UNION ALL SELECT id+1 FROM people WHERE id < ?
) INSERT INTO photo_people(id,name,name_fold,manual_name) SELECT id,'Person '||id,'person '||id,1 FROM people`, 1000+MaxFamilyTreePeople)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO person_siblings(person_a,person_b) SELECT ?,id FROM photo_people WHERE id>=1000`, ids[0]); err != nil {
		t.Fatal(err)
	}
	page, err := l.FamilyTrees(context.Background())
	if !errors.Is(err, ErrFamilyTreeLarge) || len(page.Trees) != 0 {
		t.Fatalf("partial result on limit: trees=%d error=%v", len(page.Trees), err)
	}
}
