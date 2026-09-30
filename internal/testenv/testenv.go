// Package testenv keeps test runs away from the user's real Monocle state.
//
// Monocle resolves its config (XDG_CONFIG_HOME), database and media store
// (MONOCLE_DB, XDG_DATA_HOME) from the environment, and a test that reaches
// SaveConfig, opens the default database, or stores a media artifact through a
// real engine writes wherever those point. Left unset they point at the user's
// own ~/.config/monocle and ~/.local/share/monocle: the engine round-trip test
// rewrote the real config.json on every run, and a test opening the default
// database would migrate the live one to the working tree's schema — after
// which an installed binary built from an older tree refuses to open it.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
)

// Isolate points the config dir, the data dir and the database at a fresh
// temporary directory for the whole test binary, and returns a cleanup that
// removes it. Call it from TestMain before m.Run. Tests that set these
// variables themselves (t.Setenv) still win for their own duration.
func Isolate(pkg string) (func(), error) {
	dir, err := os.MkdirTemp("", "monocle-"+pkg+"-test-")
	if err != nil {
		return nil, fmt.Errorf("testenv: %w", err)
	}
	for k, v := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME":   filepath.Join(dir, "data"),
		"MONOCLE_DB":      filepath.Join(dir, "data", "monocle", "monocle.db"),
	} {
		if err := os.Setenv(k, v); err != nil {
			return nil, fmt.Errorf("testenv: set %s: %w", k, err)
		}
	}
	return func() { _ = os.RemoveAll(dir) }, nil
}

// Main is a whole TestMain: isolate, run, clean up, exit with the run's code.
func Main(pkg string, run func() int) {
	cleanup, err := Isolate(pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := run()
	cleanup()
	os.Exit(code)
}
