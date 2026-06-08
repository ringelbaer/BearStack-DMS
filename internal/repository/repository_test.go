package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

func TestRepositoryRecordsSchemaVersion(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	version, found, err := sqlutil.CurrentSchemaVersion(ctx, repo.db, repositorySchemaComponent)
	if err != nil {
		t.Fatal(err)
	}
	if !found || version != repositorySchemaVersion {
		t.Fatalf("schema version = %d, found %v; want %d", version, found, repositorySchemaVersion)
	}
}

func TestRepositorySchemaMigrationsEndAtSupportedVersion(t *testing.T) {
	if len(repositorySchemaMigrations) == 0 {
		t.Fatal("repository schema migrations are empty")
	}
	if got := repositorySchemaMigrations[len(repositorySchemaMigrations)-1].Version; got != repositorySchemaVersion {
		t.Fatalf("last migration version = %d, supported = %d", got, repositorySchemaVersion)
	}
}

func TestRepositoryRejectsNewerSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlutil.RecordSchemaVersion(ctx, db, repositorySchemaComponent, repositorySchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err := Open(ctx, dbPath)
	if repo != nil {
		_ = repo.Close()
	}
	if err == nil {
		t.Fatal("expected newer schema version to be rejected")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryCreatesDocumentSortIndexes(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	required := map[string]bool{
		"idx_documents_active_original_name": false,
		"idx_documents_trash_original_name":  false,
		"idx_documents_active_title":         false,
		"idx_documents_trash_title":          false,
		"idx_documents_active_size":          false,
		"idx_documents_trash_size":           false,
		"idx_documents_post_import_pending":  false,
	}
	rows, err := repo.db.QueryContext(ctx, `PRAGMA index_list(documents)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		if _, ok := required[name]; ok {
			if partial != 1 {
				t.Fatalf("%s is not a partial index", name)
			}
			required[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for name, found := range required {
		if !found {
			t.Fatalf("missing index %s", name)
		}
	}
}

func TestRepositoryCreateDocumentMarksPostImportPending(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "note.txt",
		StoredPath:        "2026/05/note.txt",
		UploadWay:         document.UploadWayAPI,
		Title:             "note",
		MIMEType:          "text/plain",
		SizeBytes:         12,
		SHA256:            "sum",
		ContentTextSource: document.ContentTextSourceNone,
		SearchVersion:     document.CurrentSearchVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	pending, err := repo.PostImportPendingDocuments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != id || pending[0].StoredPath != "2026/05/note.txt" {
		t.Fatalf("pending docs = %#v, want document %d", pending, id)
	}

	if err := repo.MarkPostImportAttempted(ctx, id); err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "second.txt",
		StoredPath:        "2026/05/second.txt",
		UploadWay:         document.UploadWayAPI,
		Title:             "second",
		MIMEType:          "text/plain",
		SizeBytes:         13,
		SHA256:            "sum2",
		ContentTextSource: document.ContentTextSourceNone,
		SearchVersion:     document.CurrentSearchVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err = repo.PostImportPendingDocuments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].ID != secondID || pending[1].ID != id {
		t.Fatalf("pending docs after retry mark = %#v, want new document before attempted document", pending)
	}

	if err := repo.MarkPostImportComplete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkPostImportComplete(ctx, secondID); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.PostImportPendingDocuments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending docs after complete = %#v, want none", pending)
	}
}

func TestRepositoryCreatesTagFilterIndex(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	var found bool
	rows, err := repo.db.QueryContext(ctx, `PRAGMA index_list(document_tags)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		if name == "idx_document_tags_tag_document" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("missing idx_document_tags_tag_document")
	}
}

func TestRepositoryCreatesPerformanceIndexes(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	required := map[string][]string{
		"documents": {
			"idx_documents_active_sha256",
			"idx_documents_deleted_document_date",
			"idx_documents_thumbnail_candidates",
		},
		"document_custom_values": {
			"idx_document_custom_values_field_value_document",
		},
		"ocr_jobs": {
			"idx_ocr_jobs_document_id_desc",
		},
	}
	for table, names := range required {
		found, err := repositoryIndexNames(ctx, repo, table)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if !found[name] {
				t.Fatalf("missing index %s on %s", name, table)
			}
		}
	}
}

func repositoryIndexNames(ctx context.Context, repo *Repository, table string) (map[string]bool, error) {
	rows, err := repo.db.QueryContext(ctx, `PRAGMA index_list(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		found[name] = true
	}
	return found, rows.Err()
}

func TestRepositoryConfiguresSQLiteConnectionPool(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if got := repo.db.Stats().MaxOpenConnections; got != sqliteMaxOpenConns || got <= 1 {
		t.Fatalf("max open connections = %d", got)
	}
}

func TestSQLiteDSNAddsPragmas(t *testing.T) {
	dsn, err := sqliteDSN("file:test.db?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pragmas := parsed.Query()["_pragma"]
	for _, want := range []string{"busy_timeout(5000)", "foreign_keys(ON)", "journal_mode(WAL)"} {
		if !containsString(pragmas, want) {
			t.Fatalf("pragma %q missing in %q", want, dsn)
		}
	}
}

func TestRepositoryDocumentLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "2024-03-15_Rechnung.pdf",
		StoredPath:   "2024/03/2024-03-15_Rechnung.pdf",
		Title:        "Rechnung",
		Description:  "Strom",
		Tags:         []string{"steuer", "privat"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "abc",
		DocumentDate: &docDate,
		ContentText:  "inhalt stromtarif",
	})
	if err != nil {
		t.Fatal(err)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{Query: "strom", Tags: []string{"steuer"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != id {
		t.Fatalf("docs = %#v", docs)
	}
	if docs[0].UploadWay != document.UploadWayWeb {
		t.Fatalf("upload way = %q", docs[0].UploadWay)
	}
	if len(docs[0].Tags) != 2 {
		t.Fatalf("tags = %#v", docs[0].Tags)
	}
	docs, err = repo.ListDocuments(ctx, document.ListFilter{Tags: []string{"steuer", "privat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != id {
		t.Fatalf("multi tag docs = %#v", docs)
	}

	newDocDate := time.Date(2024, 4, 10, 0, 0, 0, 0, time.UTC)
	if err := repo.UpdateMetadata(ctx, id, "Neue Rechnung", "bezahlt", &newDocDate, []string{"archiv"}, nil); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetDocument(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Neue Rechnung" || len(updated.Tags) != 1 || updated.Tags[0] != "archiv" {
		t.Fatalf("updated = %#v", updated)
	}
	if updated.ContentText != "inhalt stromtarif" {
		t.Fatalf("content text was not preserved: %q", updated.ContentText)
	}
	if updated.DocumentDate == nil || !updated.DocumentDate.Equal(newDocDate) {
		t.Fatalf("document date was not updated: %#v", updated.DocumentDate)
	}

	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	active, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active = %#v", active)
	}
	trash, err := repo.ListDocuments(ctx, document.ListFilter{Trash: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 {
		t.Fatalf("trash = %#v", trash)
	}

	if err := repo.Restore(ctx, id); err != nil {
		t.Fatal(err)
	}
	active, err = repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active after restore = %#v", active)
	}
}

func TestRepositoryListDocumentsUsesSummaryRows(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	body := strings.Repeat("needle content ", 1024)
	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "text.pdf",
		StoredPath:        "2024/03/text.pdf",
		Title:             "Text",
		MIMEType:          "application/pdf",
		SizeBytes:         42,
		SHA256:            "summary",
		ContentText:       body,
		ContentTextSource: document.ContentTextSourcePDF,
	})
	if err != nil {
		t.Fatal(err)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{Query: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != id {
		t.Fatalf("docs = %#v", docs)
	}
	if docs[0].ContentText != "" {
		t.Fatalf("list loaded content text: %d bytes", len(docs[0].ContentText))
	}
	if docs[0].ContentTextSource != document.ContentTextSourcePDF {
		t.Fatalf("content text source = %q", docs[0].ContentTextSource)
	}

	full, err := repo.GetDocument(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if full.ContentText != body {
		t.Fatalf("full document text not loaded: content=%d", len(full.ContentText))
	}
}

func TestRepositoryGetDocumentFileDoesNotLoadContentText(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	body := strings.Repeat("ocr text ", 2048)
	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "large-text.pdf",
		StoredPath:   "2024/03/large-text.pdf",
		Title:        "Large Text",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "file-view",
		ContentText:  body,
	})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := repo.GetDocumentFile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != id || doc.StoredPath != "2024/03/large-text.pdf" || doc.OriginalName != "large-text.pdf" {
		t.Fatalf("doc = %#v", doc)
	}
	if doc.ContentText != "" {
		t.Fatalf("file document loaded content text: %d bytes", len(doc.ContentText))
	}
}

func TestRepositoryListByIDsEmptyDoesNotListAllDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "doc.pdf",
		StoredPath:   "2024/03/doc.pdf",
		Title:        "Doc",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "list-by-ids-empty",
	}); err != nil {
		t.Fatal(err)
	}

	docs, err := repo.ListByIDs(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("ListByIDs(nil) returned %d document(s), want 0", len(docs))
	}
}

func TestRepositoryDuplicateGroupsLoadsDocumentsInGroupOrder(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	firstTime := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Hour)
	docs := []document.Document{
		{OriginalName: "a.pdf", StoredPath: "2026/05/a.pdf", Title: "A", MIMEType: "application/pdf", SizeBytes: 10, SHA256: "same-a", UploadedAt: firstTime, Tags: []string{"steuer"}},
		{OriginalName: "b.pdf", StoredPath: "2026/05/b.pdf", Title: "B", MIMEType: "application/pdf", SizeBytes: 10, SHA256: "same-a", UploadedAt: secondTime, Tags: []string{"steuer"}},
		{OriginalName: "c.pdf", StoredPath: "2026/05/c.pdf", Title: "C", MIMEType: "application/pdf", SizeBytes: 20, SHA256: "same-b", UploadedAt: firstTime},
		{OriginalName: "d.pdf", StoredPath: "2026/05/d.pdf", Title: "D", MIMEType: "application/pdf", SizeBytes: 20, SHA256: "same-b", UploadedAt: secondTime.Add(time.Hour)},
		{OriginalName: "single.pdf", StoredPath: "2026/05/single.pdf", Title: "Single", MIMEType: "application/pdf", SizeBytes: 1, SHA256: "single", UploadedAt: firstTime},
	}
	for _, doc := range docs {
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := repo.DuplicateGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	for _, group := range groups {
		if len(group.Documents) != 2 {
			t.Fatalf("group documents = %#v", group)
		}
		if group.Documents[0].UploadedAt.Before(group.Documents[1].UploadedAt) {
			t.Fatalf("documents not ordered newest first: %#v", group.Documents)
		}
	}
	if groups[0].SHA256 != "same-b" {
		t.Fatalf("first group = %#v", groups[0])
	}
}

func TestRepositorySearchMatchesSubstringsWithTrigram(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "farbe.pdf",
		StoredPath:   "2024/03/farbe.pdf",
		Title:        "Farbe",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "trigram-substring",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertSearchFindsDocument(t, repo, "arb", id)
	assertSearchFindsDocument(t, repo, "rbe", id)
	assertSearchFindsDocument(t, repo, "farb", id)
}

func TestRepositorySearchMatchesGermanUmlautTransliterations(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	maerzID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "maerz.pdf",
		StoredPath:   "2024/03/maerz.pdf",
		Title:        "Maerz Bericht",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "umlaut-maerz",
	})
	if err != nil {
		t.Fatal(err)
	}
	umlautID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "märz.pdf",
		StoredPath:   "2024/03/märz.pdf",
		Title:        "März Bericht",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "umlaut-märz",
	})
	if err != nil {
		t.Fatal(err)
	}
	strasseID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "strasse.pdf",
		StoredPath:   "2024/03/strasse.pdf",
		Title:        "Straße",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "umlaut-strasse",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertSearchContainsDocument(t, repo, "März", maerzID)
	assertSearchContainsDocument(t, repo, "Maerz", umlautID)
	assertSearchContainsDocument(t, repo, "ä", maerzID)
	assertSearchContainsDocument(t, repo, "strasse", strasseID)
}

func TestRepositoryMigratesLegacySearchIndexToTrigram(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	repo, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}

	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "farbe.pdf",
		StoredPath:   "2024/03/farbe.pdf",
		Title:        "Farbe",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "legacy-search-index",
	})
	if err != nil {
		t.Fatal(err)
	}
	var originalUpdatedAt string
	if err := repo.db.QueryRowContext(ctx, `SELECT updated_at FROM documents WHERE id = ?`, id).Scan(&originalUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	dsn, err := sqliteDSN(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE document_search`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE document_search USING fts5(original_name, title, description, tags, search_text)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE documents SET search_version = 0 WHERE id = ?`, id); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err = Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	usesTrigram, err := repo.documentSearchUsesTrigram(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !usesTrigram {
		t.Fatal("document_search does not use trigram tokenizer after migration")
	}
	assertSearchFindsDocument(t, repo, "arb", id)

	var searchVersion int
	var updatedAt string
	if err := repo.db.QueryRowContext(ctx, `SELECT search_version, updated_at FROM documents WHERE id = ?`, id).Scan(&searchVersion, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if searchVersion != document.CurrentSearchVersion {
		t.Fatalf("search_version = %d, want %d", searchVersion, document.CurrentSearchVersion)
	}
	if updatedAt != originalUpdatedAt {
		t.Fatalf("updated_at = %q, want preserved %q", updatedAt, originalUpdatedAt)
	}
}

func TestBuildListWhereUsesSingleGroupedTagFilter(t *testing.T) {
	where, args := buildListWhere(document.ListFilter{Tags: []string{"Steuer", "privat", "steuer"}})

	if strings.Count(where, "GROUP BY fdt.document_id") != 1 {
		t.Fatalf("where = %s", where)
	}
	if strings.Count(where, "ft.name IN") != 1 {
		t.Fatalf("where = %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args = %#v", args)
	}
	if args[0] != "steuer" || args[1] != "privat" || args[2] != 2 {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildCountQueryUsesSharedDocumentFilter(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	filter := document.ListFilter{
		Query: "Acme GmbH",
		Tags:  []string{"Steuer", "laufend", "steuer"},
		CustomFields: []document.CustomFieldFilter{
			{FieldID: 7, Value: "Berlin"},
		},
		From: &from,
		To:   &to,
	}

	listWhere, listArgs := buildListWhere(filter)
	countQuery, countArgs := buildCountQuery(filter)
	if !strings.Contains(countQuery, listWhere) {
		t.Fatalf("count query = %s, want shared filter %s", countQuery, listWhere)
	}
	if !reflect.DeepEqual(countArgs, listArgs) {
		t.Fatalf("count args = %#v, want %#v", countArgs, listArgs)
	}
}

func TestRepositoryCountDocumentsWithSearchAndTags(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	matchingID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   "2026/05/rechnung.pdf",
		Title:        "Rechnung",
		Tags:         []string{"steuer", "laufend"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "count-match",
		ContentText:  "acme zahlung",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung-archiv.pdf",
		StoredPath:   "2026/05/rechnung-archiv.pdf",
		Title:        "Rechnung Archiv",
		Tags:         []string{"steuer", "archiv"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "count-archive",
		ContentText:  "acme zahlung",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "notiz.pdf",
		StoredPath:   "2026/05/notiz.pdf",
		Title:        "Notiz",
		Tags:         []string{"steuer", "laufend"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "count-note",
		ContentText:  "anderer text",
	}); err != nil {
		t.Fatal(err)
	}

	total, err := repo.CountDocuments(ctx, document.ListFilter{
		Query: "acme",
		Tags:  []string{"steuer", "laufend"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}

	if err := repo.SoftDelete(ctx, matchingID); err != nil {
		t.Fatal(err)
	}
	active, err := repo.CountDocuments(ctx, document.ListFilter{
		Query: "acme",
		Tags:  []string{"steuer", "laufend"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active total = %d, want 0", active)
	}
	trash, err := repo.CountDocuments(ctx, document.ListFilter{
		Query: "acme",
		Tags:  []string{"steuer", "laufend"},
		Trash: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if trash != 1 {
		t.Fatalf("trash total = %d, want 1", trash)
	}
	counts, err := repo.CountDocumentFilters(ctx, []document.ListFilter{
		{Query: "acme", Tags: []string{"steuer", "laufend"}},
		{Query: "acme", Tags: []string{"steuer", "laufend"}, Trash: true},
		{Tags: []string{"steuer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := []int{0, 1, 2}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatalf("batch counts = %#v, want %#v", counts, wantCounts)
	}
}

func TestRepositoryFiltersByDocumentDateRange(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	date2024 := time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC)
	date2025 := time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)
	matchingID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "matching.pdf",
		StoredPath:   "matching.pdf",
		Title:        "Matching",
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "date-range-match",
		DocumentDate: &date2024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "newer.pdf",
		StoredPath:   "newer.pdf",
		Title:        "Newer",
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "date-range-newer",
		DocumentDate: &date2025,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "missing-date.pdf",
		StoredPath:   "missing-date.pdf",
		Title:        "Missing",
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "date-range-missing",
	}); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	filter := document.ListFilter{From: &from, To: &to}
	docs, err := repo.ListDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != matchingID {
		t.Fatalf("docs = %#v", docs)
	}
	total, err := repo.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

func TestRepositoryListFolderTags(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	date2024 := time.Date(2024, time.February, 12, 0, 0, 0, 0, time.UTC)
	date2023 := time.Date(2023, time.March, 7, 0, 0, 0, 0, time.UTC)
	for _, doc := range []document.Document{
		{OriginalName: "steuer-privat.pdf", StoredPath: "a.pdf", Title: "Steuer privat", Tags: []string{"steuer", "privat"}, MIMEType: "application/pdf", SizeBytes: 1, SHA256: "folder-a", DocumentDate: &date2024},
		{OriginalName: "steuer-bank.pdf", StoredPath: "b.pdf", Title: "Steuer Bank", Tags: []string{"steuer", "bank"}, MIMEType: "application/pdf", SizeBytes: 1, SHA256: "folder-b", DocumentDate: &date2023},
		{OriginalName: "privat-bank.pdf", StoredPath: "c.pdf", Title: "Privat Bank", Tags: []string{"privat", "bank"}, MIMEType: "application/pdf", SizeBytes: 1, SHA256: "folder-c", DocumentDate: &date2024},
	} {
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := repo.ListFolderTags(ctx, document.ListFilter{Tags: []string{"steuer"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagCounts(tags); got["bank"] != 1 || got["privat"] != 1 || got["steuer"] != 0 || len(got) != 2 {
		t.Fatalf("tags after steuer = %#v", got)
	}

	tags, err = repo.ListFolderTags(ctx, document.ListFilter{Tags: []string{"steuer", "privat"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagCounts(tags); len(got) != 0 {
		t.Fatalf("tags after steuer + privat = %#v", got)
	}

	from := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.December, 31, 0, 0, 0, 0, time.UTC)
	tags, err = repo.ListFolderTags(ctx, document.ListFilter{Tags: []string{"steuer"}, From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagCounts(tags); got["privat"] != 1 || got["bank"] != 0 || len(got) != 1 {
		t.Fatalf("tags after steuer + 2024 = %#v", got)
	}
}

func TestRepositoryListFolderCustomFieldValuesUsesThresholdsAndExactSelections(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Status", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false, 5); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Intern", false, document.CustomFieldValueFolderNever); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fieldID := map[string]int64{}
	for _, field := range fields {
		fieldID[field.Label] = field.ID
	}

	for i, item := range []struct {
		name   string
		tags   []string
		values map[int64]string
	}{
		{"a.pdf", []string{"steuer", "privat"}, map[int64]string{fieldID["Kunde"]: "ACME", fieldID["Status"]: "Offen", fieldID["Projekt"]: "Alpha", fieldID["Intern"]: "Ja"}},
		{"b.pdf", []string{"steuer", "bank"}, map[int64]string{fieldID["Kunde"]: "ACME", fieldID["Status"]: "Offen", fieldID["Projekt"]: "Alpha"}},
		{"c.pdf", []string{"steuer", "bank"}, map[int64]string{fieldID["Kunde"]: "Beta", fieldID["Status"]: "Bezahlt", fieldID["Projekt"]: "Beta"}},
		{"d.pdf", []string{"archiv"}, map[int64]string{fieldID["Kunde"]: "ACME GmbH", fieldID["Status"]: "Offen", fieldID["Projekt"]: "Alpha"}},
	} {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			Tags:         item.tags,
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("folder-custom-value-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, item.tags, item.values); err != nil {
			t.Fatal(err)
		}
	}

	values, err := repo.ListFolderCustomFieldValues(ctx, document.ListFilter{Tags: []string{"steuer"}})
	if err != nil {
		t.Fatal(err)
	}
	counts := customFieldValueFolderCounts(values)
	if counts["Kunde\x00ACME"] != 2 || counts["Kunde\x00Beta"] != 1 || counts["Status\x00Offen"] != 2 || counts["Status\x00Bezahlt"] != 1 {
		t.Fatalf("custom value folders after steuer = %#v", counts)
	}
	if counts["Projekt\x00Alpha"] != 0 || counts["Intern\x00Ja"] != 0 || len(counts) != 4 {
		t.Fatalf("threshold/never folders leaked: %#v", counts)
	}

	values, err = repo.ListFolderCustomFieldValues(ctx, document.ListFilter{
		Tags: []string{"steuer"},
		CustomFields: []document.CustomFieldFilter{{
			FieldID: fieldID["Kunde"],
			Value:   "ACME",
			Exact:   true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	counts = customFieldValueFolderCounts(values)
	if counts["Kunde\x00ACME"] != 0 || counts["Status\x00Offen"] != 2 || len(counts) != 1 {
		t.Fatalf("exact selected field was not excluded: %#v", counts)
	}

	total, err := repo.CountDocuments(ctx, document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: fieldID["Kunde"], Value: "ACME"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("like custom field filter total = %d, want 3", total)
	}
	total, err = repo.CountDocuments(ctx, document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: fieldID["Kunde"], Value: "ACME", Exact: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("exact custom field filter total = %d, want 2", total)
	}
}

func TestRepositoryBulkTagChangesSkipTrashedDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	activeID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "active.pdf",
		StoredPath:   "active.pdf",
		Title:        "Active",
		Tags:         []string{"old"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "bulk-tags-active",
	})
	if err != nil {
		t.Fatal(err)
	}
	trashID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "trash.pdf",
		StoredPath:   "trash.pdf",
		Title:        "Trash",
		Tags:         []string{"old"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "bulk-tags-trash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashID); err != nil {
		t.Fatal(err)
	}

	changed, err := repo.AddTagsToDocuments(ctx, []int64{activeID, trashID}, []string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("add changed = %d, want 1", changed)
	}
	active, err := repo.GetDocument(ctx, activeID)
	if err != nil {
		t.Fatal(err)
	}
	trash, err := repo.GetDocument(ctx, trashID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTag(active.Tags, "new") || containsTag(trash.Tags, "new") {
		t.Fatalf("tags after add active=%#v trash=%#v", active.Tags, trash.Tags)
	}

	changed, err = repo.RemoveTagsFromDocuments(ctx, []int64{activeID, trashID}, []string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("remove changed = %d, want 1", changed)
	}
	active, err = repo.GetDocument(ctx, activeID)
	if err != nil {
		t.Fatal(err)
	}
	trash, err = repo.GetDocument(ctx, trashID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(active.Tags, "old") || !containsTag(trash.Tags, "old") {
		t.Fatalf("tags after remove active=%#v trash=%#v", active.Tags, trash.Tags)
	}
}

func tagCounts(tags []document.Tag) map[string]int {
	counts := make(map[string]int, len(tags))
	for _, tag := range tags {
		counts[tag.Name] = tag.Count
	}
	return counts
}

func customFieldValueFolderCounts(values []document.CustomFieldValueFolder) map[string]int {
	counts := make(map[string]int, len(values))
	for _, value := range values {
		counts[value.FieldLabel+"\x00"+value.Value] = value.Count
	}
	return counts
}

func TestRepositoryCustomFieldsAreSearchable(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kundennummer", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %#v", fields)
	}

	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "vertrag.pdf",
		StoredPath:   "2024/01/vertrag.pdf",
		Title:        "Vertrag",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "custom",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, id, "Vertrag", "notiz", nil, []string{"kunde"}, map[int64]string{fields[0].ID: "ACME-4711"}); err != nil {
		t.Fatal(err)
	}

	found, err := repo.ListDocuments(ctx, document.ListFilter{Query: "ACME-4711"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != id {
		t.Fatalf("found = %#v", found)
	}
	if found[0].CustomValues[fields[0].ID] != "ACME-4711" {
		t.Fatalf("custom values = %#v", found[0].CustomValues)
	}

	if err := repo.DeleteCustomField(ctx, fields[0].ID); err != nil {
		t.Fatal(err)
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 0 {
		t.Fatalf("fields after delete = %#v", fields)
	}
	found, err = repo.ListDocuments(ctx, document.ListFilter{Query: "ACME-4711"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("deleted custom field value is still searchable: %#v", found)
	}
	updated, err := repo.GetDocument(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.CustomValues) != 0 {
		t.Fatalf("custom values after delete = %#v", updated.CustomValues)
	}
}

func TestRepositoryFiltersByCustomFieldValues(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", false); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields = %#v", fields)
	}

	values := []struct {
		name    string
		sha     string
		kunde   string
		projekt string
	}{
		{name: "acme-umbau.pdf", sha: "custom-filter-1", kunde: "ACME-4711", projekt: "Umbau"},
		{name: "acme-archiv.pdf", sha: "custom-filter-2", kunde: "ACME-9999", projekt: "Archiv"},
		{name: "beta-umbau.pdf", sha: "custom-filter-3", kunde: "Beta", projekt: "Umbau"},
	}
	ids := make(map[string]int64, len(values))
	for _, item := range values {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			MIMEType:     "application/pdf",
			SizeBytes:    10,
			SHA256:       item.sha,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[item.name] = id
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, nil, map[int64]string{
			fields[0].ID: item.kunde,
			fields[1].ID: item.projekt,
		}); err != nil {
			t.Fatal(err)
		}
	}

	acmeFilter := document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: fields[0].ID, Value: "acme"}},
	}
	found, err := repo.ListDocuments(ctx, acmeFilter)
	if err != nil {
		t.Fatal(err)
	}
	if got := documentIDSet(found); !got[ids["acme-umbau.pdf"]] || !got[ids["acme-archiv.pdf"]] || len(got) != 2 {
		t.Fatalf("acme filter = %#v", found)
	}
	total, err := repo.CountDocuments(ctx, acmeFilter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("acme count = %d, want 2", total)
	}

	combinedFilter := document.ListFilter{
		CustomFields: []document.CustomFieldFilter{
			{FieldID: fields[0].ID, Value: "ACME"},
			{FieldID: fields[1].ID, Value: "umb"},
		},
	}
	found, err = repo.ListDocuments(ctx, combinedFilter)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != ids["acme-umbau.pdf"] {
		t.Fatalf("combined filter = %#v", found)
	}
}

func documentIDSet(docs []document.Document) map[int64]bool {
	ids := make(map[int64]bool, len(docs))
	for _, doc := range docs {
		ids[doc.ID] = true
	}
	return ids
}

func TestRepositoryUpdateCustomField(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kundennummer", false); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateCustomField(ctx, fields[0].ID, " Kunden-ID ", true); err != nil {
		t.Fatal(err)
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fields[0].Label != "Kunden-ID" || !fields[0].AutocompleteEnabled {
		t.Fatalf("updated field = %#v", fields[0])
	}
	if err := repo.UpdateCustomField(ctx, fields[0].ID, "Projekt", true); !errors.Is(err, ErrCustomFieldLabelExists) {
		t.Fatalf("duplicate update err = %v", err)
	}
}

func TestRepositoryCustomFieldValueSuggestions(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || !fields[0].AutocompleteEnabled || fields[1].AutocompleteEnabled {
		t.Fatalf("fields = %#v", fields)
	}

	docValues := []struct {
		name  string
		value string
		trash bool
	}{
		{name: "a.pdf", value: "ACME"},
		{name: "b.pdf", value: "ACME"},
		{name: "c.pdf", value: "Alpha"},
		{name: "d.pdf", value: "Beta"},
		{name: "trash.pdf", value: "Papierkorb", trash: true},
	}
	for i, item := range docValues {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			MIMEType:     "application/pdf",
			SizeBytes:    10,
			SHA256:       fmt.Sprintf("suggestion-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, nil, map[int64]string{fields[0].ID: item.value}); err != nil {
			t.Fatal(err)
		}
		if item.trash {
			if err := repo.SoftDelete(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
	}

	values, err := repo.CustomFieldValueSuggestions(ctx, fields[0].ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(values, ","), "ACME,Alpha,Beta"; got != want {
		t.Fatalf("suggestions = %q, want %q", got, want)
	}
	values, err = repo.CustomFieldValueSuggestions(ctx, fields[0].ID, "bet", 20)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(values, ","), "Beta"; got != want {
		t.Fatalf("filtered suggestions = %q, want %q", got, want)
	}
	values, err = repo.CustomFieldValueSuggestions(ctx, fields[1].ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("disabled field suggestions = %#v", values)
	}
	if err := repo.UpdateCustomFieldAutocomplete(ctx, fields[1].ID, true); err != nil {
		t.Fatal(err)
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !fields[1].AutocompleteEnabled {
		t.Fatalf("autocomplete toggle was not saved: %#v", fields)
	}
}

func TestRepositoryCustomFieldValuesCanBeRenamed(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fieldID := fields[0].ID
	for i, value := range []string{"ACME GmbH", "ACME GmbH", "Acme GmbH"} {
		docID, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: fmt.Sprintf("rename-%d.pdf", i),
			StoredPath:   fmt.Sprintf("rename-%d.pdf", i),
			Title:        fmt.Sprintf("Dokument %d", i),
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("rename-field-value-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, docID, fmt.Sprintf("Dokument %d", i), "", nil, nil, map[int64]string{fieldID: value}); err != nil {
			t.Fatal(err)
		}
	}

	values, err := repo.CustomFieldValues(ctx, fieldID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].Value != "ACME GmbH" || values[0].Count != 2 || values[1].Value != "Acme GmbH" || values[1].Count != 1 {
		t.Fatalf("values before rename = %#v", values)
	}
	updated, err := repo.RenameCustomFieldValue(ctx, fieldID, " ACME  GmbH ", "ACME AG")
	if err != nil {
		t.Fatal(err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	values, err = repo.CustomFieldValues(ctx, fieldID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].Value != "ACME AG" || values[0].Count != 2 || values[1].Value != "Acme GmbH" || values[1].Count != 1 {
		t.Fatalf("values after rename = %#v", values)
	}
	found, err := repo.ListDocuments(ctx, document.ListFilter{CustomFields: []document.CustomFieldFilter{{FieldID: fieldID, Value: "ACME AG"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("filtered renamed docs = %#v", found)
	}
}

func TestRepositoryListDocumentsSortsByUploadDate(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	olderUpload := time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC)
	newerUpload := time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC)
	olderDocumentDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	newerDocumentDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	olderID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "older-upload.pdf",
		StoredPath:   "2024/01/older-upload.pdf",
		Title:        "Older Upload",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "older-upload",
		DocumentDate: &olderDocumentDate,
		UploadedAt:   olderUpload,
	})
	if err != nil {
		t.Fatal(err)
	}
	newerID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "newer-upload.pdf",
		StoredPath:   "2024/01/newer-upload.pdf",
		Title:        "Newer Upload",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "newer-upload",
		DocumentDate: &newerDocumentDate,
		UploadedAt:   newerUpload,
	})
	if err != nil {
		t.Fatal(err)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{Sort: "upload_date"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].ID != newerID || docs[1].ID != olderID {
		t.Fatalf("docs = %#v", docs)
	}

	docs, err = repo.ListDocuments(ctx, document.ListFilter{Sort: "upload_date", Direction: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].ID != olderID || docs[1].ID != newerID {
		t.Fatalf("ascending docs = %#v", docs)
	}
}

func TestRepositoryOCRJobLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "scan.pdf",
		StoredPath:   "2024/01/scan.pdf",
		Title:        "Scan",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "ocr-job",
	})
	if err != nil {
		t.Fatal(err)
	}
	job, created, err := repo.EnqueueOCRJob(ctx, docID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if !created || job.Status != document.OCRJobStatusQueued {
		t.Fatalf("created=%v job=%#v", created, job)
	}
	duplicate, created, err := repo.EnqueueOCRJob(ctx, docID, "eng", "eng")
	if err != nil {
		t.Fatal(err)
	}
	if created || duplicate.ID != job.ID {
		t.Fatalf("duplicate created=%v job=%#v", created, duplicate)
	}
	if err := repo.StartOCRJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateOCRJobProgressMessage(ctx, job.ID, 2, 5, "Seite 2 von 5 wird gelesen."); err != nil {
		t.Fatal(err)
	}
	latest, ok, err := repo.LatestOCRJobForDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || latest.Status != document.OCRJobStatusRunning || latest.CurrentPage != 2 || latest.TotalPages != 5 {
		t.Fatalf("latest ok=%v job=%#v", ok, latest)
	}
	if latest.Message != "Seite 2 von 5 wird gelesen." {
		t.Fatalf("message = %q", latest.Message)
	}
	if err := repo.CompleteOCRJob(ctx, job.ID, 123); err != nil {
		t.Fatal(err)
	}
	next, created, err := repo.EnqueueOCRJob(ctx, docID, "eng", "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !created || next.ID == job.ID {
		t.Fatalf("next created=%v job=%#v", created, next)
	}
}

func TestRepositoryLatestRelevantOCRJobsForDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	failedDocID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "failed.pdf",
		StoredPath:   "2024/01/failed.pdf",
		Title:        "Failed",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "ocr-failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	completedDocID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "completed.pdf",
		StoredPath:   "2024/01/completed.pdf",
		Title:        "Completed",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "ocr-completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	queuedDocID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "queued.pdf",
		StoredPath:   "2024/01/queued.pdf",
		Title:        "Queued",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "ocr-queued",
	})
	if err != nil {
		t.Fatal(err)
	}

	failedJob, _, err := repo.EnqueueOCRJob(ctx, failedDocID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FailOCRJob(ctx, failedJob.ID, "kaputt"); err != nil {
		t.Fatal(err)
	}

	oldFailedJob, _, err := repo.EnqueueOCRJob(ctx, completedDocID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FailOCRJob(ctx, oldFailedJob.ID, "kaputt"); err != nil {
		t.Fatal(err)
	}
	completedJob, _, err := repo.EnqueueOCRJob(ctx, completedDocID, "eng", "eng")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteOCRJob(ctx, completedJob.ID, 12); err != nil {
		t.Fatal(err)
	}

	queuedJob, _, err := repo.EnqueueOCRJob(ctx, queuedDocID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}

	jobs, err := repo.LatestRelevantOCRJobsForDocuments(ctx, []int64{failedDocID, completedDocID, queuedDocID})
	if err != nil {
		t.Fatal(err)
	}
	if got := jobs[failedDocID]; got.ID != failedJob.ID || got.Status != document.OCRJobStatusFailed {
		t.Fatalf("failed job = %#v", got)
	}
	if _, ok := jobs[completedDocID]; ok {
		t.Fatalf("completed doc should not have relevant OCR job: %#v", jobs[completedDocID])
	}
	if got := jobs[queuedDocID]; got.ID != queuedJob.ID || got.Status != document.OCRJobStatusQueued {
		t.Fatalf("queued job = %#v", got)
	}
}

func TestRepositoryPurgeTrash(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	trashedA, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "trash-a.pdf",
		StoredPath:   "2024/01/trash-a.pdf",
		Title:        "Trash A",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "trash-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	trashedB, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "trash-b.pdf",
		StoredPath:   "2024/01/trash-b.pdf",
		Title:        "Trash B",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "trash-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "active.pdf",
		StoredPath:   "2024/01/active.pdf",
		Title:        "Active",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashedA); err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashedB); err != nil {
		t.Fatal(err)
	}

	purged, err := repo.PurgeTrash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 2 {
		t.Fatalf("purged = %#v", purged)
	}
	if _, err := repo.GetDocument(ctx, trashedA); !errorsIsNoRows(err) {
		t.Fatalf("trashedA err = %v", err)
	}
	if _, err := repo.GetDocument(ctx, trashedB); !errorsIsNoRows(err) {
		t.Fatalf("trashedB err = %v", err)
	}
	if _, err := repo.GetDocument(ctx, active); err != nil {
		t.Fatalf("active err = %v", err)
	}
	trash, err := repo.ListDocuments(ctx, document.ListFilter{Trash: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 0 {
		t.Fatalf("trash = %#v", trash)
	}
}

func TestRepositoryDeleteProtectedTagBlocksSoftDelete(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "schutz.pdf",
		StoredPath:   "2024/01/schutz.pdf",
		Title:        "Schutz",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "protected-soft-delete",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Archiv", "", "#176b87", false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, docID, "Schutz", "", nil, []string{"archiv"}, nil); err != nil {
		t.Fatal(err)
	}

	if err := repo.SoftDelete(ctx, docID); !errors.Is(err, ErrDeleteProtected) {
		t.Fatalf("soft delete err = %v", err)
	}
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.DeleteProtected || doc.DeletedAt != nil {
		t.Fatalf("document = %#v", doc)
	}
}

func TestRepositoryPurgeTrashBeforeKeepsRecentAndProtectedDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	oldID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "old.pdf",
		StoredPath:   "2024/01/old.pdf",
		Title:        "Old",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "trash-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	recentID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "recent.pdf",
		StoredPath:   "2024/01/recent.pdf",
		Title:        "Recent",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "trash-recent",
	})
	if err != nil {
		t.Fatal(err)
	}
	protectedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "protected.pdf",
		StoredPath:   "2024/01/protected.pdf",
		Title:        "Protected",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "trash-protected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Archiv", "", "#176b87", false, false, true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{oldID, recentID, protectedID} {
		if err := repo.SoftDelete(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.UpdateMetadata(ctx, protectedID, "Protected", "", nil, []string{"archiv"}, nil); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	oldDeletedAt := formatTime(now.AddDate(0, 0, -40))
	recentDeletedAt := formatTime(now.AddDate(0, 0, -10))
	if _, err := repo.db.ExecContext(ctx, `UPDATE documents SET deleted_at = ? WHERE id IN (?, ?)`, oldDeletedAt, oldID, protectedID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE documents SET deleted_at = ? WHERE id = ?`, recentDeletedAt, recentID); err != nil {
		t.Fatal(err)
	}

	purged, err := repo.PurgeTrashBefore(ctx, now.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0].ID != oldID {
		t.Fatalf("purged = %#v", purged)
	}
	if _, err := repo.GetDocument(ctx, oldID); !errorsIsNoRows(err) {
		t.Fatalf("old err = %v", err)
	}
	if _, err := repo.GetDocument(ctx, recentID); err != nil {
		t.Fatalf("recent err = %v", err)
	}
	protected, err := repo.GetDocument(ctx, protectedID)
	if err != nil {
		t.Fatalf("protected err = %v", err)
	}
	if !protected.DeleteProtected || protected.DeletedAt == nil {
		t.Fatalf("protected document = %#v", protected)
	}
}

func TestRepositoryDocumentLinks(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "first.pdf",
		StoredPath:   "2024/01/first.pdf",
		Title:        "First",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "link-first",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "second.pdf",
		StoredPath:   "2024/01/second.pdf",
		Title:        "Second",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "link-second",
	})
	if err != nil {
		t.Fatal(err)
	}
	thirdID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "third.pdf",
		StoredPath:   "2024/01/third.pdf",
		Title:        "Third",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "link-third",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.LinkDocuments(ctx, []int64{thirdID, firstID, secondID, secondID}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.GetDocument(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.LinkedCount != 2 {
		t.Fatalf("first linked count = %d", first.LinkedCount)
	}
	firstLinks, err := repo.LinkedDocuments(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if !documentIDsEqual(firstLinks, []int64{thirdID, secondID}) {
		t.Fatalf("first links = %#v", firstLinks)
	}
	secondLinks, err := repo.LinkedDocuments(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !documentIDsEqual(secondLinks, []int64{thirdID, firstID}) {
		t.Fatalf("second links = %#v", secondLinks)
	}
	batchLinks, err := repo.LinkedDocumentsForDocuments(ctx, []int64{firstID, secondID})
	if err != nil {
		t.Fatal(err)
	}
	if !documentIDsEqual(batchLinks[firstID], []int64{thirdID, secondID}) {
		t.Fatalf("first batch links = %#v", batchLinks[firstID])
	}
	if !documentIDsEqual(batchLinks[secondID], []int64{thirdID, firstID}) {
		t.Fatalf("second batch links = %#v", batchLinks[secondID])
	}
	if err := repo.UnlinkDocuments(ctx, firstID, secondID); err != nil {
		t.Fatal(err)
	}
	first, err = repo.GetDocument(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.LinkedCount != 1 {
		t.Fatalf("first linked count after unlink = %d", first.LinkedCount)
	}
	firstLinks, err = repo.LinkedDocuments(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if !documentIDsEqual(firstLinks, []int64{thirdID}) {
		t.Fatalf("first links after unlink = %#v", firstLinks)
	}
	if err := repo.UnlinkDocuments(ctx, firstID, secondID); !errorsIsNoRows(err) {
		t.Fatalf("duplicate unlink err = %v", err)
	}
	if err := repo.LinkDocuments(ctx, []int64{firstID}); err == nil {
		t.Fatal("single document link should fail")
	}

	if err := repo.SoftDelete(ctx, thirdID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Purge(ctx, thirdID); err != nil {
		t.Fatal(err)
	}
	firstLinks, err = repo.LinkedDocuments(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstLinks) != 0 {
		t.Fatalf("first links after purge = %#v", firstLinks)
	}
}

func TestRepositoryGroupedDocumentsUseGroupModeTags(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.SaveTag(ctx, "Projekt", "", "#176b87", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Sprint", "", "#2f855a", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Normal", "", "#744210", false, false); err != nil {
		t.Fatal(err)
	}

	firstDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secondDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	thirdDate := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	mainID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "main.pdf",
		StoredPath:   "2026/01/main.pdf",
		Title:        "Main",
		Tags:         []string{"projekt", "sprint", "normal"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "group-main",
		DocumentDate: &firstDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	latestID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "latest.pdf",
		StoredPath:   "2026/01/latest.pdf",
		Title:        "Latest",
		Tags:         []string{"projekt", "sprint"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "group-latest",
		DocumentDate: &thirdDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	olderID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "older.pdf",
		StoredPath:   "2026/01/older.pdf",
		Title:        "Older",
		Tags:         []string{"projekt"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "group-older",
		DocumentDate: &secondDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "normal.pdf",
		StoredPath:   "2026/01/normal.pdf",
		Title:        "Normal",
		Tags:         []string{"normal"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "group-normal",
		DocumentDate: &thirdDate,
	}); err != nil {
		t.Fatal(err)
	}

	grouped, err := repo.GroupedDocuments(ctx, mainID)
	if err != nil {
		t.Fatal(err)
	}
	if !documentIDsEqual(grouped, []int64{latestID, olderID}) {
		t.Fatalf("grouped docs = %#v", grouped)
	}
}

func TestRepositorySettings(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, ok, err := repo.GetSetting(ctx, "document_columns"); err != nil || ok {
		t.Fatalf("initial setting ok=%v err=%v", ok, err)
	}
	if err := repo.SaveSetting(ctx, "document_columns", `["name","title"]`); err != nil {
		t.Fatal(err)
	}
	value, ok, err := repo.GetSetting(ctx, "document_columns")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != `["name","title"]` {
		t.Fatalf("setting ok=%v value=%q", ok, value)
	}
}

func TestRepositoryAuditLogsRotateAndList(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -31)
	recent := now.Add(-time.Hour)
	newer := now.Add(-time.Minute)

	entries := []document.AuditLogEntry{
		{OccurredAt: old, Actor: "admin", Method: "POST", Path: "/fields", Action: "Feld anlegen", Status: 303},
		{OccurredAt: recent, Actor: "admin", Method: "POST", Path: "/tags", Action: "Tag speichern", Status: 303},
		{OccurredAt: newer, Actor: "admin", Method: "POST", Path: "/upload", Action: "Dokumente hochladen", Status: 303},
	}
	for _, entry := range entries {
		if err := repo.SaveAuditLog(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.PruneAuditLogs(ctx, now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %#v", logs)
	}
	if logs[0].Action != "Dokumente hochladen" || logs[1].Action != "Tag speichern" {
		t.Fatalf("logs order = %#v", logs)
	}
}

func TestRepositoryMailImportSettings(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	settings, ok, err := repo.GetMailImportSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ok || settings.Mailbox != "INBOX" || settings.Port != 993 || settings.PollIntervalMinutes != 15 {
		t.Fatalf("defaults ok=%v settings=%#v", ok, settings)
	}

	settings = document.MailImportSettings{
		Enabled:             true,
		Host:                "imap.example.com",
		Port:                143,
		Security:            document.MailImportSecuritySTARTTLS,
		Username:            "importer",
		Password:            "secret",
		Mailbox:             "Invoices",
		PollIntervalMinutes: 5,
		AllowedSenders:      " Billing@Example.com \n\nexample.org\r\n",
	}
	if err := repo.SaveMailImportSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	stored, ok, err := repo.GetMailImportSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || stored.Host != settings.Host || stored.Security != settings.Security || stored.Password != settings.Password || stored.AllowedSenders != "billing@example.com\nexample.org" {
		t.Fatalf("stored ok=%v settings=%#v", ok, stored)
	}
}

func TestRepositoryDocumentDateYears(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	date2024 := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	date2024May := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	date2023 := time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC)
	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "2024-03-15_Rechnung.pdf",
		StoredPath:    "2024/03/2024-03-15_Rechnung.pdf",
		Title:         "Rechnung",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "year-2024",
		DocumentDate:  &date2024,
		SearchVersion: document.CurrentSearchVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "2024-05-20_Quittung.pdf",
		StoredPath:    "2024/05/2024-05-20_Quittung.pdf",
		Title:         "Quittung",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "year-2024-may",
		DocumentDate:  &date2024May,
		SearchVersion: document.CurrentSearchVersion,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "2023-09-01_Vertrag.pdf",
		StoredPath:    "2023/09/2023-09-01_Vertrag.pdf",
		Title:         "Vertrag",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "year-2023",
		DocumentDate:  &date2023,
		SearchVersion: document.CurrentSearchVersion,
	}); err != nil {
		t.Fatal(err)
	}

	years, err := repo.DocumentDateYears(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(years) != 2 || years[0] != 2024 || years[1] != 2023 {
		t.Fatalf("years = %#v", years)
	}
	months, err := repo.DocumentDateMonths(ctx, false, 2024)
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 || months[0] != 3 || months[1] != 5 {
		t.Fatalf("months = %#v", months)
	}

	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	activeYears, err := repo.DocumentDateYears(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeYears) != 2 || activeYears[0] != 2024 || activeYears[1] != 2023 {
		t.Fatalf("active years = %#v", activeYears)
	}
	activeMonths, err := repo.DocumentDateMonths(ctx, false, 2024)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeMonths) != 1 || activeMonths[0] != 5 {
		t.Fatalf("active months = %#v", activeMonths)
	}
	trashYears, err := repo.DocumentDateYears(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashYears) != 1 || trashYears[0] != 2024 {
		t.Fatalf("trash years = %#v", trashYears)
	}
}

func TestRepositoryTagDescriptions(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	tagID, err := repo.SaveTag(ctx, "Steuer", "Unterlagen für die Steuer", "#2f855a", true, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := repo.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("tags = %#v", tags)
	}
	if tags[0].ID != tagID || tags[0].Name != "steuer" || tags[0].Description != "Unterlagen für die Steuer" || tags[0].Color != "#2f855a" || !tags[0].PrimaryTag || !tags[0].GroupMode || !tags[0].ListHidden || tags[0].Count != 0 {
		t.Fatalf("tag = %#v", tags[0])
	}
	found, err := repo.GetTagByName(ctx, "Steuer")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != tagID || found.Name != "steuer" || found.Description != "Unterlagen für die Steuer" || found.Color != "#2f855a" || !found.PrimaryTag || !found.GroupMode || !found.ListHidden {
		t.Fatalf("found tag = %#v", found)
	}
}

func TestRepositoryTagCloudCentralAndPrimaryClusters(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer-a.pdf",
		StoredPath:   "cloud/steuer-a.pdf",
		Title:        "Steuer A",
		Tags:         []string{"steuer", "privat"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "cloud-central-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer-b.pdf",
		StoredPath:   "cloud/steuer-b.pdf",
		Title:        "Steuer B",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "cloud-central-b",
	}); err != nil {
		t.Fatal(err)
	}
	trashID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "privat-trash.pdf",
		StoredPath:   "cloud/privat-trash.pdf",
		Title:        "Privat Trash",
		Tags:         []string{"privat"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "cloud-central-trash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashID); err != nil {
		t.Fatal(err)
	}

	central, err := repo.TagCloud(ctx, 10, 18)
	if err != nil {
		t.Fatal(err)
	}
	if central.HasPrimaryTags || central.MaxCount != 2 || tagCloudItemCount(central.Items, "steuer") != 2 || tagCloudItemCount(central.Items, "privat") != 1 {
		t.Fatalf("central cloud = %#v", central)
	}

	if _, err := repo.SaveTag(ctx, "arbeit", "", "#176b87", false, false, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "privat", "", "#2f855a", false, false, false, true); err != nil {
		t.Fatal(err)
	}
	for i, item := range []struct {
		name string
		tags []string
	}{
		{"arbeit-a.pdf", []string{"arbeit", "kunde", "rechnung"}},
		{"arbeit-b.pdf", []string{"arbeit", "kunde"}},
		{"arbeit-c.pdf", []string{"arbeit", "reise"}},
		{"privat-a.pdf", []string{"privat", "familie", "reise"}},
		{"privat-b.pdf", []string{"privat", "familie"}},
	} {
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   "cloud/primary-" + item.name,
			Title:        item.name,
			Tags:         item.tags,
			MIMEType:     "application/pdf",
			SizeBytes:    10,
			SHA256:       fmt.Sprintf("cloud-primary-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	clustered, err := repo.TagCloud(ctx, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !clustered.HasPrimaryTags || len(clustered.Clusters) != 2 {
		t.Fatalf("clustered cloud = %#v", clustered)
	}
	arbeit := tagCloudCluster(clustered.Clusters, "arbeit")
	if arbeit.Primary.Count != 3 || len(arbeit.Items) != 2 || tagCloudItemCount(arbeit.Items, "kunde") != 2 || tagCloudItemCount(arbeit.Items, "rechnung") != 1 {
		t.Fatalf("arbeit cluster = %#v", arbeit)
	}
	privat := tagCloudCluster(clustered.Clusters, "privat")
	if privat.Primary.Count != 3 || len(privat.Items) != 2 || tagCloudItemCount(privat.Items, "familie") != 2 || tagCloudItemCount(privat.Items, "reise") != 1 {
		t.Fatalf("privat cluster = %#v", privat)
	}
}

func TestRepositoryStatistics(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	now := time.Now().UTC()
	docDate := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	secondDocDate := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "rechnung.pdf",
		StoredPath:        "2024/05/rechnung.pdf",
		Title:             "Rechnung",
		Tags:              []string{"steuer"},
		MIMEType:          "application/pdf",
		SizeBytes:         100,
		SHA256:            "same",
		DocumentDate:      &docDate,
		UploadedAt:        now.AddDate(0, 0, -2),
		ContentText:       "ocr text",
		ContentTextSource: document.ContentTextSourcePDF,
		UploadWay:         document.UploadWayWeb,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "foto.jpg",
		StoredPath:   "2024/05/foto.jpg",
		Title:        "Foto",
		Tags:         []string{"steuer", "privat"},
		MIMEType:     "image/jpeg",
		SizeBytes:    50,
		SHA256:       "same",
		DocumentDate: &secondDocDate,
		UploadedAt:   now.AddDate(0, 0, -1),
		UploadWay:    document.UploadWayMail,
	})
	if err != nil {
		t.Fatal(err)
	}
	runningID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "scan.pdf",
		StoredPath:        "2024/05/scan.pdf",
		Title:             "Scan",
		MIMEType:          "application/pdf",
		SizeBytes:         70,
		SHA256:            "running",
		UploadedAt:        now,
		ContentText:       "raw fallback",
		ContentTextSource: document.ContentTextSourceRaw,
		UploadWay:         document.UploadWayWeb,
	})
	if err != nil {
		t.Fatal(err)
	}
	queuedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "beleg.jpg",
		StoredPath:   "2024/05/beleg.jpg",
		Title:        "Beleg",
		MIMEType:     "image/jpeg",
		SizeBytes:    30,
		SHA256:       "queued",
		UploadedAt:   now,
		UploadWay:    document.UploadWayWeb,
	})
	if err != nil {
		t.Fatal(err)
	}
	trashID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "alt.pdf",
		StoredPath:   "2024/04/alt.pdf",
		Title:        "Alt",
		MIMEType:     "application/pdf",
		SizeBytes:    20,
		SHA256:       "trash",
		UploadedAt:   now.AddDate(0, -2, 0),
		UploadWay:    document.UploadWayAPI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashID); err != nil {
		t.Fatal(err)
	}

	job, _, err := repo.EnqueueOCRJob(ctx, firstID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StartOCRJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteOCRJob(ctx, job.ID, 42); err != nil {
		t.Fatal(err)
	}
	job, _, err = repo.EnqueueOCRJob(ctx, secondID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FailOCRJob(ctx, job.ID, "kaputt"); err != nil {
		t.Fatal(err)
	}
	job, _, err = repo.EnqueueOCRJob(ctx, runningID, "deu", "de")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StartOCRJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateOCRJobProgressMessage(ctx, job.ID, 2, 5, "Seite 2 von 5 wird gelesen."); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.EnqueueOCRJob(ctx, queuedID, "eng", "eng"); err != nil {
		t.Fatal(err)
	}

	stats, err := repo.Statistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ActiveDocuments != 4 || stats.TrashDocuments != 1 || stats.TotalDocuments != 5 {
		t.Fatalf("document counts = %#v", stats)
	}
	if stats.TotalBytes != 250 || stats.TrashBytes != 20 || stats.AverageBytes != 62 {
		t.Fatalf("bytes = %#v", stats)
	}
	if stats.UploadedLast30Days != 4 || stats.OCRCoveragePercent != 50 || stats.DocumentDateCoveragePercent != 50 {
		t.Fatalf("coverage = %#v", stats)
	}
	if stats.DuplicateGroups != 1 || stats.DuplicateDocuments != 2 {
		t.Fatalf("duplicates = %#v", stats)
	}
	if len(stats.MonthlyUploads) != 12 || stats.MonthlyUploadsMax < 1 {
		t.Fatalf("monthly uploads = %#v", stats.MonthlyUploads)
	}
	if countBucket(stats.DocumentDateYears, "2024") != 1 || countBucket(stats.DocumentDateYears, "2025") != 1 || countBucket(stats.DocumentDateYears, "missing") != 2 {
		t.Fatalf("document date years = %#v", stats.DocumentDateYears)
	}
	if countBucket(stats.UploadWays, "web") != 3 || countBucket(stats.UploadWays, "mail") != 1 {
		t.Fatalf("upload ways = %#v", stats.UploadWays)
	}
	if countBucket(stats.FileTypes, "pdf") != 2 || countBucket(stats.FileTypes, "image") != 2 {
		t.Fatalf("file types = %#v", stats.FileTypes)
	}
	if countBucket(stats.TopTags, "steuer") != 2 {
		t.Fatalf("top tags = %#v", stats.TopTags)
	}
	if len(stats.TagUsageYears) != 2 || stats.TagUsageYearMax != 2 {
		t.Fatalf("tag usage years = %#v", stats.TagUsageYears)
	}
	if tagUsageSegmentCount(stats.TagUsageYears, "2024", "steuer") != 1 ||
		tagUsageSegmentCount(stats.TagUsageYears, "2025", "steuer") != 1 ||
		tagUsageSegmentCount(stats.TagUsageYears, "2025", "privat") != 1 {
		t.Fatalf("tag usage segments = %#v", stats.TagUsageYears)
	}
	if countBucket(stats.OCRStatuses, document.OCRJobStatusCompleted) != 1 ||
		countBucket(stats.OCRStatuses, document.OCRJobStatusFailed) != 1 ||
		countBucket(stats.OCRStatuses, document.OCRJobStatusRunning) != 1 ||
		countBucket(stats.OCRStatuses, document.OCRJobStatusQueued) != 1 {
		t.Fatalf("ocr statuses = %#v", stats.OCRStatuses)
	}
	if countBucket(stats.ContentTextSources, document.ContentTextSourcePDF) != 1 ||
		countBucket(stats.ContentTextSources, document.ContentTextSourceRaw) != 1 ||
		countBucket(stats.ContentTextSources, document.ContentTextSourceNone) != 2 {
		t.Fatalf("content text sources = %#v", stats.ContentTextSources)
	}
	if stats.TextIssueDocumentCount != 3 || len(stats.TextIssueDocuments) != 3 {
		t.Fatalf("text issue documents = %d %#v", stats.TextIssueDocumentCount, stats.TextIssueDocuments)
	}
	if stats.TextIssueDocuments[0].ContentTextSource != document.ContentTextSourceRaw || stats.TextIssueDocuments[0].OriginalName != "scan.pdf" {
		t.Fatalf("raw text issue = %#v", stats.TextIssueDocuments[0])
	}
	candidates, err := repo.ProblemContentOCRCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || candidates[0].ID != runningID {
		t.Fatalf("problem content ocr candidates = %#v", candidates)
	}
	if len(stats.OCRAttentionJobs) != 3 {
		t.Fatalf("ocr attention jobs = %#v", stats.OCRAttentionJobs)
	}
	if stats.OCRAttentionJobs[0].Status != document.OCRJobStatusRunning || stats.OCRAttentionJobs[0].DocumentOriginalName != "scan.pdf" || stats.OCRAttentionJobs[0].CurrentPage != 2 || stats.OCRAttentionJobs[0].TotalPages != 5 {
		t.Fatalf("running attention job = %#v", stats.OCRAttentionJobs[0])
	}
	if stats.OCRAttentionJobs[1].Status != document.OCRJobStatusQueued || stats.OCRAttentionJobs[1].DocumentOriginalName != "beleg.jpg" {
		t.Fatalf("queued attention job = %#v", stats.OCRAttentionJobs[1])
	}
	if stats.OCRAttentionJobs[2].Status != document.OCRJobStatusFailed || stats.OCRAttentionJobs[2].DocumentOriginalName != "foto.jpg" || stats.OCRAttentionJobs[2].Error != "kaputt" {
		t.Fatalf("failed attention job = %#v", stats.OCRAttentionJobs[2])
	}
	if !stats.Database.UpToDate() ||
		stats.Database.TargetSearchVersion != document.CurrentSearchVersion ||
		stats.Database.TotalDocuments != 5 ||
		stats.Database.CurrentSearchVersionDocuments != 5 ||
		stats.Database.OutdatedSearchVersionDocuments != 0 ||
		stats.Database.SearchIndexDocuments != 5 ||
		!stats.Database.SearchIndexTrigram {
		t.Fatalf("database status = %#v", stats.Database)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE documents SET search_version = ? WHERE id = ?`, document.CurrentSearchVersion-1, firstID); err != nil {
		t.Fatal(err)
	}
	stats, err = repo.Statistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Database.UpToDate() ||
		stats.Database.CurrentSearchVersionDocuments != 4 ||
		stats.Database.OutdatedSearchVersionDocuments != 1 ||
		stats.Database.MinSearchVersion != document.CurrentSearchVersion-1 ||
		stats.Database.MaxSearchVersion != document.CurrentSearchVersion {
		t.Fatalf("outdated database status = %#v", stats.Database)
	}
}

func countBucket(buckets []document.StatisticBucket, key string) int {
	for _, bucket := range buckets {
		if bucket.Key == key {
			return bucket.Count
		}
	}
	return 0
}

func tagUsageSegmentCount(years []document.TagUsageYear, year, tag string) int {
	for _, item := range years {
		if item.Year != year {
			continue
		}
		for _, segment := range item.Segments {
			if segment.Tag == tag {
				return segment.Count
			}
		}
	}
	return 0
}

func tagCloudItemCount(items []document.TagCloudItem, tag string) int {
	for _, item := range items {
		if item.Tag == tag {
			return item.Count
		}
	}
	return 0
}

func tagCloudCluster(clusters []document.TagCloudCluster, tag string) document.TagCloudCluster {
	for _, cluster := range clusters {
		if cluster.Primary.Tag == tag {
			return cluster
		}
	}
	return document.TagCloudCluster{}
}

func TestRepositoryDeleteTagRemovesAssignmentsRulesAndSearchIndex(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "beleg.pdf",
		StoredPath:   "2024/01/beleg.pdf",
		Title:        "Beleg",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "delete-tag",
	})
	if err != nil {
		t.Fatal(err)
	}
	tagID, err := repo.SaveTag(ctx, "Steuer", "Unterlagen", "#2f855a", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, docID, "Beleg", "", nil, []string{"steuer"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTagRules(ctx, tagID, []document.TagRule{{
		Label:     "Steuer",
		Scope:     document.TagRuleScopeBoth,
		MatchMode: document.TagRuleMatchAny,
		Keywords:  []string{"steuer"},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	found, err := repo.ListDocuments(ctx, document.ListFilter{Query: "steuer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != docID {
		t.Fatalf("found before delete = %#v", found)
	}

	deleted, err := repo.DeleteTag(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Name != "steuer" {
		t.Fatalf("deleted = %#v", deleted)
	}
	if _, err := repo.GetTag(ctx, tagID); !errorsIsNoRows(err) {
		t.Fatalf("tag err = %v", err)
	}
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(doc.Tags, "steuer") {
		t.Fatalf("tags after delete = %#v", doc.Tags)
	}
	found, err = repo.ListDocuments(ctx, document.ListFilter{Tags: []string{"steuer"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("tag filter found = %#v", found)
	}
	found, err = repo.ListDocuments(ctx, document.ListFilter{Query: "steuer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("search found = %#v", found)
	}
	rules, err := repo.ListTagRules(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestRepositoryAutoTagRulesApplyOnlyWhenDocumentIsCreated(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	existingID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "European-before-rule.pdf",
		StoredPath:   "2024/01/european-before-rule.pdf",
		Title:        "Before Rule",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "auto-existing",
		ContentText:  "The European Sustainable Products Regulation applies.",
	})
	if err != nil {
		t.Fatal(err)
	}

	tagID, err := repo.SaveTag(ctx, "ESPR", "EU-Regelwerk", "#176b87", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTagRules(ctx, tagID, []document.TagRule{{
		Label:     "EU-Bezug",
		Scope:     document.TagRuleScopeBoth,
		MatchMode: document.TagRuleMatchAny,
		Keywords:  []string{"european"},
		Excludes:  []string{"draft"},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	rules, err := repo.ListTagRules(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || len(rules[0].Excludes) != 1 || rules[0].Excludes[0] != "draft" {
		t.Fatalf("rules = %#v", rules)
	}

	existing, err := repo.GetDocument(ctx, existingID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(existing.Tags, "espr") {
		t.Fatalf("existing tags = %#v", existing.Tags)
	}

	newID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "European-market.pdf",
		StoredPath:   "2024/01/european-market.pdf",
		Title:        "Market",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "auto-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := repo.GetDocument(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTag(created.Tags, "espr") {
		t.Fatalf("new tags = %#v", created.Tags)
	}
	excludedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "European-draft.pdf",
		StoredPath:   "2024/01/european-draft.pdf",
		Title:        "Draft",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "auto-excluded",
	})
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := repo.GetDocument(ctx, excludedID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(excluded.Tags, "espr") {
		t.Fatalf("excluded tags = %#v", excluded.Tags)
	}
	if err := repo.UpdateMetadata(ctx, newID, "Market", "", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	created, err = repo.GetDocument(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(created.Tags, "espr") {
		t.Fatalf("new tags after manual removal = %#v", created.Tags)
	}

	if err := repo.UpdateSearchText(ctx, existingID, "The European Sustainable Products Regulation applies.", document.ContentTextSourceOCR, document.CurrentSearchVersion); err != nil {
		t.Fatal(err)
	}
	existing, err = repo.GetDocument(ctx, existingID)
	if err != nil {
		t.Fatal(err)
	}
	if containsTag(existing.Tags, "espr") {
		t.Fatalf("existing tags after reindex = %#v", existing.Tags)
	}

	found, err := repo.ListDocuments(ctx, document.ListFilter{Tags: []string{"espr"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %#v", found)
	}
}

func TestRepositoryFindActiveByChecksumIgnoresTrash(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	id, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "doc.pdf",
		StoredPath:   "2024/01/doc.pdf",
		Title:        "doc",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.FindActiveByChecksum(ctx, "same"); err != nil || !ok {
		t.Fatalf("find active ok=%v err=%v", ok, err)
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.FindActiveByChecksum(ctx, "same"); err != nil || ok {
		t.Fatalf("find trashed ok=%v err=%v", ok, err)
	}
}

func TestRepositorySearchFavoritesCRUD(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %#v", fields)
	}

	id, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name:  " Rechnungen aktuell ",
		Query: " rechnung  2026 ",
		Tags:  []string{"Steuer", "steuer", " offen "},
		CustomFields: []document.CustomFieldFilter{
			{FieldID: fields[0].ID, Value: " ACME  4711 "},
			{FieldID: 0, Value: "ignoriert"},
		},
		DateMode: document.SearchFavoriteDateLast30Days,
	})
	if err != nil {
		t.Fatal(err)
	}
	favorites, err := repo.ListSearchFavorites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 1 {
		t.Fatalf("favorites = %#v", favorites)
	}
	got := favorites[0]
	if got.ID != id || got.Name != "Rechnungen aktuell" || got.Query != "rechnung 2026" {
		t.Fatalf("favorite = %#v", got)
	}
	if !containsString(got.Tags, "steuer") || !containsString(got.Tags, "offen") || len(got.Tags) != 2 {
		t.Fatalf("tags = %#v", got.Tags)
	}
	if len(got.CustomFields) != 1 || got.CustomFields[0].FieldID != fields[0].ID || got.CustomFields[0].Value != "ACME 4711" {
		t.Fatalf("custom fields = %#v", got.CustomFields)
	}
	if got.DateMode != document.SearchFavoriteDateLast30Days || got.DateYear != 0 {
		t.Fatalf("date = %#v", got)
	}

	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name:     "rechnungen aktuell",
		Tags:     []string{"archiv"},
		DateMode: document.SearchFavoriteDateNone,
	}); !errors.Is(err, ErrSearchFavoriteNameExists) {
		t.Fatalf("duplicate err = %v", err)
	}

	if err := repo.UpdateSearchFavorite(ctx, id, document.SearchFavorite{
		Name: "Archiv",
		CustomFields: []document.CustomFieldFilter{
			{FieldID: fields[0].ID, Value: "Archivkunde"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetSearchFavorite(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Archiv" || got.DateMode != document.SearchFavoriteDateNone || got.DateYear != 0 {
		t.Fatalf("updated favorite = %#v", got)
	}
	if len(got.Tags) != 0 || len(got.CustomFields) != 1 || got.CustomFields[0].Value != "Archivkunde" {
		t.Fatalf("updated custom fields = %#v", got)
	}

	deleted, err := repo.DeleteSearchFavorite(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Name != "Archiv" {
		t.Fatalf("deleted = %#v", deleted)
	}
	favorites, err = repo.ListSearchFavorites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 0 {
		t.Fatalf("favorites after delete = %#v", favorites)
	}
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertSearchFindsDocument(t *testing.T, repo *Repository, query string, id int64) {
	t.Helper()
	docs, err := repo.ListDocuments(context.Background(), document.ListFilter{Query: query})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != id {
		t.Fatalf("query %q found %#v, want document %d", query, docs, id)
	}
}

func assertSearchContainsDocument(t *testing.T, repo *Repository, query string, id int64) {
	t.Helper()
	docs, err := repo.ListDocuments(context.Background(), document.ListFilter{Query: query})
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		if doc.ID == id {
			return
		}
	}
	t.Fatalf("query %q found %#v, want document %d", query, docs, id)
}

func documentIDsEqual(docs []document.Document, want []int64) bool {
	if len(docs) != len(want) {
		return false
	}
	for i := range docs {
		if docs[i].ID != want[i] {
			return false
		}
	}
	return true
}
