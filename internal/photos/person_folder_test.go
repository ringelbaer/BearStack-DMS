package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func folderAction(t *testing.T, l *Library, id int64, directory, action string, target int64) error {
	t.Helper()
	p, err := l.PersonFolders(context.Background(), id, 1)
	if err != nil {
		t.Fatal(err)
	}
	return l.ApplyPersonFolderAction(context.Background(), id, PersonFolderAction{Directory: directory, Action: action, TargetID: target, Revision: p.Revision})
}

func TestPersonFoldersFormattingPreviewsAndActions(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/Nested_Folder/b.jpg")
	for range 3 {
		finishFace(t, l, 0)
	}
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].PersonID
	if err := l.RenamePerson(ctx, id, "Ada"); err != nil {
		t.Fatal(err)
	}
	// More than a usual 500-face selection; only eight previews may be returned.
	for range 510 {
		if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, faces[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	p, err := l.PersonFolders(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Folders) != 3 || p.Folders[0].Count != 511 || len(p.Folders[0].FaceIDs) != 8 {
		t.Fatalf("%+v", p)
	}
	want := []string{"Fotos", "Fotos / 02.01.2024 · Family Trip", "Fotos / 02.01.2024 · Family Trip / Nested Folder"}
	for i, f := range p.Folders {
		if f.DisplayPath != want[i] {
			t.Fatalf("path %q want %q", f.DisplayPath, want[i])
		}
	}
	if err := folderAction(t, l, id, "", "unnamed", 0); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE person_id=?`, id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d %v", count, err)
	}
	if err := l.ApplyPersonFolderAction(ctx, id, PersonFolderAction{Directory: "20240102_Family_Trip", Action: "ignore", Revision: p.Revision}); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("stale: %v", err)
	}
	if err := folderAction(t, l, id, "20240102_Family_Trip", "ignore", 0); err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonFolders(ctx, id, 1)
	if err != nil || len(p.Folders) != 1 || p.Folders[0].Directory != "20240102_Family_Trip/Nested_Folder" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestPersonFolderExclusionBlocksAssignmentsAndSurvivesMerge(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a/a.jpg", "b/b.jpg", "c/c.jpg")
	for range 2 {
		finishFace(t, l, 0)
	}
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "a/a.jpg")
	c, _ := l.AutomaticFaces(ctx, "c/c.jpg")
	id, target := a[0].PersonID, c[0].PersonID
	if err := l.RenamePerson(ctx, id, "Ada"); err != nil {
		t.Fatal(err)
	}
	if err := l.RenamePerson(ctx, target, "Other"); err != nil {
		t.Fatal(err)
	}
	if err := folderAction(t, l, id, "a", "exclude", 0); err != nil {
		t.Fatal(err)
	}
	p, err := l.PersonFolders(ctx, id, 1)
	if err != nil || len(p.Folders) != 2 || !p.Folders[0].Excluded || len(p.Folders[0].FaceIDs) != 0 {
		t.Fatalf("%+v %v", p, err)
	}
	a, _ = l.AutomaticFaces(ctx, "a/a.jpg")
	unnamed := a[0].PersonID
	if unnamed == id || a[0].Name != "" {
		t.Fatalf("not unassigned: %+v", a)
	}
	if err := folderAction(t, l, unnamed, "a", "move", id); !IsPersonFolderExcluded(err) {
		t.Fatalf("expected exclusion error: %v", err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := faceReconcileExclusions(ctx, tx, "a/new.jpg", unnamed)
	if err != nil || !excluded[id] {
		t.Fatalf("reconciliation exclusions: %v %v", excluded, err)
	}
	if err = mergePersonFolderExclusionsTx(ctx, tx, id, unnamed); !errors.Is(err, ErrPersonFolderExcluded) {
		t.Fatalf("conflicting merge: %v", err)
	}
	tx.Rollback()
	// Reanalysis must also avoid the named target, even with a matching vector.
	if _, err = l.index.db.Exec(`UPDATE photo_faces SET manual=0 WHERE person_id=?`, unnamed); err != nil {
		t.Fatal(err)
	}
	if err = l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	media, err := l.MediaContext(ctx, "a/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	job := FaceJob{Path: media.Path, Size: media.SizeBytes, ModTime: media.ModTime.UnixNano(), XMP: media.XMPFingerprint, Model: facerec.Model}
	if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
		t.Fatal(err)
	}
	a, _ = l.AutomaticFaces(ctx, "a/a.jpg")
	if a[0].PersonID == id {
		t.Fatal("recognition bypassed exclusion")
	}
	if err := l.MergePeople(ctx, id, target); err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonFolders(ctx, target, 1)
	if err != nil || !p.Folders[0].Excluded {
		t.Fatalf("merge lost exclusion: %+v %v", p, err)
	}
	if err := folderAction(t, l, target, "a", "include", 0); err != nil {
		t.Fatal(err)
	}
	if err := folderAction(t, l, a[0].PersonID, "a", "move", target); err != nil {
		t.Fatal(err)
	}
}

func TestPersonFolderRollbackChangedSourceAndPagination(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].PersonID
	p, err := l.PersonFolders(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(l.root, "b.jpg"))
	info, _ := os.Stat(filepath.Join(l.root, "b.jpg"))
	if err = os.WriteFile(filepath.Join(l.root, "b.jpg"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	err = l.ApplyPersonFolderAction(ctx, id, PersonFolderAction{Action: "exclude", Revision: p.Revision})
	if !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("changed source: %v", err)
	}
	var count int
	l.index.db.QueryRow(`SELECT count(*) FROM person_folder_exclusions`).Scan(&count)
	if count != 0 {
		t.Fatal("partial exclusion")
	}
	if err = os.WriteFile(filepath.Join(l.root, "b.jpg"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(filepath.Join(l.root, "b.jpg"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Inject a write failure after creating a target; no partial group may survive.
	_, err = l.index.db.Exec(`CREATE TRIGGER fail_folder BEFORE UPDATE OF person_id ON photo_faces BEGIN SELECT RAISE(ABORT,'injected'); END`)
	if err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonFolders(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = l.ApplyPersonFolderAction(ctx, id, PersonFolderAction{Action: "move", Name: "Rollback", Revision: p.Revision})
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("expected injected failure: %v", err)
	}
	l.index.db.QueryRow(`SELECT count(*) FROM photo_people WHERE name='Rollback'`).Scan(&count)
	if count != 0 {
		t.Fatal("partial target")
	}
	for i := range 81 {
		if err := os.Mkdir(filepath.Join(l.root, fmt.Sprintf("folder%03d", i)), 0750); err != nil {
			t.Fatal(err)
		}
		if _, err := l.index.db.Exec(`INSERT INTO person_folder_exclusions VALUES(?,?)`, id, fmt.Sprintf("folder%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	p, err = l.PersonFolders(ctx, id, 1)
	if err != nil || len(p.Folders) != 40 || !p.HasNext {
		t.Fatalf("first page: %+v %v", p, err)
	}
	p, err = l.PersonFolders(ctx, id, 3)
	if err != nil || p.HasNext || len(p.Folders) != 2 {
		t.Fatalf("last page: %+v %v", p, err)
	}
}

func TestPersonSummaryOnlyRecordedCurrentRelations(t *testing.T) {
	d := PersonDetails{BirthDate: "2000-01-02", Parents: PersonParents{Mother: &PersonParent{Name: "Mother"}}, Siblings: []PersonParent{{Name: "Sibling"}}, Marriages: []PersonMarriage{{Spouse: PersonParent{Name: "Ex"}, DivorceDate: "2020-01-01"}, {Spouse: PersonParent{Name: "Current"}, WeddingDate: "2021-02-03"}}}
	got := d.Summary()
	for _, want := range []string{"02.01.2000", "Mutter: Mother", "Geschwister: Sibling", "Current seit 03.02.2021"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lacks %q", got, want)
		}
	}
	for _, bad := range []string{"Ex", "Vater", "Gestorben"} {
		if strings.Contains(got, bad) {
			t.Fatalf("unexpected %q in %q", bad, got)
		}
	}
	if (PersonDetails{}).Summary() != "" {
		t.Fatal("empty summary contains placeholders")
	}
}

func TestPersonFolderSchemaMigration(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	_, err := l.index.db.Exec(`DROP TRIGGER person_folder_insert_guard; DROP TRIGGER person_folder_update_guard; DROP TRIGGER person_folder_delete; DROP TABLE person_folder_exclusions; DROP INDEX idx_faces_person_folder; DROP INDEX idx_faces_folder_person; UPDATE schema_migrations SET version=37 WHERE component='photos'`)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(l.root, l.cacheDir, l.dbPath, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var version int
	if err := reopened.index.db.QueryRow(`SELECT version FROM schema_migrations WHERE component='photos'`).Scan(&version); err != nil || version != photoSchemaVersion {
		t.Fatalf("migration: %d %v", version, err)
	}
	faces, err := reopened.AutomaticFaces(context.Background(), "a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("migration lost faces: %v %v", faces, err)
	}
	if err := folderAction(t, reopened, faces[0].PersonID, "", "exclude", 0); err != nil {
		t.Fatal(err)
	}
}

func TestPersonFolderExclusionsFilterAutomaticGroupCandidates(t *testing.T) {
	for _, wholeGroup := range []bool{false, true} {
		t.Run(fmt.Sprint(wholeGroup), func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"source/a.jpg", "blocked/b.jpg", "allowed/c.jpg"}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, [][]float32{faceDetection(0).Embedding, matchingVector(.95), matchingVector(.75)}, []int{2, 1, 1})
			for _, id := range []int64{2, 3} {
				if err := l.RenamePerson(ctx, id, fmt.Sprintf("Named %d", id)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := l.index.db.Exec(`INSERT INTO person_folder_exclusions VALUES(2,'source')`); err != nil {
				t.Fatal(err)
			}
			v := DefaultFaceThresholds()
			v.ReconcileUnnamedGroups = wholeGroup
			if err := l.SetFaceThresholds(ctx, v); err != nil {
				t.Fatal(err)
			}
			if _, err := l.ReconcileFacesBatch(ctx, 1); err != nil {
				t.Fatal(err)
			}
			face, err := l.Face(ctx, 1)
			if err != nil || face.PersonID != 3 {
				t.Fatalf("blocked leader prevented allowed match: %+v %v", face, err)
			}
		})
	}
}

func TestPersonFolderPrivacyAndPersistentGuards(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a/a.jpg", "b/b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a/a.jpg")
	id := faces[0].PersonID
	if err := folderAction(t, l, id, "a", "exclude", 0); err != nil {
		t.Fatal(err)
	}
	// Reopen the library and verify that the exclusion and guards persist.
	reopened, err := New(l.root, l.cacheDir, l.dbPath, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	p, err := reopened.PersonFolders(ctx, id, 1)
	if err != nil || !p.Folders[0].Excluded {
		t.Fatalf("reopen: %+v %v", p, err)
	}
	if _, err := reopened.index.db.Exec(`UPDATE photo_faces SET person_id=? WHERE path='a/a.jpg'`, id); err == nil {
		t.Fatal("guard missing after restart")
	}
	if _, err := reopened.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE path='a/a.jpg'`, id); err == nil {
		t.Fatal("insert guard missing")
	}
	// No names or paths leak when both the active and excluded folders become private.
	for _, dir := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(l.root, dir, ".adminonly"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reopened.PersonFolders(ctx, id, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private person exposed: %v", err)
	}
}

func TestPersonFolderConcurrentActionsAreAtomic(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].PersonID
	p, err := l.PersonFolders(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, action := range []string{"exclude", "unnamed"} {
		go func() {
			results <- l.ApplyPersonFolderAction(ctx, id, PersonFolderAction{Action: action, Revision: p.Revision})
		}()
	}
	succeeded, conflicted := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrLabelConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("successes %d conflicts %d", succeeded, conflicted)
	}
	var people int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_people`).Scan(&people); err != nil || people != 2 {
		t.Fatalf("partial target created: %d %v", people, err)
	}
}
