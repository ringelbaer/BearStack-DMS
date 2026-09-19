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

func TestLabelFolderActionsAtomicScopeAndReplay(t *testing.T) {
	for _, action := range []string{"move", "new_name", "unnamed", "ignore", "exclude"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "20240102_Family_Trip/a.jpg", "20240102_Family_Trip/Nested_Folder/b.jpg", "target.jpg")
			finishFace(t, l, 0)
			finishFace(t, l, 0)
			finishFace(t, l, 1)
			faces, _ := l.AutomaticFaces(ctx, "20240102_Family_Trip/a.jpg")
			targets, _ := l.AutomaticFaces(ctx, "target.jpg")
			id, target := faces[0].PersonID, targets[0].PersonID
			if err := l.RenamePerson(ctx, id, "Ada"); err != nil {
				t.Fatal(err)
			}
			if err := l.RenamePerson(ctx, target, "Other"); err != nil {
				t.Fatal(err)
			}
			for range 510 {
				if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, faces[0].ID); err != nil {
					t.Fatal(err)
				}
			}
			session, _ := l.LabelSession(ctx)
			if !session.PersonFolders {
				t.Fatal("capability missing")
			}
			page, err := l.LabelPersonFolders(ctx, id, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Folders) != 2 || len(page.Folders[0].Faces) != 8 || page.Folders[0].Count != 511 {
				t.Fatalf("page: %+v", page)
			}
			f := page.Folders[0]
			if f.DisplayPath != "Fotos / 02.01.2024 · Family Trip" || f.Faces[0].DisplayPath != "Fotos / 02.01.2024 · Family Trip / a.jpg" || len(f.Faces[0].OriginalKey) != 64 || f.Faces[0].Bounds.Width <= 0 {
				t.Fatalf("preview: %+v", f)
			}
			a := LabelAction{OperationID: "folder-operation-" + action, Dataset: session.Dataset, Revision: page.Revision, Action: "folder_" + action, Directory: &f.Directory}
			if action == "move" {
				p, _ := l.LabelPerson(ctx, target, 0)
				a.TargetID = target
				a.TargetRevision = p.Revision
			}
			if action == "new_name" {
				a.Action = "folder_move"
				a.Name = "New Person"
			}
			receipt, err := l.ApplyLabelAction(ctx, "editor", id, a)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Faces != 511 || receipt.SourceID != id || receipt.SourceRevision <= page.Revision {
				t.Fatalf("receipt: %+v", receipt)
			}
			replay, err := l.ApplyLabelAction(ctx, "editor", id, a)
			if err != nil || replay != receipt {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			saved, err := l.LabelReceipt(ctx, "editor", a.OperationID, session.Dataset)
			if err != nil || saved != receipt {
				t.Fatalf("receipt lookup: %+v %v", saved, err)
			}
			if _, err := l.LabelReceipt(ctx, "other", a.OperationID, session.Dataset); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("actor isolation: %v", err)
			}
			a.Name = "changed request"
			if _, err := l.ApplyLabelAction(ctx, "editor", id, a); err == nil {
				t.Fatal("reused operation with changed payload")
			}
			var remaining int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE person_id=? AND ignored=0`, id).Scan(&remaining); err != nil || remaining != 1 {
				t.Fatalf("remaining %d %v", remaining, err)
			}
			if action == "exclude" {
				page, err = l.LabelPersonFolders(ctx, id, 1)
				if err != nil || !page.Folders[0].Excluded || len(page.Folders[0].Faces) != 0 {
					t.Fatalf("exclusion: %+v %v", page, err)
				}
				a = LabelAction{OperationID: "folder-include-operation", Dataset: session.Dataset, Revision: page.Revision, Action: "folder_include", Directory: &f.Directory}
				included, err := l.ApplyLabelAction(ctx, "editor", id, a)
				if err != nil || included.Faces != 0 {
					t.Fatalf("include %+v %v", included, err)
				}
				again, err := l.ApplyLabelAction(ctx, "editor", id, a)
				if err != nil || again != included {
					t.Fatalf("include replay %+v %v", again, err)
				}
			}
		})
	}
}

func TestLabelFolderValidationAndRollback(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	others, _ := l.AutomaticFaces(ctx, "b.jpg")
	id, target := faces[0].PersonID, others[0].PersonID
	if err := l.RenamePerson(ctx, target, "Other"); err != nil {
		t.Fatal(err)
	}
	session, _ := l.LabelSession(ctx)
	page, _ := l.PersonFolders(ctx, id, 1)
	other, _ := l.LabelPerson(ctx, target, 0)
	root := ""
	original := LabelAction{OperationID: "folder-valid-operation", Dataset: session.Dataset, Revision: page.Revision, Action: "folder_move", Directory: &root, TargetID: target, TargetRevision: other.Revision}
	for i, change := range []func(*LabelAction){
		func(a *LabelAction) { a.Directory = nil },
		func(a *LabelAction) { bad := "../private"; a.Directory = &bad },
		func(a *LabelAction) { a.Revision++ },
		func(a *LabelAction) { a.TargetRevision++ },
		func(a *LabelAction) { a.Dataset = "other" },
		func(a *LabelAction) { a.TargetID = id },
		func(a *LabelAction) { a.Name = "also a name" },
		func(a *LabelAction) { a.FaceIDs = []int64{faces[0].ID} },
	} {
		a := original
		a.OperationID = fmt.Sprintf("folder-invalid-operation-%d", i)
		change(&a)
		if _, err := l.ApplyLabelAction(ctx, "editor", id, a); err == nil {
			t.Fatalf("invalid %d accepted", i)
		}
		p, err := l.PersonFolders(ctx, id, 1)
		if err != nil || p.Revision != page.Revision {
			t.Fatalf("partial write %d: %+v %v", i, p, err)
		}
		if _, err := l.LabelReceipt(ctx, "editor", a.OperationID, session.Dataset); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("invalid receipt: %v", err)
		}
	}
	// Exclusions still work after the last active face leaves the source.
	original.Action = "folder_exclude"
	original.TargetID = 0
	original.TargetRevision = 0
	if _, err := l.ApplyLabelAction(ctx, "editor", id, original); err != nil {
		t.Fatal(err)
	}
	excluded, err := l.LabelPersonFolders(ctx, id, 1)
	if err != nil || len(excluded.Folders) != 1 || !excluded.Folders[0].Excluded {
		t.Fatalf("last face: %+v %v", excluded, err)
	}
	original.Action = "folder_include"
	original.OperationID = "include-empty-person"
	original.Revision = excluded.Revision
	receipt, err := l.ApplyLabelAction(ctx, "editor", id, original)
	if err != nil {
		t.Fatal(err)
	}
	again, err := l.ApplyLabelAction(ctx, "editor", id, original)
	if err != nil || again != receipt {
		t.Fatalf("empty replay: %+v %v", again, err)
	}
}

func TestLabelFolderPagingPrivacyAndExcludedPeople(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a/a.jpg", "b/b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a/a.jpg")
	id := faces[0].PersonID
	if err := l.RenamePerson(ctx, id, "Ada"); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"a", "b"} {
		if err := folderAction(t, l, id, directory, "exclude", 0); err != nil {
			t.Fatal(err)
		}
	}
	session, _ := l.LabelSession(ctx)
	legacy, err := l.LabelNamedPeople(ctx, 0, session.UpperID)
	if err != nil || len(legacy.People) != 0 {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
	list, err := l.LabelNamedPeopleWithFolders(ctx, 0, session.UpperID, "Ada")
	if err != nil || len(list.People) != 1 || list.People[0].Count != 0 || list.People[0].FaceID != 0 {
		t.Fatalf("excluded person: %+v %v", list, err)
	}
	for i := range 81 {
		dir := fmt.Sprintf("folder%03d", i)
		if err := os.Mkdir(filepath.Join(l.root, dir), 0750); err != nil {
			t.Fatal(err)
		}
		if _, err := l.index.db.Exec(`INSERT INTO person_folder_exclusions VALUES(?,?)`, id, dir); err != nil {
			t.Fatal(err)
		}
	}
	for page, want := range []int{40, 40, 3} {
		p, err := l.LabelPersonFolders(ctx, id, page+1)
		if err != nil || len(p.Folders) != want || p.HasNext != (page < 2) {
			t.Fatalf("page %d: %+v %v", page+1, p, err)
		}
	}
	if err := os.WriteFile(filepath.Join(l.root, ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	list, err = l.LabelNamedPeopleWithFolders(ctx, 0, session.UpperID, "Ada")
	if err != nil || len(list.People) != 0 {
		t.Fatalf("private name leaked: %+v %v", list, err)
	}
	if _, err := l.LabelPersonFolders(ctx, id, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private folders: %v", err)
	}
}

func TestLabelExcludedPeoplePaginationSkipsPrivateWithoutLosingVisibleRows(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "visible/a.jpg", "private/b.jpg")
	if err := os.WriteFile(filepath.Join(l.root, "private", ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	var want []int64
	for i := range 45 {
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, fmt.Sprintf("Person %d", i), fmt.Sprintf("person %d", i))
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		dir := "visible"
		if i >= 18 && i < 23 {
			dir = "private"
		} else {
			want = append(want, id)
		}
		if _, err := l.index.db.Exec(`INSERT INTO person_folder_exclusions VALUES(?,?)`, id, dir); err != nil {
			t.Fatal(err)
		}
	}
	session, _ := l.LabelSession(ctx)
	var got []int64
	var cursor int64
	for range 4 {
		p, err := l.LabelNamedPeopleWithFolders(ctx, cursor, session.UpperID, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, person := range p.People {
			got = append(got, person.ID)
		}
		if !p.HasNext {
			break
		}
		if p.Next <= cursor {
			t.Fatal("non-progressing page")
		}
		cursor = p.Next
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs %v want %v", got, want)
	}
}

func TestLabelFolderReceiptFailureRollsBackAndConcurrentReplayCommitsOnce(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].PersonID
	page, _ := l.PersonFolders(ctx, id, 1)
	session, _ := l.LabelSession(ctx)
	root := ""
	a := LabelAction{OperationID: "folder-atomic-replay", Dataset: session.Dataset, Revision: page.Revision, Action: "folder_exclude", Directory: &root}
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'injected receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyLabelAction(ctx, "editor", id, a); err == nil {
		t.Fatal("receipt failure accepted")
	}
	unchanged, err := l.PersonFolders(ctx, id, 1)
	if err != nil || unchanged.Revision != page.Revision || unchanged.Folders[0].Excluded || unchanged.Folders[0].Count != 1 {
		t.Fatalf("partial commit: %+v %v", unchanged, err)
	}
	var count int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_people`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial target: %d %v", count, err)
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_receipt`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		receipt LabelReceipt
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { r, e := l.ApplyLabelAction(ctx, "editor", id, a); results <- result{r, e} }()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.receipt != second.receipt || first.receipt.Faces != 1 {
		t.Fatalf("replay: %+v %+v", first, second)
	}
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_labeling_actions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate receipt: %d %v", count, err)
	}
}
