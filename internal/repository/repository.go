// Datei oeffnet das Repository, haelt die Datenbankverbindung und buendelt zentrale Zugriffsmethoden.
package repository

import (
	"context"
	"database/sql"

	"bearstack/internal/sqlitefuncs"

	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

const sqliteMaxOpenConns = 4

func Open(ctx context.Context, dbPath string) (*Repository, error) {
	if err := ensureParentDir(dbPath); err != nil {
		return nil, err
	}
	if err := sqlitefuncs.RegisterGermanFold(); err != nil {
		return nil, err
	}

	dsn, err := sqliteDSN(dbPath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)

	repo := &Repository{db: db}
	if err := repo.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}
