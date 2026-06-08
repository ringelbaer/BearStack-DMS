// Datei implementiert Workflow-Operationen fuer Dokumentstatus, Fristen und Bearbeitungsschritte.
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"bearstack/internal/document"
)

func documentWithMetadataInput(current document.Document, title, description string, documentDate *time.Time, customValues map[int64]string) document.Document {
	current.Title = strings.TrimSpace(title)
	if current.Title == "" {
		current.Title = current.OriginalName
	}
	current.Description = strings.TrimSpace(description)
	current.DocumentDate = documentDate
	current.CustomValues = cleanCustomValues(customValues)
	current.SearchVersion = document.CurrentSearchVersion
	return current
}

func updateDocumentMetadataTx(ctx context.Context, tx *sql.Tx, doc document.Document) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE documents
		SET title = ?, description = ?, document_date = ?, search_version = ?, updated_at = ?
		WHERE id = ?`,
		doc.Title,
		doc.Description,
		documentDateSQLValue(doc.DocumentDate),
		doc.SearchVersion,
		formatTime(time.Now().UTC()),
		doc.ID,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func documentDateSQLValue(documentDate *time.Time) any {
	if documentDate == nil {
		return nil
	}
	return documentDate.Format("2006-01-02")
}

func ensureTagIDsTx(ctx context.Context, tx *sql.Tx, tags []string) ([]int64, error) {
	tagIDs := make([]int64, 0, len(tags))
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES (?)`, tag); err != nil {
			return nil, err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID); err != nil {
			return nil, err
		}
		tagIDs = append(tagIDs, tagID)
	}
	return tagIDs, nil
}

func activeDocumentIDsForTagAdd(docs map[int64]document.Document, ids []int64, tags []string) []int64 {
	activeIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		doc, ok := docs[id]
		if !ok {
			continue
		}
		if doc.IsDeleted() {
			continue
		}
		activeIDs = append(activeIDs, id)
		doc.Tags = appendMissingTags(doc.Tags, tags)
		docs[id] = doc
	}
	return activeIDs
}

func tagRemovalSet(tags []string) map[string]struct{} {
	remove := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		remove[tag] = struct{}{}
	}
	return remove
}

func changedDocumentIDsForTagRemoval(docs map[int64]document.Document, ids []int64, remove map[string]struct{}) []int64 {
	changedIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		doc, ok := docs[id]
		if !ok {
			continue
		}
		if doc.IsDeleted() {
			continue
		}
		nextTags := make([]string, 0, len(doc.Tags))
		changed := false
		for _, tag := range doc.Tags {
			if _, ok := remove[tag]; ok {
				changed = true
				continue
			}
			nextTags = append(nextTags, tag)
		}
		if !changed {
			continue
		}
		doc.Tags = nextTags
		docs[id] = doc
		changedIDs = append(changedIDs, id)
	}
	return changedIDs
}

func touchAndUpdateSearchTagsTx(ctx context.Context, tx *sql.Tx, docs map[int64]document.Document, ids []int64, updatedAt string) error {
	if err := touchDocumentsTx(ctx, tx, ids, updatedAt); err != nil {
		return err
	}
	for _, id := range ids {
		doc := docs[id]
		if err := updateSearchIndexTagsTx(ctx, tx, id, doc.Tags); err != nil {
			return err
		}
	}
	return nil
}
