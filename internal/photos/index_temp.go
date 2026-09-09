package photos

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strconv"
	"time"
)

// Isolate file-backed temporary sorting to a leased connection. Restore pool
// settings even when a client cancels, discarding the connection on failure.
func photoFileTempConn(ctx context.Context, db *sql.DB) (*sql.Conn, func(), error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	var previous int
	if err = conn.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&previous); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if _, err = conn.ExecContext(ctx, "PRAGMA temp_store=FILE"); err != nil {
		conn.Close()
		return nil, nil, err
	}
	closeConn := func() {
		restore, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, e := conn.ExecContext(restore, "PRAGMA temp_store="+strconv.Itoa(previous)); e != nil {
			// Never return a connection with unknown settings to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}
	return conn, closeConn, nil
}
