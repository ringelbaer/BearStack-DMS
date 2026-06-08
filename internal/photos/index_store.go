// Datei kapselt Oeffnen, Verfuegbarkeit und Lebenszyklus des Fotoindex-Stores.
package photos

import "database/sql"

type photoIndexStore struct {
	db *sql.DB
}

func openPhotoIndexStore(path string) (*photoIndexStore, string, error) {
	db, abs, err := openIndexDB(path)
	if err != nil || db == nil {
		return nil, abs, err
	}
	return &photoIndexStore{db: db}, abs, nil
}

func (s *photoIndexStore) available() bool {
	return s != nil && s.db != nil
}

func (s *photoIndexStore) close() error {
	if !s.available() {
		return nil
	}
	return s.db.Close()
}
