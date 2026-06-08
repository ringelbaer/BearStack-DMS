// Datei sammelt interne Hilfsfunktionen fuer SQL-Ausfuehrung, Normalisierung und Fehlerbehandlung.
package repository

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"bearstack/internal/document"
	"bearstack/internal/searchtext"
	"bearstack/internal/sqlitedsn"
)

func buildFTSQuery(input string) string {
	return searchtext.FTSAndQuery(input, 16)
}

func documentSearchTokens(input string) []string {
	return searchtext.FTSTokens(input, 16)
}

func truncateString(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range value {
		if count == maxRunes {
			return value[:i]
		}
		count++
	}
	return value
}

func searchIndexTextFor(doc document.Document) string {
	var text strings.Builder
	if len(doc.CustomValues) > 0 {
		keys := make([]int64, 0, len(doc.CustomValues))
		for key := range doc.CustomValues {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, key := range keys {
			appendNormalizedSearchTerms(&text, doc.CustomValues[key])
		}
	}
	appendNormalizedSearchTerms(&text, doc.ContentText)
	return text.String()
}

func appendNormalizedSearchTerms(text *strings.Builder, value string) {
	inTerm := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			inTerm = false
			continue
		}
		if !inTerm {
			if text.Len() > 0 {
				text.WriteByte(' ')
			}
			inTerm = true
		}
		text.WriteRune(unicode.ToLower(r))
	}
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func ensureParentDir(path string) error {
	if path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}

func sqliteDSN(path string) (string, error) {
	pragmas := []string{
		"busy_timeout(5000)",
		"foreign_keys(ON)",
		"journal_mode(WAL)",
	}
	if path == ":memory:" {
		return sqlitedsn.WithPragmas("file::memory:?cache=shared", pragmas...)
	}
	if strings.HasPrefix(path, "file:") {
		return sqlitedsn.WithPragmas(path, pragmas...)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return sqlitedsn.FilePath(abs, pragmas...)
}
