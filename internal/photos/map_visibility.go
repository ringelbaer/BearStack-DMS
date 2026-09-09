package photos

import "context"

// Aggregated positions must not reveal a newly protected directory while its
// ordinary index scan is still pending. Check only indexed GPS directories,
// using the shared visibility checker and bounded ancestor caches per batch.
func (l *Library) refreshMapVisibility(ctx context.Context, rel string) error {
	return l.faceVisibility.check(ctx, "map:"+rel, func() error {
		cursor := ""
		first := true
		for {
			where := " WHERE admin_only=0 AND latitude IS NOT NULL AND longitude IS NOT NULL"
			var args []any
			if rel != "" {
				start, end := prefixRange(rel + "/")
				where += " AND (directory=? OR (directory>=? AND directory<?))"
				args = append(args, rel, start, end)
			}
			if !first {
				where += " AND directory>?"
				args = append(args, cursor)
			}
			rows, err := l.index.db.QueryContext(ctx, `SELECT DISTINCT directory FROM media_index INDEXED BY idx_media_index_public_gps_directory`+where+` ORDER BY directory LIMIT 256`, args...)
			if err != nil {
				return err
			}
			dirs := make([]string, 0, 256)
			for rows.Next() {
				var dir string
				if err = rows.Scan(&dir); err != nil {
					rows.Close()
					return err
				}
				dirs = append(dirs, dir)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(dirs) == 0 {
				return ctx.Err()
			}
			visibility := newFaceDirectoryVisibility(l.root)
			var hidden []string
			for _, dir := range dirs {
				if err = ctx.Err(); err != nil {
					return err
				}
				private, e := visibility.check(dir)
				if e != nil {
					return e
				}
				if private {
					hidden = append(hidden, dir)
				}
			}
			if len(hidden) > 0 {
				tx, e := l.index.db.BeginTx(ctx, nil)
				if e != nil {
					return e
				}
				for _, dir := range hidden {
					if _, e = tx.ExecContext(ctx, `UPDATE media_index SET admin_only=1 WHERE directory=? AND admin_only=0`, dir); e != nil {
						tx.Rollback()
						return e
					}
				}
				if e = tx.Commit(); e != nil {
					return e
				}
			}
			if len(dirs) < 256 {
				return ctx.Err()
			}
			cursor = dirs[len(dirs)-1]
			first = false
		}
	})
}
