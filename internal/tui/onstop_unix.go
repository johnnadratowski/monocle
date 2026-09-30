//go:build !windows

package tui

import (
	"os/exec"
	"syscall"
)

// onStopShell is the shell the on-stop command runs under: sh, not $SHELL, so
// the command behaves the same for everyone and no interactive rc file runs.
func onStopShell() (string, string) { return "sh", "-c" }

// killGroupOnCancel starts the command in its own process group and makes a
// cancel kill the group.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
