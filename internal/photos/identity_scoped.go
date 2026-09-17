package photos

import (
	"context"
	"database/sql"
)

// Source/destination subtrees and their ancestors are the only folders whose
// scan signatures, counts or preview selections can change after relocation.
func invalidateIdentityFoldersTx(ctx context.Context, tx *sql.Tx, subtrees bool, paths ...string) error {
	for _, path := range paths {
		if subtrees && path != "" {
			start, end := prefixRange(path + "/")
			for _, spec := range [][2]string{{"photo_folder_scan", "path"}, {"folder_preview_index", "folder_path"}} {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+spec[0]+` WHERE `+spec[1]+`>=? AND `+spec[1]+`<?`, start, end); err != nil {
					return err
				}
			}
		}
		for {
			if _, err := tx.ExecContext(ctx, `DELETE FROM photo_folder_scan WHERE path=?; DELETE FROM folder_preview_index WHERE folder_path=?`, path, path); err != nil {
				return err
			}
			if path == "" {
				break
			}
			path = parentPath(path)
		}
	}
	return nil
}

func refreshSelectedFaceReferencesTx(ctx context.Context, tx *sql.Tx, selection string, args ...any) error {
	var after int64
	for {
		params := append(append([]any{}, args...), after)
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT person_id FROM (`+selection+`) WHERE person_id>? ORDER BY person_id LIMIT 100`, params...)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := refreshFaceReferencesTx(ctx, tx, id); err != nil {
				return err
			}
			after = id
		}
		if len(ids) < 100 {
			return nil
		}
	}
}

// Temporary transaction-local work set includes people whose last active face
// will disappear. It never loads the library's people/embeddings into memory.
func relocationPeopleTx(ctx context.Context, tx *sql.Tx, paths ...string) error {
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE relocation_people(person_id INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	for _, path := range paths {
		start, end := prefixRange(path + "/")
		for _, table := range []string{"photo_faces", "photo_retained_photo_faces"} {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO relocation_people SELECT person_id FROM `+table+` WHERE path>=? AND path<?`, start, end); err != nil {
				return err
			}
		}
	}
	return nil
}
