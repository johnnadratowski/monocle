//go:build windows

package tui

import (
	"os"
	"os/exec"
)

func onStopShell() (string, string) {
	if sh := os.Getenv("COMSPEC"); sh != "" {
		return sh, "/c"
	}
	return "cmd", "/c"
}

// killGroupOnCancel keeps exec's default: kill the process itself.
func killGroupOnCancel(*exec.Cmd) {}
