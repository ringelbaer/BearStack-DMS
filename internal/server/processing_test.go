package server

import (
	"testing"
)

func TestShouldLogOCRProgress(t *testing.T) {
	cases := []struct {
		name      string
		current   int
		total     int
		last      int
		lastTotal int
		message   string
		shouldLog bool
	}{
		{name: "empty message", current: 1, total: 100, last: -1, lastTotal: -1, shouldLog: false},
		{name: "initial", current: 0, total: 100, last: -1, lastTotal: -1, message: "start", shouldLog: true},
		{name: "duplicate state", current: 0, total: 100, last: 0, lastTotal: 100, message: "same", shouldLog: false},
		{name: "bulk unknown total", current: 0, total: 0, last: -1, lastTotal: -1, message: "prepare", shouldLog: true},
		{name: "bulk total discovered", current: 0, total: 42, last: 0, lastTotal: 0, message: "pages", shouldLog: true},
		{name: "ordinary page", current: 7, total: 100, last: 0, lastTotal: 100, message: "progress", shouldLog: false},
		{name: "tenth page", current: 10, total: 100, last: 0, lastTotal: 100, message: "progress", shouldLog: true},
		{name: "finished", current: 100, total: 100, last: 90, lastTotal: 100, message: "done", shouldLog: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldLogOCRProgress(tc.current, tc.total, tc.last, tc.lastTotal, tc.message)
			if got != tc.shouldLog {
				t.Fatalf("shouldLogOCRProgress() = %v, want %v", got, tc.shouldLog)
			}
		})
	}
}
