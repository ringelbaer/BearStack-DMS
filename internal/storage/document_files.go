package storage

import "context"

type documentFileGate struct {
	busy chan struct{}
	refs int
}

// AcquireDocumentFiles coordinates derived-file creation and final deletion.
// Idle entries are removed, so memory usage follows concurrency, not archive size.
func (s *Store) AcquireDocumentFiles(ctx context.Context, id int64) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.documentMu.Lock()
	if s.documentGates == nil {
		s.documentGates = make(map[int64]*documentFileGate)
	}
	gate := s.documentGates[id]
	if gate == nil {
		gate = &documentFileGate{busy: make(chan struct{}, 1)}
		s.documentGates[id] = gate
	}
	gate.refs++
	s.documentMu.Unlock()
	unref := func() {
		s.documentMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(s.documentGates, id)
		}
		s.documentMu.Unlock()
	}
	select {
	case gate.busy <- struct{}{}:
		return func() { <-gate.busy; unref() }, nil
	case <-ctx.Done():
		unref()
		return nil, ctx.Err()
	}
}
