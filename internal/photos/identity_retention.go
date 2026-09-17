package photos

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const PhotoRetention = 7 * 24 * time.Hour

type photoEntity struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	CachePath      string `json:"-"`
	Fingerprint    string `json:"-"`
	Size           int64  `json:"-"`
	Mtime          int64  `json:"-"`
	Revision       int64  `json:"revision"`
	MissingSince   int64  `json:"missing_since"`
	ManualRevision int64  `json:"-"`
}

const entityColumns = `id,kind,path,cache_path,fingerprint,size_bytes,mtime,revision,missing_since,manual_revision`

func scanEntity(row interface{ Scan(...any) error }) (e photoEntity, err error) {
	err = row.Scan(&e.ID, &e.Kind, &e.Path, &e.CachePath, &e.Fingerprint, &e.Size, &e.Mtime, &e.Revision, &e.MissingSince, &e.ManualRevision)
	return
}

func retainedForKind(spec retainedTable, kind string) bool {
	return spec.kind == kind || (spec.kind == "media" || spec.table == "media_index") && isMediaKind(kind)
}

type retentionSelection struct {
	where string
	args  []any
}

// Snapshots and removal share the caller's transaction. SQLite copies bounded
// entities directly; no embeddings or complete library inventory enter Go RAM.
func (s *photoIndexStore) retainSelectionTx(ctx context.Context, tx *sql.Tx, sel retentionSelection) error {
	var after int64
	for {
		args := append(append([]any{}, sel.args...), after)
		rows, err := tx.QueryContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE missing_since=0 AND (`+sel.where+`) AND id>? ORDER BY id LIMIT 100`, args...)
		if err != nil {
			return err
		}
		var entries []photoEntity
		for rows.Next() {
			e, err := scanEntity(rows)
			if err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			for _, spec := range retainedTables {
				if !retainedForKind(spec, e.Kind) {
					continue
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO photo_retained_`+spec.table+` SELECT ?,t.* FROM `+spec.table+` t WHERE `+spec.key+`=?`, e.ID, e.Path); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE photo_entities SET missing_since=?,revision=revision+1 WHERE id=? AND missing_since=0`, time.Now().Unix(), e.ID); err != nil {
				return err
			}
			after = e.ID
		}
		if len(entries) < 100 {
			return nil
		}
	}
}

func (s *photoIndexStore) restoreEntityTx(ctx context.Context, tx *sql.Tx, e photoEntity) error {
	if e.MissingSince == 0 {
		return nil
	}
	for _, spec := range retainedTables {
		if !retainedForKind(spec, e.Kind) {
			continue
		}
		if spec.table == "photo_faces" {
			var private bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM media_index WHERE path=? AND admin_only=1)`, e.Path).Scan(&private); err != nil {
				return err
			}
			if private {
				continue
			}
		}
		cols := s.retainedColumns[spec.table]
		if cols == "" {
			return fmt.Errorf("missing retained schema: %s", spec.table)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+spec.table+` (`+cols+`) SELECT `+cols+` FROM photo_retained_`+spec.table+` WHERE retention_id=?`, e.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM photo_retained_`+spec.table+` WHERE retention_id=?`, e.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_entities SET missing_since=0,revision=revision+1 WHERE id=?`, e.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_face_thumbnail_cache SET expires_at=CASE WHEN EXISTS(SELECT 1 FROM photo_faces WHERE id=face_id AND ignored=1 AND needs_review=0) THEN unixepoch()+172800 ELSE 0 END WHERE face_id IN(SELECT id FROM photo_faces WHERE entity_id=?)`, e.ID); err != nil {
		return err
	}
	if err := s.refreshEntitySearchTx(ctx, tx, e); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE photo_face_reference_settings SET pending=1,cursor=0; UPDATE photo_face_state SET revision=revision+1; DELETE FROM photo_folder_scan`)
	return err
}

func (s *photoIndexStore) refreshEntitySearchTx(ctx context.Context, tx *sql.Tx, e photoEntity) error {
	switch e.Kind {
	case "image", "video", "audio":
		return refreshMediaSearchTx(ctx, tx, e.Path)
	case "folder":
		return refreshFolderSearchTx(ctx, tx, e.Path)
	case "blog":
		return refreshBlogSearchTx(ctx, tx, e.Path)
	}
	return nil
}

func (l *Library) restorePresentEntities(ctx context.Context) error {
	var after int64
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT `+prefixEntityColumns("e")+` FROM photo_entities e JOIN photo_identity_scan s ON s.path=e.path AND s.kind=e.kind WHERE e.missing_since>0 AND e.id>? ORDER BY e.id LIMIT 100`, after)
		if err != nil {
			return err
		}
		var entries []photoEntity
		for rows.Next() {
			e, err := scanEntity(rows)
			if err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			err = l.index.restoreEntityTx(ctx, tx, e)
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
			if err != nil {
				return err
			}
			after = e.ID
		}
		if len(entries) < 100 {
			return nil
		}
	}
}

func prefixEntityColumns(alias string) string {
	return alias + "." + strings.ReplaceAll(entityColumns, ",", ","+alias+".")
}

func (s *photoIndexStore) purgeEntityTx(ctx context.Context, tx *sql.Tx, e photoEntity) error {
	// Face cache inventory is a persistent deletion journal; the existing worker
	// retries failed unlinks, including after restart.
	if _, err := tx.ExecContext(ctx, `UPDATE photo_face_thumbnail_cache SET expires_at=unixepoch() WHERE face_id IN(SELECT id FROM photo_retained_photo_faces WHERE retention_id=?)`, e.ID); err != nil {
		return err
	}
	for _, spec := range retainedTables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM photo_retained_`+spec.table+` WHERE retention_id=?`, e.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_hidden_person_names WHERE name_source=?`, e.Path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_people SET name='',name_fold='',name_source='' WHERE name_source=? AND manual_name=0`, e.Path); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM photo_entities WHERE id=? AND missing_since>0`, e.ID)
	return err
}
