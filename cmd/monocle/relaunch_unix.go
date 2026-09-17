//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// relaunchSelf replaces this process with the binary now installed at the same
// path, keeping the terminal — and so the tmux pane — the reviewer started it
// in. exec rather than spawn-and-exit: a child would inherit the terminal while
// its parent was still holding it, and the shell would see the pane's job end.
//
// Nothing needs saving first. The review, its comments and what is marked
// reviewed all live in the engine's database, and the fresh process re-runs the
// version check that reaps the now-stale engine, so both halves come back
// current.
func relaunchSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("relaunch: resolve executable: %w", err)
	}
	// Only returns on failure.
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		return fmt.Errorf("relaunch: exec %s: %w", exe, err)
	}
	return nil
}
