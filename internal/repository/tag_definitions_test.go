package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"bearstack/internal/document"
)

func TestTagDefinitionsPreserveMetadataWithoutReadingAssignments(t *testing.T) {
	ctx := context.Background()
	r, err := Open(ctx, filepath.Join(t.TempDir(), "tags.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.CreateDocument(ctx, document.Document{OriginalName: "a.pdf", StoredPath: "a.pdf", Title: "A", MIMEType: "application/pdf", SHA256: "a", Tags: []string{"alpha", "beta"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE tags SET description='Description', color='#123456', primary_tag=1, group_mode=1, list_hidden=1, delete_protected=1 WHERE name='alpha'`); err != nil {
		t.Fatal(err)
	}
	withCounts, err := r.ListTags(ctx)
	if err != nil || len(withCounts) != 2 || withCounts[0].Count != 1 || withCounts[1].Count != 1 {
		t.Fatalf("counted tags = %+v, err=%v", withCounts, err)
	}
	for i := range withCounts {
		withCounts[i].Count = 0
	}
	// A metadata-only lookup must not require the document assignment table.
	if _, err := r.db.Exec(`DROP TABLE document_tags`); err != nil {
		t.Fatal(err)
	}
	definitions, err := r.ListTagDefinitions(ctx)
	if err != nil || !reflect.DeepEqual(definitions, withCounts) {
		t.Fatalf("definitions = %+v, err=%v, want %+v", definitions, err, withCounts)
	}
	existing, err := r.ExistingTagNames(ctx, []string{" ALPHA ", "alpha", "unknown"})
	if err != nil || !reflect.DeepEqual(existing, map[string]struct{}{"alpha": {}}) {
		t.Fatalf("existing names = %v, err=%v", existing, err)
	}
	if existing, err := r.ExistingTagNames(ctx, nil); err != nil || len(existing) != 0 {
		t.Fatalf("empty lookup = %v, %v", existing, err)
	}
}

func TestExistingTagNamesSpansBatches(t *testing.T) {
	ctx := context.Background()
	r, err := Open(ctx, filepath.Join(t.TempDir(), "tags.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := make([]string, repositoryBatchSize+1)
	for i := range names {
		names[i] = fmt.Sprintf("tag-%04d", i)
	}
	if _, err := r.db.Exec(`INSERT INTO tags(name) VALUES(?), (?)`, names[0], names[len(names)-1]); err != nil {
		t.Fatal(err)
	}
	existing, err := r.ExistingTagNames(ctx, names)
	want := map[string]struct{}{names[0]: {}, names[len(names)-1]: {}}
	if err != nil || !reflect.DeepEqual(existing, want) {
		t.Fatalf("existing names = %v, err=%v, want %v", existing, err, want)
	}
}
