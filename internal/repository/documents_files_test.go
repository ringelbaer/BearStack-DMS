package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bearstack/internal/document"
)

func BenchmarkDocumentFileListing(b *testing.B) {
	ctx := context.Background()
	r := benchmarkRepository(b, ctx, 1000)
	for _, mode := range []string{"full-summary", "file-metadata"} {
		b.Run(mode, func(b *testing.B) {
			list := r.ListDocuments
			if mode == "file-metadata" {
				list = r.ListDocumentFiles
			}
			b.ReportAllocs()
			for b.Loop() {
				docs, err := list(ctx, document.ListFilter{Sort: document.ListSortDate})
				if err != nil || len(docs) != 1000 {
					b.Fatalf("listing: %d %v", len(docs), err)
				}
			}
		})
	}
}

func TestListDocumentFilesPreservesFiltersOrderAndFileMetadata(t *testing.T) {
	ctx := context.Background()
	r, err := Open(ctx, filepath.Join(t.TempDir(), "files.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	date := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		doc := document.Document{
			OriginalName: fmt.Sprintf("scan-%d.pdf", i), StoredPath: fmt.Sprintf("stored/%d.pdf", i),
			Title: fmt.Sprintf("Title %d", i%3), Description: "description only", ContentText: "OCR searchable content",
			MIMEType: "application/pdf", SizeBytes: int64(i + 1), SHA256: fmt.Sprint(i % 2),
			UploadedAt: date.Add(time.Duration(i) * time.Hour), UpdatedAt: date,
			Tags: []string{fmt.Sprintf("tag%d", i%2)},
		}
		if i%2 == 0 {
			doc.DocumentDate = &date
		}
		id, err := r.CreateDocument(ctx, doc)
		if err != nil {
			t.Fatal(err)
		}
		if i == 7 {
			if _, err := r.db.Exec(`UPDATE documents SET deleted_at=? WHERE id=?`, formatTime(date), id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := r.db.Exec(`INSERT INTO custom_fields(id,label) VALUES(1,'Customer'); INSERT INTO document_custom_values(document_id,field_id,value) VALUES(1,1,'Acme')`); err != nil {
		t.Fatal(err)
	}
	filters := []document.ListFilter{
		{}, {Trash: true}, {Tags: []string{"tag0"}}, {Query: "searchable"}, {From: &date, To: &date},
		{CustomFields: []document.CustomFieldFilter{{FieldID: 1, Value: "Acme", Exact: true}}},
		{Limit: 2, Offset: 2}, {Tags: []string{"absent"}},
	}
	for _, sort := range []string{document.ListSortDate, document.ListSortName, document.ListSortTitle, document.ListSortSize} {
		for _, direction := range []string{document.ListDirectionAscending, document.ListDirectionDescending} {
			filters = append(filters, document.ListFilter{Sort: sort, Direction: direction})
		}
	}
	for _, filter := range filters {
		want, err := r.ListDocuments(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.ListDocumentFiles(ctx, filter)
		if err != nil || len(got) != len(want) {
			t.Fatalf("%+v: files=%d list=%d err=%v", filter, len(got), len(want), err)
		}
		for i, full := range want {
			file := document.Document{ID: full.ID, OriginalName: full.OriginalName, StoredPath: full.StoredPath,
				Title: full.Title, MIMEType: full.MIMEType, SizeBytes: full.SizeBytes, SHA256: full.SHA256,
				DocumentDate: full.DocumentDate, UploadedAt: full.UploadedAt, UpdatedAt: full.UpdatedAt, DeletedAt: full.DeletedAt}
			if !reflect.DeepEqual(got[i], file) {
				t.Fatalf("%+v: file[%d]=%+v, want %+v", filter, i, got[i], file)
			}
		}
	}
	// Auxiliary data is not required even for a nonempty listing.
	if _, err := r.db.Exec(`DROP TABLE document_links; DROP TABLE document_custom_values; DROP TABLE document_tags`); err != nil {
		t.Fatal(err)
	}
	if docs, err := r.ListDocumentFiles(ctx, document.ListFilter{}); err != nil || len(docs) != 7 {
		t.Fatalf("file listing accessed auxiliary tables: %d %v", len(docs), err)
	}
}
