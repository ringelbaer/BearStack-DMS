// Datei registriert projektspezifische SQLite-Funktionen fuer Abfragen und Sortierung.
package sqlitefuncs

import (
	"database/sql/driver"
	"fmt"
	"sync"

	"bearstack/internal/searchtext"

	"modernc.org/sqlite"
)

var (
	registerGermanFoldOnce sync.Once
	registerGermanFoldErr  error
)

func RegisterGermanFold() error {
	registerGermanFoldOnce.Do(func() {
		registerGermanFoldErr = sqlite.RegisterDeterministicScalarFunction("bearstack_german_fold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) != 1 || args[0] == nil {
				return "", nil
			}
			return searchtext.GermanFold(fmt.Sprint(args[0])), nil
		})
	})
	return registerGermanFoldErr
}
