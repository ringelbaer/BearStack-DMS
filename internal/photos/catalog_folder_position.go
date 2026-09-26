package photos

import (
	"context"
	"database/sql"
	"path"
)

// CatalogFolderPosition uses the native gallery's 96-item descending-date pages.
// Both reads share a snapshot; only a count and the target key leave SQLite.
type CatalogFolderPosition struct {
	Path      string `json:"path"`
	Directory string `json:"directory"`
	Page      int    `json:"page"`
}

func (l *Library) CatalogFolderPosition(ctx context.Context, mediaPath string) (CatalogFolderPosition, error) {
	out := CatalogFolderPosition{Path: mediaPath, Directory: path.Dir(mediaPath), Page: 1}
	if out.Directory == "." {
		out.Directory = ""
	}
	state, err := l.indexedListingPathState(ctx, out.Directory, false, false)
	if err != nil {
		return out, err
	}
	if !state.Covered {
		return out, ErrCatalogIndexUnavailable
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var captured string
	var modified int64
	err = tx.QueryRowContext(ctx, `SELECT captured_at,mod_time_unix_nano FROM media_index WHERE path=? AND directory=? AND admin_only=0 AND image_group_hidden=0`, mediaPath, out.Directory).Scan(&captured, &modified)
	if err != nil {
		return out, err
	}
	var preceding int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM media_index WHERE admin_only=0 AND directory=? AND image_group_hidden=0 AND (captured_at,mod_time_unix_nano,path)>(?,?,?)`, out.Directory, captured, modified, mediaPath).Scan(&preceding)
	out.Page = preceding/96 + 1
	return out, err
}
