package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func detailsInput(t *testing.T, l *Library, id int64) PersonDetailsInput {
	t.Helper()
	d, err := l.PersonDetails(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	in := PersonDetailsInput{Revision: d.Revision, BirthDate: d.BirthDate, DeathDate: d.DeathDate, SiblingIDs: []int64{}, Marriages: []PersonMarriageInput{}}
	for _, p := range d.Siblings {
		in.SiblingIDs = append(in.SiblingIDs, p.ID)
	}
	for _, m := range d.Marriages {
		in.Marriages = append(in.Marriages, PersonMarriageInput{ID: m.ID, SpouseID: m.Spouse.ID, WeddingDate: m.WeddingDate, DivorceDate: m.DivorceDate})
	}
	return in
}
func saveDetails(t *testing.T, l *Library, id int64, in PersonDetailsInput) {
	t.Helper()
	if err := l.SetPersonDetails(context.Background(), id, in); err != nil {
		t.Fatal(err)
	}
}
func TestPersonDetailsDatesSymmetryAndRepeatedMarriage(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	in := detailsInput(t, l, ids[0])
	in.BirthDate = "1900-02-28"
	in.DeathDate = "1990-01-01"
	in.SiblingIDs = ids[1:3]
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[3], WeddingDate: "1920-01-01", DivorceDate: "1930-01-01"}, {SpouseID: ids[3], WeddingDate: "1940-01-01"}, {SpouseID: ids[4]}}
	saveDetails(t, l, ids[0], in)
	for _, other := range ids[1:3] {
		d, err := l.PersonDetails(ctx, other)
		if err != nil || len(d.Siblings) != 1 || d.Siblings[0].ID != ids[0] {
			t.Fatalf("sibling: %+v %v", d, err)
		}
	}
	partner := detailsInput(t, l, ids[3])
	if len(partner.Marriages) != 2 || partner.Marriages[0].SpouseID != ids[0] {
		t.Fatalf("partner: %+v", partner)
	}
	partner.Marriages[1].DivorceDate = "1950-01-01"
	saveDetails(t, l, ids[3], partner)
	got := detailsInput(t, l, ids[0])
	if got.Marriages[1].DivorceDate != "1950-01-01" || got.BirthDate != in.BirthDate || got.DeathDate != in.DeathDate {
		t.Fatalf("reverse update: %+v", got)
	}
	if err := l.RenamePerson(ctx, ids[3], "Renamed"); err != nil {
		t.Fatal(err)
	}
	d, err := l.PersonDetails(ctx, ids[0])
	if err != nil || d.Marriages[0].Spouse.Name != "Renamed" {
		t.Fatalf("name: %+v %v", d, err)
	}
	got.Marriages = nil
	got.SiblingIDs = nil
	got.BirthDate = ""
	got.DeathDate = ""
	saveDetails(t, l, ids[0], got)
	for _, other := range ids {
		d, err := l.PersonDetails(ctx, other)
		if err != nil || len(d.Siblings)+len(d.Marriages) != 0 {
			t.Fatalf("clear: %+v %v", d, err)
		}
	}
}

func TestPersonDetailsValidationAndOptimisticUpdates(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	base := detailsInput(t, l, ids[0])
	invalid := []PersonDetailsInput{
		{BirthDate: "2023-02-29"}, {BirthDate: "0000-01-01"}, {BirthDate: "2000-1-01"}, {BirthDate: "2000-01-02", DeathDate: "2000-01-01"},
		{SiblingIDs: []int64{ids[0]}}, {SiblingIDs: []int64{ids[1], ids[1]}}, {SiblingIDs: []int64{999999}}, {SiblingIDs: make([]int64, 101)},
		{Marriages: []PersonMarriageInput{{SpouseID: ids[0]}}}, {Marriages: []PersonMarriageInput{{SpouseID: ids[1], WeddingDate: "2000-01-02", DivorceDate: "2000-01-01"}}},
		{Marriages: []PersonMarriageInput{{SpouseID: ids[1]}, {SpouseID: ids[1]}}}, {Marriages: []PersonMarriageInput{{ID: 999999, SpouseID: ids[1]}}},
	}
	for _, in := range invalid {
		in.Revision = base.Revision
		if err := l.SetPersonDetails(ctx, ids[0], in); !errors.Is(err, ErrPersonDetails) {
			t.Fatalf("accepted %+v: %v", in, err)
		}
	}
	if got := detailsInput(t, l, ids[0]); !reflect.DeepEqual(got, base) {
		t.Fatalf("invalid writes changed data: %+v", got)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, date := range []string{"2000-02-29", "2001-01-01"} {
		wg.Add(1)
		go func(date string) {
			defer wg.Done()
			in := base
			in.BirthDate = date
			results <- l.SetPersonDetails(ctx, ids[0], in)
		}(date)
	}
	wg.Wait()
	close(results)
	saved, conflicts := 0, 0
	for err := range results {
		if err == nil {
			saved++
		} else if errors.Is(err, ErrPersonDetailsConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("concurrent: %d saved %d conflicts", saved, conflicts)
	}
	// A change from the other side invalidates an already open form too.
	stale := detailsInput(t, l, ids[1])
	in := detailsInput(t, l, ids[0])
	in.SiblingIDs = []int64{ids[1]}
	saveDetails(t, l, ids[0], in)
	if err := l.SetPersonDetails(ctx, ids[1], stale); !errors.Is(err, ErrPersonDetailsConflict) {
		t.Fatalf("stale reverse: %v", err)
	}
}

func TestPersonDetailsHiddenRelationshipsSurviveEditing(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	in := detailsInput(t, l, ids[0])
	in.SiblingIDs = []int64{ids[1]}
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[2], WeddingDate: "2000-01-01"}}
	saveDetails(t, l, ids[0], in)
	for _, dir := range []string{"b", "c"} {
		if err := os.WriteFile(filepath.Join(l.root, dir, ".adminonly"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	hidden := detailsInput(t, l, ids[0])
	if len(hidden.SiblingIDs)+len(hidden.Marriages) != 0 {
		t.Fatalf("private relations: %+v", hidden)
	}
	hidden.BirthDate = "1970-01-01"
	saveDetails(t, l, ids[0], hidden)
	raw, err := readPersonDetails(ctx, l.index.db, ids[0])
	if err != nil || len(raw.SiblingIDs) != 1 || len(raw.Marriages) != 1 {
		t.Fatalf("hidden data removed: %+v %v", raw, err)
	}
	hidden = detailsInput(t, l, ids[0])
	hidden.SiblingIDs = []int64{ids[1]}
	if err := l.SetPersonDetails(ctx, ids[0], hidden); !errors.Is(err, ErrPersonDetails) {
		t.Fatalf("private target: %v", err)
	}
	if _, err := l.PersonDetails(ctx, ids[1]); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private source: %v", err)
	}
	// Unnamed relatives are hidden as well, without exposing their dates.
	in = detailsInput(t, l, ids[3])
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[4], WeddingDate: "2010-01-01"}}
	saveDetails(t, l, ids[3], in)
	if err := l.RenamePerson(ctx, ids[4], ""); err != nil {
		t.Fatal(err)
	}
	d, err := l.PersonDetails(ctx, ids[3])
	if err != nil || len(d.Marriages) != 0 {
		t.Fatalf("unnamed: %+v %v", d, err)
	}
}

func TestPersonDetailsMergePreservesAndRejectsConflicts(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	in := detailsInput(t, l, ids[0])
	in.BirthDate = "1900-01-01"
	in.SiblingIDs = []int64{ids[2]}
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[3], WeddingDate: "1920-01-01"}}
	saveDetails(t, l, ids[0], in)
	in = detailsInput(t, l, ids[1])
	in.DeathDate = "1990-01-01"
	in.SiblingIDs = []int64{ids[2]}
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[3], WeddingDate: "1920-01-01"}}
	saveDetails(t, l, ids[1], in)
	if err := l.MergePeople(ctx, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	got := detailsInput(t, l, ids[1])
	if got.BirthDate != "1900-01-01" || got.DeathDate != "1990-01-01" || len(got.SiblingIDs) != 1 || len(got.Marriages) != 1 {
		t.Fatalf("merged: %+v", got)
	}
	reverse := detailsInput(t, l, ids[3])
	if len(reverse.Marriages) != 1 || reverse.Marriages[0].SpouseID != ids[1] {
		t.Fatalf("rewire: %+v", reverse)
	}
	in = detailsInput(t, l, ids[4])
	in.BirthDate = "1901-01-01"
	saveDetails(t, l, ids[4], in)
	for _, source := range []int64{ids[2], ids[3], ids[4]} {
		if err := l.MergePeople(ctx, source, ids[1]); !errors.Is(err, ErrPersonDetailsMerge) {
			t.Fatalf("conflicting merge %d: %v", source, err)
		}
		if got2 := detailsInput(t, l, ids[1]); !reflect.DeepEqual(got2, got) {
			t.Fatalf("partial merge: %+v", got2)
		}
	}
	// Cleanup follows person deletion and cannot leave dangling links.
	if _, err := l.index.db.Exec(`DELETE FROM photo_people WHERE id=?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	reverse = detailsInput(t, l, ids[3])
	if len(reverse.Marriages) != 0 {
		t.Fatalf("dangling: %+v", reverse)
	}
}

func TestPersonDetailsMigrationAndTransactionRollback(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	if _, err := l.index.db.Exec(`DROP TRIGGER photo_person_details_delete; DROP TABLE person_details; DROP TABLE person_siblings; DROP TABLE person_marriages; UPDATE schema_migrations SET version=34 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	if err := runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
		t.Fatal(err)
	}
	before := detailsInput(t, l, ids[0])
	in := before
	in.BirthDate = "2000-01-01"
	in.SiblingIDs = []int64{ids[1]}
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[2]}}
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_details BEFORE INSERT ON person_details BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPersonDetails(ctx, ids[0], in); err == nil || !strings.Contains(err.Error(), "fixture") {
		t.Fatalf("failure: %v", err)
	}
	if after := detailsInput(t, l, ids[0]); !reflect.DeepEqual(after, before) {
		t.Fatalf("rollback: %+v", after)
	}
	if reverse := detailsInput(t, l, ids[1]); len(reverse.SiblingIDs) != 0 {
		t.Fatalf("reverse rollback: %+v", reverse)
	}
}

func TestPersonDetailsAutomaticMergeConflictLeavesTransactionUsable(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	for i, date := range []string{"1900-01-01", "1901-01-01"} {
		in := detailsInput(t, l, ids[i])
		in.BirthDate = date
		saveDetails(t, l, ids[i], in)
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	moved, err := mergeAutomaticFaceGroupTx(ctx, tx, ids[0], ids[1])
	if err != nil || moved != 0 {
		t.Fatalf("automatic conflict: %d %v", moved, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_people SET name='Still usable' WHERE id=?`, ids[4]); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if d := detailsInput(t, l, ids[0]); d.BirthDate != "1900-01-01" {
		t.Fatalf("source changed: %+v", d)
	}
	if d := detailsInput(t, l, ids[1]); d.BirthDate != "1901-01-01" {
		t.Fatalf("target changed: %+v", d)
	}
}

func TestPersonDetailsRelationshipLimitOnBothSides(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	others := []int64{}
	for i := 0; i < 101; i++ {
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES('Relative','relative',1)`)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		others = append(others, id)
		if _, err = l.index.db.Exec(`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model)
 SELECT ?,path,directory,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE person_id=? LIMIT 1`, id, ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	in := detailsInput(t, l, ids[0])
	in.SiblingIDs = others[:100]
	saveDetails(t, l, ids[0], in)
	before := detailsInput(t, l, others[100])
	change := before
	change.SiblingIDs = []int64{ids[0]}
	change.BirthDate = "2000-01-01"
	if err := l.SetPersonDetails(ctx, others[100], change); err == nil {
		t.Fatal("reverse limit exceeded")
	}
	if got := detailsInput(t, l, others[100]); !reflect.DeepEqual(got, before) {
		t.Fatalf("limit partially saved: %+v", got)
	}
	in = detailsInput(t, l, ids[0])
	in.SiblingIDs = nil
	for i := 0; i < 100; i++ {
		in.Marriages = append(in.Marriages, PersonMarriageInput{SpouseID: ids[1], WeddingDate: fmt.Sprintf("%04d-01-01", 1800+i)})
	}
	saveDetails(t, l, ids[0], in)
	in = detailsInput(t, l, ids[2])
	in.Marriages = []PersonMarriageInput{{SpouseID: ids[1], WeddingDate: "2000-01-01"}}
	if err := l.SetPersonDetails(ctx, ids[2], in); err == nil {
		t.Fatal("partner marriage limit exceeded")
	}
	if got := detailsInput(t, l, ids[2]); len(got.Marriages) != 0 {
		t.Fatalf("marriage limit partially saved: %+v", got)
	}
}
