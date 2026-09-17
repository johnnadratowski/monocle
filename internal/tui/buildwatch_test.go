package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBinary writes an executable that prints a version, so the watch can be
// exercised end to end including the "what is installed now?" question.
func fakeBinary(t *testing.T, path, version string) {
	t.Helper()
	script := "#!/bin/sh\necho " + version + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
}

func TestBuildWatch(t *testing.T) {
	t.Run("an unchanged binary reports nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "monocle")
		fakeBinary(t, path, "v1")
		info, _ := os.Stat(path)
		w := buildWatch{path: path, size: info.Size(), modTime: info.ModTime()}

		w, upgraded := w.check()
		if upgraded || w.pending {
			t.Error("nothing changed on disk; there is no upgrade to report")
		}
		if w.notice("ctrl+r") != "" {
			t.Errorf("notice = %q, want empty", w.notice("ctrl+r"))
		}
	})

	t.Run("a replaced binary is noticed and named", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "monocle")
		fakeBinary(t, path, "main-aaaaaaa")
		info, _ := os.Stat(path)
		w := buildWatch{path: path, size: info.Size(), modTime: info.ModTime()}

		fakeBinary(t, path, "main-bbbbbbb-longer")
		// Same-second writes would compare equal on mtime alone; the size differs
		// here, which is the other half of the check.
		w, upgraded := w.check()
		if !upgraded || !w.pending {
			t.Fatal("a replaced binary should register as an upgrade")
		}
		if w.version != "main-bbbbbbb-longer" {
			t.Errorf("version = %q, want the newly installed one", w.version)
		}
		n := w.notice("ctrl+r")
		if !strings.Contains(n, "main-bbbbbbb-longer") || !strings.Contains(n, "ctrl+r") {
			t.Errorf("notice = %q, want it to name the build and the key", n)
		}
	})

	// Size alone can collide; mtime is what catches a rebuild of identical size.
	t.Run("a same-size rebuild is still noticed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "monocle")
		fakeBinary(t, path, "v1")
		info, _ := os.Stat(path)
		w := buildWatch{path: path, size: info.Size(), modTime: info.ModTime()}

		fakeBinary(t, path, "v2") // identical length
		os.Chtimes(path, time.Now().Add(time.Minute), time.Now().Add(time.Minute))
		if _, upgraded := w.check(); !upgraded {
			t.Error("a same-size rebuild should still register")
		}
	})

	// The upgrade is an edge, so a caller that polls forever reacts once.
	t.Run("the upgrade is reported once", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "monocle")
		fakeBinary(t, path, "v1")
		info, _ := os.Stat(path)
		w := buildWatch{path: path, size: info.Size(), modTime: info.ModTime()}

		fakeBinary(t, path, "v2-longer")
		w, first := w.check()
		w, second := w.check()
		if !first || second {
			t.Errorf("first=%v second=%v, want true then false", first, second)
		}
		if !w.pending {
			t.Error("pending should stay set; an upgrade does not un-happen")
		}
	})

	t.Run("a binary that vanishes mid-write is not an upgrade yet", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "monocle")
		fakeBinary(t, path, "v1")
		info, _ := os.Stat(path)
		w := buildWatch{path: path, size: info.Size(), modTime: info.ModTime()}

		os.Remove(path)
		if _, upgraded := w.check(); upgraded {
			t.Error("a missing file is a replacement in progress, not a finished one")
		}
	})

	// A watch that could not resolve its own path must stay silent rather than
	// stat "" forever.
	t.Run("an inert watch does nothing", func(t *testing.T) {
		var w buildWatch
		if _, upgraded := w.check(); upgraded {
			t.Error("a watch with no path cannot see an upgrade")
		}
		if w.notice("ctrl+r") != "" {
			t.Error("an inert watch has nothing to say")
		}
	})

	t.Run("an unnameable build still gets a notice", func(t *testing.T) {
		w := buildWatch{pending: true}
		if n := w.notice("ctrl+r"); !strings.Contains(n, "new build") {
			t.Errorf("notice = %q, want a generic upgrade hint", n)
		}
	})
}

// TestRequestRelaunch covers the two refusals. Both exist so a mistyped key
// cannot quit a session, and so a half-written comment — the one piece of state
// that lives only in this process — is never thrown away silently.
func TestRequestRelaunch(t *testing.T) {
	newModel := func() appModel {
		km := DefaultKeyMap()
		m := appModel{keys: km}
		m.buildWatch = buildWatch{pending: true, version: "main-new"}
		return m
	}

	t.Run("relaunches when a build is waiting", func(t *testing.T) {
		m, cmd := newModel().requestRelaunch()
		if !m.relaunchRequested {
			t.Error("expected the relaunch to be requested")
		}
		if cmd == nil {
			t.Error("expected the program to be asked to quit")
		}
	})

	t.Run("refuses when already current", func(t *testing.T) {
		m := newModel()
		m.buildWatch.pending = false
		m, cmd := m.requestRelaunch()
		if m.relaunchRequested || cmd != nil {
			t.Error("with no new build there is nothing to restart onto")
		}
		if !strings.Contains(m.statusBar.searchInfo, "already running") {
			t.Errorf("expected an explanation, got %q", m.statusBar.searchInfo)
		}
	})

	t.Run("refuses while an overlay is open", func(t *testing.T) {
		m := newModel()
		m.overlay = overlayHelp
		m, cmd := m.requestRelaunch()
		if m.relaunchRequested || cmd != nil {
			t.Error("a restart would discard whatever the overlay holds")
		}
		if !strings.Contains(m.statusBar.searchInfo, "close this first") {
			t.Errorf("expected an explanation, got %q", m.statusBar.searchInfo)
		}
	})
}

func TestWantsRelaunch(t *testing.T) {
	if WantsRelaunch(appModel{}) {
		t.Error("a model that did not ask must not be restarted")
	}
	if !WantsRelaunch(appModel{relaunchRequested: true}) {
		t.Error("a model that asked should be restarted")
	}
}
