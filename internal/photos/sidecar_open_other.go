//go:build !unix

package photos

// The common reader checks file type and identity before reading any bytes.
const sidecarOpenFlags = 0
