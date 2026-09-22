//go:build linux

package processrun

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if cmd.Cancel != nil {
		cmd.Cancel = func() error { return killGroup(cmd) }
	}
}
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
func cleanupProcess(cmd *exec.Cmd) { _ = killGroup(cmd) }

func confine(cmd *exec.Cmd, files Files) error {
	launcher, err := exec.LookPath("bwrap")
	if err != nil {
		return fmt.Errorf("converter sandbox requires bubblewrap: %w", err)
	}
	args := []string{launcher, "--unshare-all", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
	// Only public runtime files are visible. The server's data, configuration and
	// other users' files are absent from this mount namespace, including via /proc.
	for _, path := range []string{"/usr", "/bin", "/lib", "/lib64", "/etc/fonts", "/var/cache/fontconfig", "/etc/libreoffice", "/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/alternatives", "/etc/localtime", "/etc/papersize", "/etc/passwd", "/etc/group", "/etc/nsswitch.conf", "/etc/gnutls"} {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}
	outputs, err := absolutePaths(files.Write)
	if err != nil {
		return err
	}
	for _, path := range outputs {
		args = append(args, "--bind", path, path)
	}
	// Bind inputs last: even an input beside an output must remain read-only.
	inputs, err := absolutePaths(files.Read)
	if err != nil {
		return err
	}
	for _, path := range inputs {
		args = append(args, "--ro-bind", path, path)
	}
	binary, err := filepath.Abs(cmd.Path)
	if err != nil {
		return err
	}
	// Custom converter installations carry libraries/helpers beside the executable.
	dir := filepath.Dir(binary)
	cwd := cmd.Dir
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	args = append(args, "--ro-bind", dir, dir, "--dir", cwd, "--chdir", cwd, "--clearenv")
	for _, entry := range cmd.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			args = append(args, "--setenv", key, value)
		}
	}
	args = append(args, "--", binary)
	args = append(args, cmd.Args[1:]...)
	cmd.Path, cmd.Args = launcher, args
	return nil
}
