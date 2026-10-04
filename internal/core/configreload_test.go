package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// reloadEngine is an engine made by NewEngine over a config dir of its own,
// and the path of the global config file it reads.
func reloadEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Chdir(t.TempDir()) // no project-level .monocle/config.json
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	e, err := NewEngine(DefaultConfig(), database, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "monocle", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return e, path
}

// The engine outlives the TUIs attached to it, so a setting added to
// config.json while it runs takes effect on the next read, without a restart.
func TestEngineReadsAChangedConfigFile(t *testing.T) {
	e, path := reloadEngine(t)
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resolve := func() string { return e.GetConfig().WalkthroughResolve }

	if got := resolve(); got != "" {
		t.Fatalf("walkthrough_resolve %q before any file, want empty", got)
	}
	write(`{"walkthrough_resolve": "resolve-refs"}`)
	if got := resolve(); got != "resolve-refs" {
		t.Errorf("after writing config.json: %q, want resolve-refs", got)
	}
	// A file caught half-written keeps the last good config.
	write(`{"walkthrough_resolve": "resol`)
	if got := resolve(); got != "resolve-refs" {
		t.Errorf("a half-written file gave %q, want the last good resolve-refs", got)
	}
	write(`{"walkthrough_resolve": "resolve-refs-v2", "wrap": true}`)
	if cfg := e.GetConfig(); cfg.WalkthroughResolve != "resolve-refs-v2" || !cfg.Wrap {
		t.Errorf("after the write finished: %q wrap=%v", cfg.WalkthroughResolve, cfg.Wrap)
	}
	// Settings the files leave out are the defaults.
	if got := e.GetConfig().TabSize; got != DefaultConfig().TabSize {
		t.Errorf("tab_size %d, want the default %d", got, DefaultConfig().TabSize)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := resolve(); got != "" {
		t.Errorf("after removing config.json: %q, want the default", got)
	}
}

// The engine's own write is not read back as an outside change: the config it
// saved stays the one it answers with.
func TestEngineDoesNotReadBackItsOwnSave(t *testing.T) {
	e, _ := reloadEngine(t)
	cfg := e.GetConfig()
	cfg.Wrap = true
	if err := e.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	if got := e.GetConfig(); got != cfg {
		t.Error("the engine read its own save back in place of the config it saved")
	}
	// A client's save, the same.
	if resp := e.handleSaveConfig(&protocol.SaveConfigMsg{Config: types.Config{Theme: "light"}}); resp.Error != "" {
		t.Fatal(resp.Error)
	}
	stored := e.cfg.Load()
	if got := e.GetConfig(); got != stored || got.Theme != "light" {
		t.Error("the engine read a client's save back in place of the config it stored")
	}
}
