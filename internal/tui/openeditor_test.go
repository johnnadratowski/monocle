package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

// zoomTmux is fakeTmux answering "not zoomed" when asked about a window, so a
// zoom goes ahead.
func zoomTmux(t *testing.T) string {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$1\" in split-window) echo '%7' ;; display-message) echo 0 ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// openEditorApp is a tour on 1.2 with related_editor_add recording each file
// it is run for, and the related pane alive as alive says.
func openEditorApp(t *testing.T, alive bool) (appModel, string, *[]relatedPanePlan, string) {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	added := filepath.Join(dir, "added")
	m, _ := tourAppWith(t, testTour(), &types.Config{Editor: "nvim",
		RelatedEditorAdd: `printf '%s|%s\n' {file} {line} >> ` + added})
	m.repoRoot, m.diffView.repoRoot = dir, dir
	m = pressKey(t, m, ".") // 1.2
	plans := captureRelated(t)
	log := zoomTmux(t)
	orig := findPane
	findPane = func(string, string) string {
		if alive {
			return "%7"
		}
		return ""
	}
	t.Cleanup(func() { findPane = orig })
	return m, added, plans, log
}

func openEditor(t *testing.T, m appModel, msg openEditorMsg) appModel {
	t.Helper()
	next, cmd := m.Update(msg)
	return driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
}

// open_editor opens the file in the editor already beside Monocle, gives that
// pane the keyboard, and with full zooms it — leaving the diff where it is.
func TestOpenEditorUsesTheRelatedEditor(t *testing.T) {
	for _, full := range []bool{false, true} {
		m, added, plans, log := openEditorApp(t, true)
		cursor, offset := m.diffView.cursor, m.diffView.offset
		m = openEditor(t, m, openEditorMsg{path: "lib/util.go", line: 12, full: full})
		want := []string{filepath.Join(m.repoRoot, "lib/util.go") + "|12"}
		if got := readLines(t, added); !reflect.DeepEqual(got, want) {
			t.Errorf("full=%v: related_editor_add ran for %q, want %q", full, got, want)
		}
		if len(*plans) != 0 {
			t.Errorf("full=%v: the pane was respawned", full)
		}
		data, _ := os.ReadFile(log)
		if !strings.Contains(string(data), "select-pane -t %7") {
			t.Errorf("full=%v: the pane did not get the keyboard:\n%s", full, data)
		}
		if zoomed := strings.Contains(string(data), "resize-pane -Z -t %7"); zoomed != full {
			t.Errorf("full=%v: zoomed %v:\n%s", full, zoomed, data)
		}
		if m.diffView.cursor != cursor || m.diffView.offset != offset {
			t.Errorf("full=%v: the diff moved", full)
		}
	}
}

// With no editor running beside Monocle one is started, as ctrl+g does in a
// tour: with the stop's related files and the file asked for.
func TestOpenEditorStartsTheRelatedEditor(t *testing.T) {
	m, added, plans, _ := openEditorApp(t, false)
	m = openEditor(t, m, openEditorMsg{path: "lib/util.go", line: 12})
	if got := readLines(t, added); len(got) != 0 {
		t.Errorf("related_editor_add ran with no editor alive: %q", got)
	}
	if len(*plans) != 1 {
		t.Fatalf("%d plans, want the editor started", len(*plans))
	}
	root := m.repoRoot
	want := []relatedFile{{filepath.Join(root, "a.go"), 5}, {filepath.Join(root, "lib/util.go"), 12}}
	if got := (*plans)[0].files; !reflect.DeepEqual(got, want) || !(*plans)[0].selectPane {
		t.Errorf("started with %+v (focus %v), want %+v with the keyboard", got, (*plans)[0].selectPane, want)
	}
}
