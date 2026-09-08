package fsutil

import "os"

// RootPathBatch shares ancestor checks within one synchronous traversal.
// It is not concurrency-safe and must never survive that traversal or be used
// for subsequent file access: later filesystem changes require a fresh check.
type RootPathBatch struct {
	root    string
	checked map[string]pathStatResult
	lstat   func(string) (os.FileInfo, error)
}

type pathStatResult struct {
	info os.FileInfo
	err  error
}

func NewRootPathBatch(root string) *RootPathBatch {
	return &RootPathBatch{root: root, checked: make(map[string]pathStatResult), lstat: os.Lstat}
}

func (b *RootPathBatch) Resolve(rel string, allowEmpty bool, escapeErr error) (string, string, error) {
	return resolveWithinRoot(b.root, rel, allowEmpty, escapeErr, b.stat)
}

func (b *RootPathBatch) stat(path string) (os.FileInfo, error) {
	if result, ok := b.checked[path]; ok {
		return result.info, result.err
	}
	info, err := b.lstat(path)
	b.checked[path] = pathStatResult{info: info, err: err}
	return info, err
}
