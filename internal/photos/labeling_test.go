package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func labelFixture(t *testing.T) (*Library, LabelPerson, LabelSession) {
	t.Helper()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	for range 3 {
		finishFace(t, l, 0)
	}
	session, err := l.LabelSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	list, err := l.LabelCandidates(context.Background(), 0, session.UpperID)
	if err != nil || len(list.People) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	p, err := l.LabelPerson(context.Background(), list.People[0].ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return l, p, session
}
func labelAction(p LabelPerson, s LabelSession, action string) LabelAction {
	return LabelAction{OperationID: "test-operation-" + action, Dataset: s.Dataset, Revision: p.Revision, Action: action}
}
func TestLabelingDetachNameAssignAndReceipts(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	a := labelAction(p, s, "detach")
	a.FaceID = p.Faces[0].ID
	detached, err := l.ApplyLabelAction(ctx, "manager", p.ID, a)
	if err != nil || detached.Faces != 1 || detached.Groups != 0 || detached.NewID == 0 {
		t.Fatalf("%+v %v", detached, err)
	}
	repeated, err := l.ApplyLabelAction(ctx, "manager", p.ID, a)
	if err != nil || repeated != detached {
		t.Fatalf("repeat %+v %v", repeated, err)
	}
	a.FaceID = p.Faces[1].ID
	if _, err = l.ApplyLabelAction(ctx, "manager", p.ID, a); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("payload collision %v", err)
	}
	if _, err = l.LabelReceipt(ctx, "other", detached.OperationID, s.Dataset); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("receipt leaked %v", err)
	}
	target, err := l.LabelPerson(ctx, detached.NewID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a = labelAction(target, s, "name")
	a.Name = "Erika"
	named, err := l.ApplyLabelAction(ctx, "manager", target.ID, a)
	if err != nil || named.Faces != 1 {
		t.Fatalf("%+v %v", named, err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	target, _ = l.LabelPerson(ctx, target.ID, 0)
	a = labelAction(p, s, "name")
	a.OperationID += "-duplicate"
	a.Name = "Erika"
	if _, err = l.ApplyLabelAction(ctx, "manager", p.ID, a); !errors.Is(err, ErrLabelNameExists) {
		t.Fatalf("duplicate %v", err)
	}
	a = labelAction(p, s, "assign")
	a.TargetID = target.ID
	a.TargetRevision = target.Revision
	assigned, err := l.ApplyLabelAction(ctx, "manager", p.ID, a)
	if err != nil || assigned.Faces != 2 || assigned.TargetID != target.ID {
		t.Fatalf("%+v %v", assigned, err)
	}
	target, _ = l.LabelPerson(ctx, target.ID, 0)
	if target.Count != 3 {
		t.Fatal(target)
	}
	if _, err = l.ApplyLabelAction(ctx, "manager", p.ID, a); err != nil {
		t.Fatal(err)
	}
	found, err := l.LabelSuggestions(ctx, "rik", false)
	if err != nil || len(found) != 1 {
		t.Fatalf("%+v %v", found, err)
	}
}
func TestLabelingRevisionsRollbackAndConcurrentAction(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	a := labelAction(p, s, "ignore")
	if err := l.RenamePerson(ctx, p.ID, "Web"); err != nil {
		t.Fatal(err)
	}
	if err := l.RenamePerson(ctx, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyLabelAction(ctx, "manager", p.ID, a); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("web revision %v", err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	a.Revision = p.Revision
	// Force failure when recording the receipt: the face mutation must roll back too.
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_label_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyLabelAction(ctx, "manager", p.ID, a); err == nil {
		t.Fatal("expected failure")
	}
	after, _ := l.LabelPerson(ctx, p.ID, 0)
	if after.Count != 3 || after.Revision != p.Revision {
		t.Fatalf("partial commit %+v", after)
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_label_receipt`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan LabelReceipt, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { r, e := l.ApplyLabelAction(ctx, "manager", p.ID, a); results <- r; errs <- e })
	}
	wg.Wait()
	if e1, e2 := <-errs, <-errs; e1 != nil || e2 != nil {
		t.Fatalf("%v %v", e1, e2)
	}
	if r1, r2 := <-results, <-results; r1 != r2 || r1.Faces != 3 {
		t.Fatalf("%+v %+v", r1, r2)
	}
	if _, err := l.LabelPerson(ctx, p.ID, 0); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}
func TestLabelingFullLargeGroupAndCursorBound(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	// Duplicate an indexed detection, avoiding hundreds of image decodes in the fixture.
	tx, err := l.index.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for range 601 {
		if _, err = tx.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, p.Faces[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	p, err = l.LabelPerson(ctx, p.ID, 600)
	if err != nil || len(p.Faces) != 4 || p.Count != 604 {
		t.Fatalf("%+v %v", p, err)
	}
	a := labelAction(p, s, "ignore")
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil || r.Faces != 604 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = l.index.db.Exec(`INSERT INTO photo_people(name) VALUES('')`); err != nil {
		t.Fatal(err)
	}
	out, err := l.LabelCandidates(ctx, 0, s.UpperID)
	if err != nil || len(out.People) != 0 {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestLabelingProtectedPhotosAndDatasetReset(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyLabelAction(ctx, "m", p.ID, labelAction(p, s, "ignore")); err == nil {
		t.Fatal("protected mutation succeeded")
	}
	out, err := l.LabelCandidates(ctx, 0, s.UpperID)
	if err != nil || len(out.People) != 0 {
		t.Fatalf("protected groups %+v %v", out, err)
	}
	if err = l.ClearFaces(ctx); err != nil {
		t.Fatal(err)
	}
	reset, _ := l.LabelSession(ctx)
	if reset.Instance != s.Instance || reset.Dataset == s.Dataset {
		t.Fatalf("identity %+v", reset)
	}
	if _, err = l.ApplyLabelAction(ctx, "m", p.ID, labelAction(p, s, "ignore")); !errors.Is(err, ErrLabelConflict) {
		t.Fatal(err)
	}
}
func TestLabelingCandidatePagesAndTargetConflict(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	// The high-water mark excludes detached groups, which clients explicitly prioritize.
	for i := 0; i < 25; i++ {
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name) VALUES('')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		_, err = l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, id, p.Faces[0].ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := l.LabelCandidates(ctx, 0, s.UpperID)
	if err != nil || len(page.People) != 1 {
		t.Fatalf("boundary %+v %v", page, err)
	}
	s, _ = l.LabelSession(ctx)
	page, err = l.LabelCandidates(ctx, 0, s.UpperID)
	if err != nil || len(page.People) != 20 || !page.HasNext {
		t.Fatalf("page %+v %v", page, err)
	}
	next, err := l.LabelCandidates(ctx, page.Next, s.UpperID)
	if err != nil || len(next.People) != 6 || next.HasNext {
		t.Fatalf("next %+v %v", next, err)
	}
	target := next.People[0]
	if err = l.RenamePerson(ctx, target.ID, "Ziel"); err != nil {
		t.Fatal(err)
	}
	a := labelAction(p, s, "assign")
	a.TargetID = target.ID
	a.TargetRevision = target.Revision
	if _, err = l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
		t.Fatal(fmt.Errorf("stale target: %w", err))
	}
}

func TestLabelingUpgradeBackfillsExistingGroupsAndPreservesIdentity(t *testing.T) {
	ctx := context.Background()
	l, p, _ := labelFixture(t)
	if err := l.RenamePerson(ctx, p.ID, "Bestand"); err != nil {
		t.Fatal(err)
	}
	// Recreate a version-18 photo database with real indexed faces and names.
	for _, name := range []string{"labeling_person_insert", "labeling_person_name", "labeling_person_delete", "labeling_face_insert", "labeling_face_delete", "labeling_face_update"} {
		if _, err := l.index.db.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"photo_labeling_identity", "photo_labeling_actions", "photo_person_revisions"} {
		if _, err := l.index.db.Exec(`DROP TABLE ` + name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.index.db.Exec(`UPDATE schema_migrations SET version=18 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	if err := runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
		t.Fatal(err)
	}
	session, err := l.LabelSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil || existing.Name != "Bestand" || existing.Count != 3 || existing.Revision != 1 {
		t.Fatalf("backfill %+v %v", existing, err)
	}
	if err = l.RenamePerson(ctx, p.ID, "Bestand neu"); err != nil {
		t.Fatal(err)
	}
	// A migration retried after interruption must not reset revisions or identity.
	if err = runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
		t.Fatal(err)
	}
	again, _ := l.LabelSession(ctx)
	changed, _ := l.LabelPerson(ctx, p.ID, 0)
	if again != session || changed.Revision <= existing.Revision {
		t.Fatalf("migration not stable %+v %+v", again, changed)
	}
}

func TestLabelingCompetingDecisionsCommitOnlyOnce(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, kind := range []string{"name", "ignore"} {
		wg.Go(func() {
			a := labelAction(p, s, kind)
			a.Name = "Anna"
			_, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
			results <- err
		})
	}
	wg.Wait()
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, ErrLabelConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
