package photos

import (
	"context"
	"database/sql"
)

// Membership refers to immutable identities; originals and sidecars are never written.
// The serving flag avoids a join for each candidate in gallery/map/frame queries.
func setupImageGroups(ctx context.Context, db *sql.DB) error {
	if _, err := ensurePhotoColumn(ctx, db, "media_index", "image_group_hidden", `ALTER TABLE media_index ADD COLUMN image_group_hidden INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS photo_image_groups(id INTEGER PRIMARY KEY AUTOINCREMENT, primary_entity_id INTEGER NOT NULL, revision INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS photo_image_group_members(entity_id INTEGER PRIMARY KEY REFERENCES photo_entities(id) ON DELETE CASCADE, group_id INTEGER NOT NULL REFERENCES photo_image_groups(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_image_group_members_group ON photo_image_group_members(group_id,entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_media_image_group_hidden ON media_index(directory,admin_only) WHERE image_group_hidden=1`,
		`CREATE TRIGGER IF NOT EXISTS image_group_delete AFTER DELETE ON photo_image_groups BEGIN DELETE FROM photo_image_group_members WHERE group_id=old.id; END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_entity_delete BEFORE DELETE ON photo_entities BEGIN DELETE FROM photo_image_group_members WHERE entity_id=old.id; END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_primary AFTER UPDATE OF primary_entity_id ON photo_image_groups BEGIN ` + refreshImageGroupSQL("new.id") + ` END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_member_insert AFTER INSERT ON photo_image_group_members BEGIN UPDATE media_index SET image_group_hidden=CASE WHEN new.entity_id=(SELECT ` + imageGroupPrimarySQL("g") + ` FROM photo_image_groups g WHERE g.id=new.group_id) THEN 0 ELSE 1 END WHERE path=(SELECT path FROM photo_entities WHERE id=new.entity_id); END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_member_delete AFTER DELETE ON photo_image_group_members BEGIN
 UPDATE media_index SET image_group_hidden=0 WHERE path=(SELECT path FROM photo_entities WHERE id=old.entity_id);
 UPDATE photo_image_groups SET revision=revision+1 WHERE id=old.group_id;
 DELETE FROM photo_image_groups WHERE id=old.group_id AND (SELECT count(*) FROM photo_image_group_members WHERE group_id=old.group_id)<2;
 ` + refreshImageGroupSQL("old.group_id") + ` END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_media_insert AFTER INSERT ON media_index WHEN new.type='image' BEGIN ` + refreshImageGroupSQL(`(SELECT gm.group_id FROM photo_entities e JOIN photo_image_group_members gm ON gm.entity_id=e.id WHERE e.kind='image' AND e.path=new.path)`) + ` END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_media_delete AFTER DELETE ON media_index WHEN old.type='image' BEGIN ` + refreshImageGroupSQL(`(SELECT gm.group_id FROM photo_entities e JOIN photo_image_group_members gm ON gm.entity_id=e.id WHERE e.kind='image' AND e.path=old.path)`) + ` END`,
		`CREATE TRIGGER IF NOT EXISTS image_group_entity_move AFTER UPDATE OF path ON photo_entities WHEN new.kind='image' BEGIN ` + refreshImageGroupSQL(`(SELECT group_id FROM photo_image_group_members WHERE entity_id=new.id)`) + ` END`,
	}
	for _, stmt := range statements {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// If the chosen original is temporarily missing, use the first live member.
// Its immutable identity keeps the chosen primary when the original returns.
func imageGroupPrimarySQL(group string) string {
	return `COALESCE((SELECT e.id FROM photo_entities e JOIN media_index m ON m.path=e.path AND m.type='image' WHERE e.id=` + group + `.primary_entity_id),
 (SELECT min(gm.entity_id) FROM photo_image_group_members gm JOIN photo_entities e ON e.id=gm.entity_id JOIN media_index m ON m.path=e.path AND m.type='image' WHERE gm.group_id=` + group + `.id))`
}
func refreshImageGroupSQL(groupID string) string {
	return `UPDATE media_index SET image_group_hidden=CASE WHEN (SELECT e.id FROM photo_entities e WHERE e.path=media_index.path AND e.kind='image')=(SELECT ` + imageGroupPrimarySQL("g") + ` FROM photo_image_groups g WHERE g.id=` + groupID + `) THEN 0 ELSE 1 END
 WHERE path IN(SELECT e.path FROM photo_image_group_members gm JOIN photo_entities e ON e.id=gm.entity_id WHERE gm.group_id=` + groupID + `);`
}
