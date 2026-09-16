package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFaceReconciliationWholeGroupsPriority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores []float32
		named  []int64
		margin float64
		want   int64
	}{
		{"named before closer unnamed", []float32{.7, .99}, []int64{2}, .1, 2},
		{"named margin ignores unnamed", []float32{.75, .6, .99}, []int64{2, 3}, .1, 2},
		{"unnamed after weak named", []float32{.6, .8}, []int64{2}, .1, 3},
		{"unnamed after ambiguous named", []float32{.7, .68, .9}, []int64{2, 3}, .1, 4},
		{"strict similarity", []float32{.6}, nil, .1, 1},
		{"strict unnamed margin", []float32{.8, .75}, nil, .1, 1},
		{"unnamed runner up below threshold", []float32{.65, .6}, nil, .1, 1},
		{"unnamed clear winner", []float32{.8, .6}, nil, .1, 2},
		{"zero margin allows ties", []float32{.8, .8}, nil, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"source/a.jpg"}
			vectors := [][]float32{faceDetection(0).Embedding}
			counts := []int{3}
			for i, score := range tc.scores {
				paths = append(paths, fmt.Sprintf("target/%d.jpg", i))
				vectors = append(vectors, matchingVector(score))
				counts = append(counts, 3)
			}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, vectors, counts)
			for _, id := range tc.named {
				if err := l.RenamePerson(ctx, id, fmt.Sprintf("Named %d", id)); err != nil {
					t.Fatal(err)
				}
			}
			v := DefaultFaceThresholds()
			v.ReconcileUnnamedGroups, v.ReconcileMargin = true, tc.margin
			if err := l.SetFaceThresholds(ctx, v); err != nil {
				t.Fatal(err)
			}
			state, err := l.ReconcileFacesBatch(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			for id := int64(1); id <= 3; id++ {
				f, err := l.Face(ctx, id)
				if err != nil || f.PersonID != tc.want || f.Manual {
					t.Fatalf("face %d: %+v %v, want person %d", id, f, err, tc.want)
				}
			}
			wantMoved := int64(0)
			if tc.want != 1 {
				wantMoved = 3
			}
			if state.Reassigned != wantMoved {
				t.Fatalf("moved %d, want %d", state.Reassigned, wantMoved)
			}
		})
	}
}

func TestFaceReconciliationWholeGroupUsesAllReferencesForNamedPriority(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "source/a.jpg", "named/b.jpg", "unnamed/c.jpg")
	seedMatchingPeople(t, l, []string{"source/a.jpg", "named/b.jpg", "unnamed/c.jpg"}, [][]float32{faceDetection(0).Embedding, faceDetection(1).Embedding, matchingVector(.9)}, []int{2, 1, 1})
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=2; UPDATE photo_face_state SET revision=revision+1 WHERE id=1`, encodeVector(faceDetection(1).Embedding)); err != nil {
		t.Fatal(err)
	}
	if err := l.RenamePerson(ctx, 2, "Named"); err != nil {
		t.Fatal(err)
	}
	v := DefaultFaceThresholds()
	v.ReconcileUnnamedGroups = true
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReconcileFacesBatch(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		f, err := l.Face(ctx, id)
		if err != nil || f.PersonID != 2 {
			t.Fatalf("named reference on second source face ignored: %+v %v", f, err)
		}
	}
}

func TestFaceReconciliationWholeGroupSkipsIneligibleTargets(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(fmt.Sprintf("named=%v", named), func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"source.jpg", "blocked.jpg", "allowed.jpg"}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, [][]float32{faceDetection(0).Embedding, matchingVector(.95), matchingVector(.7)}, []int{2, 1, 1})
			stmt := `UPDATE photo_people SET manual_name=1 WHERE id=2`
			if named {
				stmt = `UPDATE photo_people SET name='Unconfirmed' WHERE id=2`
				if err := l.RenamePerson(ctx, 3, "Confirmed"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := l.index.db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
			v := DefaultFaceThresholds()
			v.ReconcileUnnamedGroups = true
			if err := l.SetFaceThresholds(ctx, v); err != nil {
				t.Fatal(err)
			}
			if _, err := l.ReconcileFacesBatch(ctx, 1); err != nil {
				t.Fatal(err)
			}
			f, err := l.Face(ctx, 1)
			if err != nil || f.PersonID != 3 {
				t.Fatalf("ineligible leader starved valid group: %+v %v", f, err)
			}
		})
	}
}

func TestFaceReconciliationWholeGroupConfiguredSimilarity(t *testing.T) {
	for _, threshold := range []float64{.6, .7} {
		t.Run(fmt.Sprint(threshold), func(t *testing.T) {
			ctx := context.Background()
			l, source, target := reconciliationFixture(t, .65, false)
			v := DefaultFaceThresholds()
			v.ReconcileUnnamedGroups, v.ReconcileSimilarity = true, threshold
			if err := l.SetFaceThresholds(ctx, v); err != nil {
				t.Fatal(err)
			}
			if _, err := l.ReconcileFacesBatch(ctx, 1); err != nil {
				t.Fatal(err)
			}
			f, err := l.Face(ctx, source.ID)
			if err != nil || (f.PersonID == target.PersonID) != (threshold == .6) {
				t.Fatalf("configured threshold %v ignored: %+v %v", threshold, f, err)
			}
		})
	}
}

func TestFaceReconciliationWholeGroupProtections(t *testing.T) {
	for _, protect := range []string{"disabled", "manual member", "favorite member", "ignored member", "drawn member", "weak member", "old model", "split source", "split target", "unconfirmed name", "shared other photo", "private member", "XMP member", "replaced member", "missing member", "changed sidecar", "rejected"} {
		t.Run(protect, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "source/a.jpg", "source/b.jpg", "target/c.jpg", "private/d.jpg")
			seedMatchingPeople(t, l, []string{"source/a.jpg", "target/c.jpg"}, [][]float32{faceDetection(0).Embedding, matchingVector(.9)}, []int{2, 1})
			if _, err := l.index.db.Exec(`UPDATE photo_faces SET path='source/b.jpg' WHERE id=2`); err != nil {
				t.Fatal(err)
			}
			var stmt string
			switch protect {
			case "manual member":
				stmt = `UPDATE photo_faces SET manual=1 WHERE id=2`
			case "favorite member":
				stmt = `UPDATE photo_faces SET favorite=1 WHERE id=2`
			case "ignored member":
				stmt = `UPDATE photo_faces SET ignored=1 WHERE id=2`
			case "drawn member":
				stmt = `UPDATE photo_faces SET drawn=1 WHERE id=2`
			case "weak member":
				stmt = `UPDATE photo_faces SET reference_eligible=0 WHERE id=2`
			case "old model":
				stmt = `UPDATE photo_faces SET model='old' WHERE id=2`
			case "split source":
				stmt = `UPDATE photo_people SET manual_name=1 WHERE id=1`
			case "split target":
				stmt = `UPDATE photo_people SET manual_name=1 WHERE id=2`
			case "unconfirmed name":
				stmt = `UPDATE photo_people SET name='Imported',manual_name=0 WHERE id=2`
			case "shared other photo":
				stmt = `UPDATE photo_faces SET path='source/b.jpg' WHERE id=3`
			case "private member":
				stmt = `UPDATE photo_faces SET path='private/d.jpg',directory='private' WHERE id=2`
				if err := os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "XMP member":
				stmt = `UPDATE media_index SET faces='[{"Name":"Imported","X":0.2,"Y":0.2,"Width":0.3,"Height":0.3}]' WHERE path='source/b.jpg'`
			case "replaced member":
				if err := os.WriteFile(filepath.Join(l.Root(), "source/b.jpg"), []byte("replaced"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing member":
				if err := os.Remove(filepath.Join(l.Root(), "source/b.jpg")); err != nil {
					t.Fatal(err)
				}
			case "changed sidecar":
				if err := os.WriteFile(filepath.Join(l.Root(), "source/b.jpg.xmp"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "rejected":
				stmt = `INSERT INTO photo_face_merge_suggestions(source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model,rejected) VALUES(1,2,1,1,1,3,.9,'test',1)`
			}
			if stmt != "" {
				if _, err := l.index.db.Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			v := DefaultFaceThresholds()
			v.ReconcileUnnamedGroups = protect != "disabled"
			if err := l.SetFaceThresholds(ctx, v); err != nil {
				t.Fatal(err)
			}
			state, err := l.ReconcileFacesBatch(ctx, 1)
			if err != nil || state.Reassigned != 0 {
				t.Fatalf("protected group changed: %+v %v", state, err)
			}
			var person int64
			if err := l.index.db.QueryRow(`SELECT person_id FROM photo_faces WHERE id=1`).Scan(&person); err != nil || person != 1 {
				t.Fatalf("source moved: %d %v", person, err)
			}
		})
	}
}

func TestFaceReconciliationWholeGroupsPreserveTagsRejectionsAndResume(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	seedMatchingPeople(t, l, []string{"a.jpg", "b.jpg", "c.jpg"}, [][]float32{faceDetection(0).Embedding, matchingVector(.9), matchingVector(.95)}, []int{2, 1, 1})
	if _, err := l.index.db.Exec(`INSERT INTO person_tag_index VALUES(1,'source'),(2,'target');
 INSERT INTO photo_face_merge_suggestions(source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model,rejected) VALUES(3,1,1,1,4,1,.95,'test',1)`); err != nil {
		t.Fatal(err)
	}
	v := DefaultFaceThresholds()
	v.ReconcileUnnamedGroups = true
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	state, err := l.ReconcileFacesBatch(ctx, 1)
	if err != nil || state.Reassigned != 2 {
		t.Fatalf("first merge: %+v %v", state, err)
	}
	var rejected bool
	if err := l.index.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM photo_face_merge_suggestions WHERE source_id=3 AND target_id=2 AND rejected=1)`).Scan(&rejected); err != nil || !rejected {
		t.Fatalf("rejection lost: %v %v", rejected, err)
	}
	if tags, err := l.personTags(ctx, 2); err != nil || fmt.Sprint(tags) != "[source target]" {
		t.Fatalf("tags: %v %v", tags, err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	completeReconciliation(t, reopened)
	var groups int
	if err := reopened.index.db.QueryRow(`SELECT count(DISTINCT person_id) FROM photo_faces`).Scan(&groups); err != nil || groups != 2 {
		t.Fatalf("rejected identities merged after restart: %d %v", groups, err)
	}
}

func TestFaceReconciliationWholeGroupRollbackAndLargeGroup(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "source.jpg", "target.jpg")
	const count = 2000
	seedMatchingPeople(t, l, []string{"source.jpg", "target.jpg"}, [][]float32{faceDetection(0).Embedding, matchingVector(.9)}, []int{count, 1})
	v := DefaultFaceThresholds()
	v.ReconcileUnnamedGroups = true
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO person_tag_index VALUES(1,'source');
 CREATE TRIGGER fail_group_merge BEFORE UPDATE OF person_id ON photo_faces WHEN old.id=1000 BEGIN SELECT RAISE(ABORT,'injected group failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReconcileFacesBatch(ctx, 1); err == nil {
		t.Fatal("expected merge failure")
	}
	var members, tags int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE person_id=1`).Scan(&members); err != nil || members != count {
		t.Fatalf("partial merge: %d %v", members, err)
	}
	if err := l.index.db.QueryRow(`SELECT count(*) FROM person_tag_index WHERE person_id=2`).Scan(&tags); err != nil || tags != 0 {
		t.Fatalf("partial tag transfer: %d %v", tags, err)
	}
	state, err := l.FaceReconciliationStatus(ctx)
	if err != nil || state.Cursor != 0 || state.Processed != 0 || state.Reassigned != 0 {
		t.Fatalf("partial progress: %+v %v", state, err)
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_group_merge`); err != nil {
		t.Fatal(err)
	}
	state, err = l.ReconcileFacesBatch(ctx, 1)
	if err != nil || state.Reassigned != count {
		t.Fatalf("large group merge: %+v %v", state, err)
	}
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE person_id=2 AND manual=0`).Scan(&members); err != nil || members != count+1 {
		t.Fatalf("large group not fully merged: %d %v", members, err)
	}
	// A whole merge must refresh its references before the next matching batch.
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references WHERE person_id=1`).Scan(&members); err != nil || members != 0 {
		t.Fatalf("obsolete references: %d %v", members, err)
	}
}

func TestFaceReconciliationWholeGroupFamilyConflictRollsBack(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "source.jpg", "target.jpg", "child.jpg")
	seedMatchingPeople(t, l, []string{"source.jpg", "target.jpg", "child.jpg"}, [][]float32{faceDetection(0).Embedding, matchingVector(.9), faceDetection(2).Embedding}, []int{2, 1, 1})
	// Both groups are parents of one child: combining them would violate the
	// distinct-parent rule after relationship transfer has already started.
	if _, err := l.index.db.Exec(`INSERT INTO person_parents(person_id,role,parent_id) VALUES(3,'mother',1),(3,'father',2)`); err != nil {
		t.Fatal(err)
	}
	v := DefaultFaceThresholds()
	v.ReconcileUnnamedGroups = true
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	state, err := l.ReconcileFacesBatch(ctx, 1)
	if err != nil || state.Reassigned != 0 {
		t.Fatalf("family conflict changed group: %+v %v", state, err)
	}
	var mother, father int64
	if err := l.index.db.QueryRow(`SELECT parent_id FROM person_parents WHERE person_id=3 AND role='mother'`).Scan(&mother); err != nil {
		t.Fatal(err)
	}
	if err := l.index.db.QueryRow(`SELECT parent_id FROM person_parents WHERE person_id=3 AND role='father'`).Scan(&father); err != nil {
		t.Fatal(err)
	}
	if mother != 1 || father != 2 {
		t.Fatalf("partial family transfer: mother=%d father=%d", mother, father)
	}
}
