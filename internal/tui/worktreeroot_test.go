package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// linkedWorktree makes a repo with one commit and a linked worktree of it on a
// branch that adds server/new.sql, which the main checkout does not have. It
// returns both checkouts' paths, symlinks resolved.
func linkedWorktree(t *testing.T) (mainDir, wtDir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mainDir, wtDir = filepath.Join(base, "main"), filepath.Join(base, "main", ".claude", "worktrees", "feature")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(mainDir, "init", "-q", "-b", "master")
	if err := os.WriteFile(filepath.Join(mainDir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(mainDir, "add", "a.go")
	git(mainDir, "commit", "-q", "-m", "a")
	git(mainDir, "worktree", "add", "-q", "-b", "feature", wtDir)
	if err := os.MkdirAll(filepath.Join(wtDir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, "server", "new.sql"), []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainDir, wtDir
}

// A TUI started in the main clone and pointed at a worktree's engine works in
// the worktree: that is the review's checkout. Measured live (John
// 2026-10-04): the related pane ran in the main clone, where the PR's new
// files do not exist.
func TestTheEnginesCheckoutIsTheTUIsRoot(t *testing.T) {
	mainDir, wtDir := linkedWorktree(t)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileModified}}
	tour := &types.Walkthrough{Stops: []types.WalkthroughStop{{ID: "1", File: "a.go", LineStart: 1,
		Related: []types.DocRef{{Kind: types.DocRefFile, Doc: "server/new.sql", StartLine: 1}}}}}
	e := &tourEngine{
		stubEngine: stubEngine{cfg: &types.Config{Editor: "nvim"}, changedFiles: files,
			session: &types.ReviewSession{ID: "s", RepoRoot: wtDir, ChangedFiles: files, Walkthrough: tour, WalkthroughStop: "1"}},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 3)},
	}
	m := NewApp(e, AppOptions{RepoRoot: mainDir})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})
	m = updateApp(t, m, initialLoadMsg{files: files})
	plans := captureRelated(t)
	m = settle(t, m)

	if m.repoRoot != wtDir || m.diffView.repoRoot != wtDir {
		t.Fatalf("root %q (diff view %q), want the worktree %q", m.repoRoot, m.diffView.repoRoot, wtDir)
	}
	if len(*plans) != 1 {
		t.Fatalf("%d related-pane plans, want the stop's", len(*plans))
	}
	p := (*plans)[0]
	want := filepath.Join(wtDir, "server", "new.sql")
	if p.dir != wtDir || !slices.Contains(p.argv, want) {
		t.Errorf("related pane in %q opening %q; want it in the worktree, opening %s", p.dir, p.argv, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the path handed to the editor does not exist: %v", err)
	}
	// ctrl+g and the terminal split resolve against the same checkout.
	m.setFocus(focusMain)
	if path, _, ok := m.editorTargetFile(); !ok || path != filepath.Join(wtDir, "a.go") {
		t.Errorf("ctrl+g would open %q, want the worktree's a.go", path)
	}
	if dir := m.terminalTargetDir(); dir != wtDir {
		t.Errorf("a terminal would open in %q, want the worktree", dir)
	}
}
