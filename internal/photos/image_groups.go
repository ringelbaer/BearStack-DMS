package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"

	"bearstack/internal/sqlutil"
)

const MaxImageGroupSize = 500

var ErrImageGroupConflict = errors.New("Die Bildgruppe wurde inzwischen geändert. Bitte die Ansicht neu laden.")
var ErrImageGroupInvalid = errors.New("Bitte 2 bis 500 noch nicht gruppierte Bilder und ein Hauptbild aus dieser Auswahl wählen.")

type ImageGroupMember struct {
	Media       Media
	EntityID    int64
	DisplayPath string
	Missing     bool
}
type ImageGroup struct {
	ID        int64
	Revision  int64
	PrimaryID int64
	Members   []ImageGroupMember
}

func (l *Library) CreateImageGroup(ctx context.Context, paths []string, primary string, admin bool) (int64, error) {
	if l == nil || !l.index.available() {
		return 0, errors.New("Der Fotoindex ist nicht verfügbar.")
	}
	if len(paths) < 2 || len(paths) > MaxImageGroupSize {
		return 0, ErrImageGroupInvalid
	}
	if err := l.lockPhotoIdentities(ctx); err != nil {
		return 0, err
	}
	defer l.identityMu.Unlock()
	items, err := l.MediaBatchContext(ctx, paths)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	primaryID := int64(0)
	for _, m := range items {
		if m.Type != MediaTypeImage || seen[m.Path] {
			return 0, ErrImageGroupInvalid
		}
		seen[m.Path] = true
		if !admin && m.AdminOnly {
			return 0, errAdminOnly
		}
	}
	if !seen[primary] {
		return 0, ErrImageGroupInvalid
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Acquire the SQLite writer before reading membership, also across processes.
	result, err := tx.ExecContext(ctx, `INSERT INTO photo_image_groups(primary_entity_id) VALUES(0)`)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(items))
	for _, m := range items {
		var entity int64
		var grouped bool
		err = tx.QueryRowContext(ctx, `SELECT e.id,EXISTS(SELECT 1 FROM photo_image_group_members gm WHERE gm.entity_id=e.id) FROM photo_entities e JOIN media_index m ON m.path=e.path AND m.type=e.kind WHERE e.path=? AND e.kind='image'`, m.Path).Scan(&entity, &grouped)
		if err != nil {
			return 0, err
		}
		if grouped {
			return 0, ErrImageGroupConflict
		}
		ids = append(ids, entity)
		if m.Path == primary {
			primaryID = entity
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_image_groups SET primary_entity_id=? WHERE id=?`, primaryID, id); err != nil {
		return 0, err
	}
	// One bounded statement publishes all members atomically.
	values := make([]string, len(ids))
	args := make([]any, 0, len(ids)*2)
	for i, entity := range ids {
		values[i] = "(?,?)"
		args = append(args, entity, id)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO photo_image_group_members(entity_id,group_id) VALUES `+strings.Join(values, ","), args...); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_entities SET manual_revision=manual_revision+1 WHERE id IN(SELECT entity_id FROM photo_image_group_members WHERE group_id=?)`, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func readImageGroup(ctx context.Context, tx *sql.Tx, id int64) (ImageGroup, error) {
	out := ImageGroup{ID: id}
	err := tx.QueryRowContext(ctx, `SELECT revision,COALESCE(`+imageGroupPrimarySQL("g")+`,g.primary_entity_id) FROM photo_image_groups g WHERE id=?`, id).Scan(&out.Revision, &out.PrimaryID)
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.path FROM photo_image_group_members gm JOIN photo_entities e ON e.id=gm.entity_id WHERE gm.group_id=? ORDER BY e.id LIMIT ?`, id, MaxImageGroupSize+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var m ImageGroupMember
		if err = rows.Scan(&m.EntityID, &m.Media.Path); err != nil {
			break
		}
		m.DisplayPath = MediaDisplayPath(m.Media.Path)
		m.Missing = true
		out.Members = append(out.Members, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Members) > MaxImageGroupSize {
		return out, ErrImageGroupInvalid
	}
	rows, err = tx.QueryContext(ctx, `SELECT `+mediaIndexColumns("mi")+`,e.id FROM photo_image_group_members gm JOIN photo_entities e ON e.id=gm.entity_id JOIN media_index mi ON mi.path=e.path AND mi.type='image' WHERE gm.group_id=?`, id)
	if err != nil {
		return out, err
	}
	live := map[int64]Media{}
	for rows.Next() {
		var m Media
		var entity int64
		m, entity, err = scanIndexedMediaWithRowID(rows)
		if err != nil {
			break
		}
		m.ImageGroupID = id
		live[entity] = m
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range out.Members {
		if m, ok := live[out.Members[i].EntityID]; ok {
			out.Members[i].Media = m
			out.Members[i].Missing = false
		}
	}
	return out, nil
}

// Readers only receive visible members. Mutations require access to the entire
// group so an editor cannot silently change private images through a public one.
func (l *Library) visibleImageGroup(group ImageGroup, admin, write bool) (ImageGroup, error) {
	if admin {
		return group, nil
	}
	members := make([]ImageGroupMember, 0, len(group.Members))
	dirs := map[string]bool{}
	for _, m := range group.Members {
		directory := parentPath(m.Media.Path)
		private, ok := dirs[directory]
		if !ok {
			var err error
			private, err = l.FolderAdminOnly(directory)
			if err != nil {
				return ImageGroup{}, err
			}
			dirs[directory] = private
		}
		if private || m.Media.AdminOnly {
			if write || m.EntityID == group.PrimaryID {
				return ImageGroup{}, errAdminOnly
			}
			continue
		}
		members = append(members, m)
	}
	if len(members) == 0 {
		return ImageGroup{}, os.ErrNotExist
	}
	group.Members = members
	return group, nil
}
func (l *Library) ImageGroup(ctx context.Context, id int64, admin bool) (ImageGroup, error) {
	if l == nil || !l.index.available() {
		return ImageGroup{}, os.ErrNotExist
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ImageGroup{}, err
	}
	defer tx.Rollback()
	group, err := readImageGroup(ctx, tx, id)
	if err != nil {
		return group, err
	}
	return l.visibleImageGroup(group, admin, false)
}

// ApplyImageGroupAction returns false when removing the penultimate member or
// dissolving the group. A stale revision never changes a newer group.
func (l *Library) ApplyImageGroupAction(ctx context.Context, id, revision, entity int64, action string, admin bool) (bool, error) {
	if l == nil || !l.index.available() {
		return false, os.ErrNotExist
	}
	if action != "primary" && action != "remove" && action != "dissolve" {
		return false, ErrImageGroupInvalid
	}
	if revision < 1 {
		return false, ErrImageGroupConflict
	}
	if err := l.lockPhotoIdentities(ctx); err != nil {
		return false, err
	}
	defer l.identityMu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE photo_image_groups SET revision=revision+1 WHERE id=? AND revision=?`, id, revision)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, ErrImageGroupConflict
	}
	group, err := readImageGroup(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if _, err = l.visibleImageGroup(group, admin, true); err != nil {
		return false, err
	}
	found, live := false, false
	for _, m := range group.Members {
		if m.EntityID == entity {
			found = true
			live = !m.Missing
		}
	}
	if action != "dissolve" && (!found || (action == "primary" && !live)) {
		return false, ErrImageGroupInvalid
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_entities SET manual_revision=manual_revision+1 WHERE id IN(SELECT entity_id FROM photo_image_group_members WHERE group_id=?)`, id); err != nil {
		return false, err
	}
	switch action {
	case "primary":
		_, err = tx.ExecContext(ctx, `UPDATE photo_image_groups SET primary_entity_id=? WHERE id=?`, entity, id)
	case "remove":
		if group.PrimaryID == entity {
			for _, m := range group.Members {
				if m.EntityID != entity && !m.Missing {
					_, err = tx.ExecContext(ctx, `UPDATE photo_image_groups SET primary_entity_id=? WHERE id=?`, m.EntityID, id)
					break
				}
			}
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM photo_image_group_members WHERE group_id=? AND entity_id=?`, id, entity)
		}
	case "dissolve":
		_, err = tx.ExecContext(ctx, `DELETE FROM photo_image_groups WHERE id=?`, id)
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_image_groups WHERE id=?)`, id).Scan(&exists); err != nil {
		return false, err
	}
	return exists, tx.Commit()
}

func (l *Library) AddImageGroups(ctx context.Context, items []Media) error {
	if l == nil || !l.index.available() || len(items) == 0 {
		return nil
	}
	positions := map[string][]int{}
	for i := range items {
		items[i].ImageGroupID = 0
		items[i].ImageGroupHidden = false
		positions[items[i].Path] = append(positions[items[i].Path], i)
	}
	paths := make([]string, 0, len(positions))
	for path := range positions {
		paths = append(paths, path)
	}
	for start := 0; start < len(paths); start += searchWriteChunkSize {
		end := min(start+searchWriteChunkSize, len(paths))
		args := make([]any, end-start)
		for i, path := range paths[start:end] {
			args[i] = path
		}
		rows, err := l.index.db.QueryContext(ctx, `SELECT e.path,gm.group_id,mi.image_group_hidden FROM photo_entities e JOIN photo_image_group_members gm ON gm.entity_id=e.id JOIN media_index mi ON mi.path=e.path WHERE e.kind='image' AND e.path IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var path string
			var id int64
			var hidden bool
			if err = rows.Scan(&path, &id, &hidden); err != nil {
				break
			}
			for _, i := range positions[path] {
				items[i].ImageGroupID = id
				items[i].ImageGroupHidden = hidden
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func (l *Library) filterImageGroupMembers(ctx context.Context, items []Media) ([]Media, error) {
	if err := l.AddImageGroups(ctx, items); err != nil {
		return nil, err
	}
	out := items[:0]
	for _, m := range items {
		if !m.ImageGroupHidden {
			out = append(out, m)
		}
	}
	return out, nil
}
