package photos

import (
	"context"

	"bearstack/internal/sqlutil"
)

func (s *photoIndexStore) mediaCacheForPaths(ctx context.Context, paths []string) (map[string]cachedMediaRow, error) {
	// A non-nil map disables the per-file fallback query, including cache misses.
	cache := make(map[string]cachedMediaRow, len(paths))
	if !s.available() {
		return cache, nil
	}
	for start := 0; start < len(paths); start += searchWriteChunkSize {
		end := min(start+searchWriteChunkSize, len(paths))
		args := make([]any, end-start)
		for i, path := range paths[start:end] {
			args[i] = path
		}
		rows, err := s.db.QueryContext(ctx, `SELECT `+mediaIndexColumns(``)+` FROM media_index WHERE path IN (`+sqlutil.Placeholders(len(args))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			row, err := scanCachedMediaRow(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			cache[row.Path] = row
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return cache, nil
}
