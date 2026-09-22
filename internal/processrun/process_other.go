//go:build !linux

package processrun

import (
	"errors"
	"os/exec"
)

func prepareProcess(cmd *exec.Cmd) {}
func cleanupProcess(cmd *exec.Cmd) {}
func confine(cmd *exec.Cmd, files Files) error {
	return errors.New("Bubblewrap converter sandbox requires Linux")
}
