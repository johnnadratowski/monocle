package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFilePathInLine(t *testing.T) {
	repo := t.TempDir()
	// repo/pkg/util.go and repo/main.go
	mustWrite(t, filepath.Join(repo, "pkg", "util.go"), "package pkg\n")
	mustWrite(t, filepath.Join(repo, "main.go"), "package main\n")
	baseDir := filepath.Join(repo, "pkg") // pretend the viewed file is pkg/handler.go

	cases := []struct {
		name     string
		line     string
		wantRel  string // expected resolved path relative to repo ("" = not found)
		wantLine int
	}{
		{"repo-relative import", `import "pkg/util.go"`, "pkg/util.go", 0},
		{"basedir-relative", `see util.go for details`, "pkg/util.go", 0},
		{"stack frame with line", `at main.go:42 in handler`, "main.go", 42},
		{"line and col", `main.go:42:7: undefined`, "main.go", 42},
		{"parenthesized", `(pkg/util.go)`, "pkg/util.go", 0},
		{"no path present", `just some prose here`, "", 0},
		{"nonexistent path", `import "pkg/missing.go"`, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, line, ok := findFilePathInLine(tc.line, repo, baseDir)
			if tc.wantRel == "" {
				if ok {
					t.Fatalf("expected no match, got %q", got)
				}
				return
			}
			if !ok {
				t.Fatalf("expected a match for %q", tc.line)
			}
			want, _ := filepath.Abs(filepath.Join(repo, tc.wantRel))
			if got != want {
				t.Errorf("path = %q, want %q", got, want)
			}
			if line != tc.wantLine {
				t.Errorf("line = %d, want %d", line, tc.wantLine)
			}
		})
	}
}

func TestFindFilePathInLine_IgnoresDirectories(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "internal", "x.go"), "x")
	// "internal" is a directory, not a regular file — must not match.
	if _, _, ok := findFilePathInLine("the internal package", repo, repo); ok {
		t.Error("a directory should not be treated as an openable file")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
