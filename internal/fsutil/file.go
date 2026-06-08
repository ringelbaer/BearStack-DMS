// Datei bietet Hilfsfunktionen fuer sichere Dateioperationen und Pfadpruefungen im Dateisystem.
package fsutil

import "os"

func FileHasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}
