//go:build !linux

// Datei stellt die neutrale Prozessprioritaets-Implementierung fuer Nicht-Linux-Systeme bereit.
package photos

func withLowIndexPriority(fn func() (IndexStats, error)) (IndexStats, error) {
	return fn()
}
