// Datei erzeugt SQLite-DSNs mit den benoetigten Optionen fuer lokale Datenbankverbindungen.
package sqlitedsn

import (
	"net/url"
	"path/filepath"
)

func WithPragmas(dsn string, pragmas ...string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for _, pragma := range pragmas {
		if pragma == "" {
			continue
		}
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func FilePath(path string, pragmas ...string) (string, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return WithPragmas(u.String(), pragmas...)
}
