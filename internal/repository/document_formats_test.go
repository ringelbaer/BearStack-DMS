package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/documentformat"
)

func TestThumbnailCandidatesMatchRendererAndPaginate(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "formats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	// Include every supported rule, parameter/case variants and unsupported gaps.
	type input struct{ name, mime string }
	inputs := []input{{"unknown.bin", "application/octet-stream"}, {"invalid.bin", "text/plain-extra"}, {"photo.webp", "image/webp"}}
	for _, format := range documentformat.Formats() {
		for _, mime := range format.MIMETypes {
			inputs = append(inputs, input{"opaque.bin", mime}, input{"opaque.bin", "\t" + strings.ToUpper(mime) + " \r\n; charset=utf-8"})
		}
		for _, ext := range format.Extensions {
			inputs = append(inputs, input{"name" + strings.ToUpper(ext), "application/octet-stream"}, input{"name" + ext + ".exe", "application/octet-stream"})
		}
	}
	var want []int64
	for i, in := range inputs {
		id, err := repo.CreateDocument(ctx, document.Document{OriginalName: in.name, MIMEType: in.mime, StoredPath: fmt.Sprint(i), Title: "format"})
		if err != nil {
			t.Fatal(err)
		}
		if documentformat.Classify(in.name, in.mime) != documentformat.Unknown {
			want = append(want, id)
		}
	}
	// Already rendered and trashed documents must never be selected.
	if err := repo.UpdateThumbnailPath(ctx, want[0], "ready.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, "UPDATE documents SET deleted_at = uploaded_at WHERE id = ?", want[1]); err != nil {
		t.Fatal(err)
	}
	want = want[2:]
	var got []int64
	var after int64
	for {
		page, err := repo.ThumbnailCandidatesAfter(ctx, after, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, doc := range page {
			if doc.ID <= after || documentformat.Classify(doc.OriginalName, doc.MIMEType) == documentformat.Unknown {
				t.Fatalf("invalid candidate: %#v", doc)
			}
			after = doc.ID
			got = append(got, doc.ID)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func BenchmarkThumbnailCandidatesSparseFormats(b *testing.B) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(b.TempDir(), "formats.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer repo.Close()
	// Sparse matches in a large backlog exercise filtering before LIMIT without
	// measuring import/search-index setup. The final 25 rows are supported.
	_, err = repo.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 50000)
	 INSERT INTO documents(original_name, stored_path, title, mime_type, size_bytes, sha256, uploaded_at, updated_at)
	 SELECT 'opaque.bin', x, 'benchmark', CASE WHEN x > 49975 THEN 'application/rtf; charset=utf-8' ELSE 'application/octet-stream' END, 0, '', '', '' FROM n`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		docs, err := repo.ThumbnailCandidatesAfter(ctx, 0, 25)
		if err != nil || len(docs) != 25 {
			b.Fatalf("candidates = %d, %v", len(docs), err)
		}
	}
}
