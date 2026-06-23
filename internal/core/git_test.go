package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func setupTestRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Isolate from the parent repo's worktree environment
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_DIR="+filepath.Join(dir, ".git"),
			"GIT_WORK_TREE="+dir,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")

	// Create initial file and commit
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc hello() {}\n"), 0o644)
	run("add", "hello.go")
	run("commit", "-m", "initial")

	// Get the base ref
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_DIR="+filepath.Join(dir, ".git"),
		"GIT_WORK_TREE="+dir,
	)
	out, _ := cmd.Output()
	baseRef := string(out[:len(out)-1])

	// Make changes
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc hello() {\n\tprintln(\"hello\")\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "world.go"), []byte("package main\n\nfunc world() {}\n"), 0o644)
	run("add", "world.go")
	// Create an untracked file (not staged)
	os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package main\n\nfunc untracked() {}\n"), 0o644)

	return dir, baseRef
}

func TestGitDiff(t *testing.T) {
	dir, baseRef := setupTestRepo(t)
	g := NewGitClient(dir)

	files, err := g.Diff(baseRef)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(files), files)
	}

	found := map[string]types.FileChangeStatus{}
	for _, f := range files {
		found[f.Path] = f.Status
	}

	if found["hello.go"] != types.FileModified {
		t.Errorf("hello.go: expected modified, got %q", found["hello.go"])
	}
	if found["world.go"] != types.FileAdded {
		t.Errorf("world.go: expected added, got %q", found["world.go"])
	}
	if found["untracked.go"] != types.FileAdded {
		t.Errorf("untracked.go: expected added (untracked), got %q", found["untracked.go"])
	}
}

func TestGitFileDiff(t *testing.T) {
	dir, baseRef := setupTestRepo(t)
	g := NewGitClient(dir)

	result, err := g.FileDiff(baseRef, "hello.go", 0)
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}

	if len(result.Hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	hunk := result.Hunks[0]
	if len(hunk.Lines) == 0 {
		t.Fatal("expected lines in hunk")
	}

	// Verify we have both added and context lines
	hasAdded := false
	for _, l := range hunk.Lines {
		if l.Kind == types.DiffLineAdded {
			hasAdded = true
		}
	}
	if !hasAdded {
		t.Error("expected added lines in diff")
	}
}

func TestGitFileDiffFullContext(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_DIR="+filepath.Join(dir, ".git"), "GIT_WORK_TREE="+dir,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")

	// A file with 20 lines; change only line 15 so the early lines fall well
	// outside the default 3-line context window.
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line"+itoa(i))
	}
	write := func(content []string) {
		os.WriteFile(filepath.Join(dir, "big.txt"), []byte(joinLines(content)), 0o644)
	}
	write(lines)
	run("add", "big.txt")
	run("commit", "-m", "initial")

	changed := append([]string(nil), lines...)
	changed[14] = "line15-changed"
	write(changed)

	g := NewGitClient(dir)

	// Compact diff (default context) must NOT include the first line.
	compact, err := g.FileDiff("HEAD", "big.txt", 0)
	if err != nil {
		t.Fatalf("compact FileDiff: %v", err)
	}
	if diffHasContextLine(compact, "line1") {
		t.Fatal("compact diff unexpectedly included line1")
	}

	// Full-file diff (negative context) must include the whole file as context.
	full, err := g.FileDiff("HEAD", "big.txt", -1)
	if err != nil {
		t.Fatalf("full FileDiff: %v", err)
	}
	if !diffHasContextLine(full, "line1") {
		t.Fatal("full diff should include line1 as context")
	}
	if !diffHasContextLine(full, "line20") {
		t.Fatal("full diff should include line20 as context")
	}
}

func diffHasContextLine(r *types.DiffResult, content string) bool {
	for _, h := range r.Hunks {
		for _, l := range h.Lines {
			if l.Kind == types.DiffLineContext && l.Content == content {
				return true
			}
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func joinLines(lines []string) string {
	var s string
	for _, l := range lines {
		s += l + "\n"
	}
	return s
}

func TestGitFileDiffUntracked(t *testing.T) {
	dir, baseRef := setupTestRepo(t)
	g := NewGitClient(dir)

	result, err := g.FileDiff(baseRef, "untracked.go", 0)
	if err != nil {
		t.Fatalf("FileDiff untracked: %v", err)
	}

	if len(result.Hunks) == 0 {
		t.Fatal("expected at least one hunk for untracked file")
	}

	hunk := result.Hunks[0]
	if hunk.OldStart != 0 || hunk.OldCount != 0 {
		t.Errorf("expected old range 0,0 for new file, got %d,%d", hunk.OldStart, hunk.OldCount)
	}
	if hunk.NewStart != 1 {
		t.Errorf("expected new start 1, got %d", hunk.NewStart)
	}

	// All lines should be added
	for _, l := range hunk.Lines {
		if l.Kind != types.DiffLineAdded {
			t.Errorf("expected all lines added, got kind %v for %q", l.Kind, l.Content)
		}
	}

	// Should contain the file content
	if len(hunk.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(hunk.Lines))
	}
}

func TestGitCurrentRef(t *testing.T) {
	dir, _ := setupTestRepo(t)
	g := NewGitClient(dir)

	ref, err := g.CurrentRef()
	if err != nil {
		t.Fatalf("CurrentRef: %v", err)
	}
	if len(ref) != 40 {
		t.Errorf("expected 40-char hash, got %q", ref)
	}
}

func TestParseDiff(t *testing.T) {
	raw := `diff --git a/hello.go b/hello.go
index abc..def 100644
--- a/hello.go
+++ b/hello.go
@@ -1,3 +1,5 @@ package main
 package main

 func hello() {
+	println("hello")
+}
`
	hunks := parseDiff(raw)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	h := hunks[0]
	if h.OldStart != 1 || h.OldCount != 3 || h.NewStart != 1 || h.NewCount != 5 {
		t.Errorf("hunk header: old=%d,%d new=%d,%d", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
}
