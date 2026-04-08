package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/adapters"
)

// --- resolveRepoRoot tests ---

func TestResolveRepoRoot_WithGitRepo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	repoRoot, nonGitMode, err := resolveRepoRoot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoRoot != dir {
		t.Errorf("repoRoot = %s, want %s", repoRoot, dir)
	}
	if nonGitMode {
		t.Error("nonGitMode = true, want false")
	}
}

func TestResolveRepoRoot_FromSubdirectory(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	sub := filepath.Join(dir, "src", "pkg")
	os.MkdirAll(sub, 0755)

	repoRoot, _, err := resolveRepoRoot(sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoRoot != dir {
		t.Errorf("repoRoot = %s, want %s", repoRoot, dir)
	}
}

func TestResolveRepoRoot_NonGitDir(t *testing.T) {
	dir := t.TempDir()

	repoRoot, nonGitMode, err := resolveRepoRoot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoRoot != dir {
		t.Errorf("repoRoot = %s, want %s", repoRoot, dir)
	}
	if !nonGitMode {
		t.Error("nonGitMode = false, want true")
	}
}

func TestResolveRepoRoot_NonexistentDir(t *testing.T) {
	_, _, err := resolveRepoRoot("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent dir, got nil")
	}
	if !strings.Contains(err.Error(), "--workdir") {
		t.Errorf("error should mention --workdir, got: %v", err)
	}
}

func TestResolveRepoRoot_FileNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	os.WriteFile(f, []byte("hello"), 0644)

	_, _, err := resolveRepoRoot(f)
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention 'not a directory', got: %v", err)
	}
}

func TestResolveRepoRoot_EmptyWorkdir_UsesCWD(t *testing.T) {
	// Empty string should fall back to CWD — just verify no error
	repoRoot, _, err := resolveRepoRoot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoRoot == "" {
		t.Error("repoRoot should not be empty")
	}
}

// --- resolveSocketForWorkDir tests ---

func TestResolveSocketForWorkDir_SocketOverrideTakesPrecedence(t *testing.T) {
	got, err := resolveSocketForWorkDir("/tmp/custom.sock", "/some/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/custom.sock" {
		t.Errorf("got %s, want /tmp/custom.sock", got)
	}
}

func TestResolveSocketForWorkDir_WorkdirDerived(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	got, err := resolveSocketForWorkDir("", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := adapters.DefaultSocketPath(dir)
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveSocketForWorkDir_WorkdirSubdir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0755)

	got, err := resolveSocketForWorkDir("", sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to the repo root's socket, not the subdir's
	want := adapters.DefaultSocketPath(dir)
	if got != want {
		t.Errorf("got %s, want %s (should use repo root, not subdir)", got, want)
	}
}

func TestResolveSocketForWorkDir_FallsThroughToCWD(t *testing.T) {
	got, err := resolveSocketForWorkDir("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("socket path should not be empty")
	}
	if !strings.HasPrefix(got, "/tmp/monocle-") {
		t.Errorf("expected /tmp/monocle- prefix, got %s", got)
	}
}

func TestResolveSocketForWorkDir_NonexistentWorkdir(t *testing.T) {
	_, err := resolveSocketForWorkDir("", "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent workdir, got nil")
	}
	if !strings.Contains(err.Error(), "--workdir") {
		t.Errorf("error should mention --workdir, got: %v", err)
	}
}

func TestResolveSocketForWorkDir_FileNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	os.WriteFile(f, []byte("hello"), 0644)

	_, err := resolveSocketForWorkDir("", f)
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention 'not a directory', got: %v", err)
	}
}

func TestResolveSocketForWorkDir_DeterministicForSameDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	p1, _ := resolveSocketForWorkDir("", dir)
	p2, _ := resolveSocketForWorkDir("", dir)
	if p1 != p2 {
		t.Errorf("not deterministic: %s != %s", p1, p2)
	}
}
