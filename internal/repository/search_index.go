// Datei aktualisiert und pflegt den Suchindex fuer Dokumente und zugehoerige Metadaten.
package repository

import (
	"context"
	"database/sql"

	"bearstack/internal/document"
)

func (r *Repository) RebuildSearchIndex(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := rebuildSearchIndexTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func rebuildSearchIndexTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_search`); err != nil {
		return err
	}

	ids, err := allDocumentIDsTx(ctx, tx)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for start := 0; start < len(ids); start += repositoryBatchSize {
		end := start + repositoryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		docs, err := documentsByIDTx(ctx, tx, batch)
		if err != nil {
			return err
		}
		for _, id := range batch {
			doc, ok := docs[id]
			if !ok {
				continue
			}
			doc.SearchVersion = document.CurrentSearchVersion
			result, err := tx.ExecContext(ctx, `
				UPDATE documents
				SET search_version = ?
				WHERE id = ?`, doc.SearchVersion, id)
			if err != nil {
				return err
			}
			if err := requireAffected(result); err != nil {
				return err
			}
			if err := replaceSearchIndex(ctx, tx, id, doc); err != nil {
				return err
			}
		}
	}
	return nil
}

func allDocumentIDsTx(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM documents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
