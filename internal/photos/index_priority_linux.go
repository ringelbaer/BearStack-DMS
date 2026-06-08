//go:build linux

// Datei setzt Linux-spezifische Prozessprioritaet fuer ressourcenschonende Indexlaeufe.
package photos

import (
	"runtime"

	"golang.org/x/sys/unix"
)

const (
	indexIOPriorityWhoProcess = 1
	indexIOPriorityClassIdle  = 3
)

func withLowIndexPriority(fn func() (IndexStats, error)) (IndexStats, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := unix.Gettid()
	if unix.Geteuid() == 0 {
		previousNice, niceErr := unix.Getpriority(unix.PRIO_PROCESS, tid)
		if niceErr == nil {
			_ = unix.Setpriority(unix.PRIO_PROCESS, tid, 19)
			defer unix.Setpriority(unix.PRIO_PROCESS, tid, previousNice)
		}
	}

	previousIO, _, ioErr := unix.Syscall(unix.SYS_IOPRIO_GET, uintptr(indexIOPriorityWhoProcess), uintptr(tid), 0)
	if ioErr == 0 {
		idlePriority := uintptr(indexIOPriorityClassIdle << 13)
		_, _, _ = unix.Syscall(unix.SYS_IOPRIO_SET, uintptr(indexIOPriorityWhoProcess), uintptr(tid), idlePriority)
		defer unix.Syscall(unix.SYS_IOPRIO_SET, uintptr(indexIOPriorityWhoProcess), uintptr(tid), previousIO)
	}

	return fn()
}
