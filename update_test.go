package bearstack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateInstallsOnlyAfterSuccessfulServiceStop(t *testing.T) {
	for _, scenario := range []struct {
		name, state          string
		stopFailure, install bool
	}{
		{"success", "ActiveState=inactive\nResult=success\nMainPID=0", false, true},
		{"timeout", "ActiveState=failed\nResult=timeout\nMainPID=0", false, false},
		{"exit-failure", "ActiveState=inactive\nResult=exit-code\nMainPID=0", false, false},
		{"still-running", "ActiveState=inactive\nResult=success\nMainPID=012", false, false},
		{"missing-result", "ActiveState=inactive\nMainPID=0", false, false},
		{"stop-failure", "", true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0750); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(root, "commands")
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/usr/bin/env bash\nset -eu\n"+body), 0750); err != nil {
					t.Fatal(err)
				}
			}
			write("git", `printf 'git %s\n' "$*" >> "$UPDATE_TEST_LOG"`)
			write("go", `printf 'go %s\n' "$*" >> "$UPDATE_TEST_LOG"`)
			write("sudo", `exec "$@"`)
			write("install", `printf 'install\n' >> "$UPDATE_TEST_LOG"`)
			write("systemctl", `printf 'systemctl %s\n' "$*" >> "$UPDATE_TEST_LOG"
if [[ "$1" == stop && "$UPDATE_TEST_STOP_FAILURE" == 1 ]]; then exit 1; fi
if [[ "$1" == show ]]; then printf '%s\n' "$UPDATE_TEST_STATE"; fi
`)
			script, err := filepath.Abs("update.sh")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script)
			failure := "0"
			if scenario.stopFailure {
				failure = "1"
			}
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "BEARSTACK_REPO_DIR="+root, "BEARSTACK_SERVICE=bearstack-test.service", "UPDATE_TEST_LOG="+logPath, "UPDATE_TEST_STATE="+scenario.state, "UPDATE_TEST_STOP_FAILURE="+failure)
			output, err := cmd.CombinedOutput()
			if (err == nil) != scenario.install {
				t.Fatalf("exit=%v output=%s", err, output)
			}
			log, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			commands := string(log)
			installed := strings.Contains(commands, "\ninstall\n")
			started := strings.Contains(commands, "systemctl start ")
			if installed != scenario.install || started != scenario.install {
				t.Fatalf("unexpected mutations:\n%s", commands)
			}
			if installed && strings.Index(commands, "systemctl show ") > strings.Index(commands, "\ninstall\n") {
				t.Fatal("installed before verifying stop")
			}
		})
	}
}
