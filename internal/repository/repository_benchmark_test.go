package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
)

func BenchmarkListDocumentsSearch(b *testing.B) {
	ctx := context.Background()
	for _, size := range []int{1000, 10000, 50000} {
		repo := benchmarkRepository(b, ctx, size)

		b.Run(fmt.Sprintf("%d_docs_unique_term", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Query: "needle-000042",
				Sort:  "date",
			})
		})

		b.Run(fmt.Sprintf("%d_docs_no_hit", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Query: "not-present-anywhere",
				Sort:  "date",
			})
		})

		b.Run(fmt.Sprintf("%d_docs_tag_filter", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Tags: []string{"steuer"},
				Sort: "date",
			})
		})

		b.Run(fmt.Sprintf("%d_docs_tag_filter_limit_100", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Tags:  []string{"steuer"},
				Sort:  "date",
				Limit: 100,
			})
		})

		b.Run(fmt.Sprintf("%d_docs_multi_tag_filter_limit_100", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Tags:  []string{"steuer", "laufend"},
				Sort:  "date",
				Limit: 100,
			})
		})

		b.Run(fmt.Sprintf("%d_docs_common_term", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Query: "rechnung",
				Sort:  "date",
			})
		})

		b.Run(fmt.Sprintf("%d_docs_common_term_limit_100", size), func(b *testing.B) {
			benchmarkListDocuments(b, repo, document.ListFilter{
				Query: "rechnung",
				Sort:  "date",
				Limit: 100,
			})
		})
	}
}

func BenchmarkListDocumentPages(b *testing.B) {
	ctx := context.Background()
	for _, size := range []int{1000, 10000, 50000} {
		repo := benchmarkRepository(b, ctx, size)

		b.Run(fmt.Sprintf("%d_docs_default_upload_date_limit_100", size), func(b *testing.B) {
			benchmarkListDocumentPage(b, repo, document.ListFilter{
				Sort:  "upload_date",
				Limit: 100,
			})
		})

		b.Run(fmt.Sprintf("%d_docs_tag_filter_limit_100", size), func(b *testing.B) {
			benchmarkListDocumentPage(b, repo, document.ListFilter{
				Tags:  []string{"steuer"},
				Sort:  "upload_date",
				Limit: 100,
			})
		})

		b.Run(fmt.Sprintf("%d_docs_common_term_limit_100", size), func(b *testing.B) {
			benchmarkListDocumentPage(b, repo, document.ListFilter{
				Query: "rechnung",
				Sort:  "upload_date",
				Limit: 100,
			})
		})
	}
}

func benchmarkListDocuments(b *testing.B, repo *Repository, filter document.ListFilter) {
	b.Helper()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		docs, err := repo.ListDocuments(ctx, filter)
		if err != nil {
			b.Fatal(err)
		}
		if filter.Query == "needle-000042" && len(docs) != 1 {
			b.Fatalf("len(docs) = %d, want 1", len(docs))
		}
	}
}

func benchmarkListDocumentPage(b *testing.B, repo *Repository, filter document.ListFilter) {
	b.Helper()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total, err := repo.CountDocuments(ctx, filter)
		if err != nil {
			b.Fatal(err)
		}
		docs, err := repo.ListDocuments(ctx, filter)
		if err != nil {
			b.Fatal(err)
		}
		if len(docs) > total {
			b.Fatalf("len(docs) = %d, total = %d", len(docs), total)
		}
		if filter.Limit > 0 && len(docs) > filter.Limit {
			b.Fatalf("len(docs) = %d, limit = %d", len(docs), filter.Limit)
		}
	}
}

func benchmarkRepository(b *testing.B, ctx context.Context, size int) *Repository {
	b.Helper()

	repo, err := Open(ctx, filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := repo.Close(); err != nil {
			b.Fatal(err)
		}
	})

	seedBenchmarkDocuments(b, ctx, repo, size)
	return repo
}

func seedBenchmarkDocuments(b *testing.B, ctx context.Context, repo *Repository, size int) {
	b.Helper()

	tags := []string{"rechnung", "steuer", "privat", "haus", "bank", "versicherung", "laufend"}
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()

	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(name) VALUES (?)`, tag); err != nil {
			b.Fatal(err)
		}
	}

	insertDoc, err := tx.PrepareContext(ctx, `
		INSERT INTO documents (
			original_name, stored_path, title, description, mime_type,
			size_bytes, sha256, document_date, uploaded_at, updated_at, content_text,
			search_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer insertDoc.Close()

	insertTag, err := tx.PrepareContext(ctx, `
		INSERT INTO document_tags(document_id, tag_id)
		VALUES (?, (SELECT id FROM tags WHERE name = ?))`)
	if err != nil {
		b.Fatal(err)
	}
	defer insertTag.Close()

	insertSearch, err := tx.PrepareContext(ctx, `
		INSERT INTO document_search(rowid, original_name, title, description, tags, search_text)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer insertSearch.Close()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < size; i++ {
		tag := tags[i%len(tags)]
		unique := "doc-" + strconv.Itoa(i)
		if i == 42 {
			unique += " needle-000042"
		}
		docTags := []string{tag}
		if tag == "steuer" {
			docTags = append(docTags, "laufend")
		}
		searchText := strings.Join([]string{
			"rechnung",
			"titel",
			unique,
			strings.Join(docTags, " "),
			"lieferant",
			fmt.Sprintf("betrag-%04d", i%1000),
		}, " ")
		when := baseTime.Add(time.Duration(i) * time.Minute)
		result, err := insertDoc.ExecContext(
			ctx,
			fmt.Sprintf("2024-01-%02d_Rechnung_%06d.pdf", (i%28)+1, i),
			fmt.Sprintf("2024/01/doc-%06d.pdf", i),
			fmt.Sprintf("Rechnung %06d", i),
			"",
			"application/pdf",
			int64(10000+i%5000),
			fmt.Sprintf("sha-%08d", i),
			when.Format("2006-01-02"),
			formatTime(when),
			formatTime(when),
			searchText,
			document.CurrentSearchVersion,
		)
		if err != nil {
			b.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			b.Fatal(err)
		}
		for _, docTag := range docTags {
			if _, err := insertTag.ExecContext(ctx, id, docTag); err != nil {
				b.Fatal(err)
			}
		}
		if _, err := insertSearch.ExecContext(
			ctx,
			id,
			fmt.Sprintf("2024-01-%02d_Rechnung_%06d.pdf", (i%28)+1, i),
			fmt.Sprintf("Rechnung %06d", i),
			"",
			strings.Join(docTags, " "),
			searchText,
		); err != nil {
			b.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}
