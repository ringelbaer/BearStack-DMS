//go:build unix

package photos

import "golang.org/x/sys/unix"

// Do not follow a swapped symlink or block on a file replaced with a FIFO.
const sidecarOpenFlags = unix.O_NOFOLLOW | unix.O_NONBLOCK
