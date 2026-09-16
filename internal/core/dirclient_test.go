package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestDirClient_RepoRoot(t *testing.T) {
	d := NewDirClient("/tmp/test", nil)
	if d.RepoRoot() != "/tmp/test" {
		t.Errorf("RepoRoot() = %q, want %q", d.RepoRoot(), "/tmp/test")
	}
}

func TestDirClient_CurrentRef(t *testing.T) {
	d := NewDirClient("/tmp/test", nil)
	ref, err := d.CurrentRef()
	if err != nil {
		t.Fatalf("CurrentRef() error: %v", err)
	}
	if ref != "WORKING" {
		t.Errorf("CurrentRef() = %q, want %q", ref, "WORKING")
	}
}

func TestDirClient_RecentCommits(t *testing.T) {
	d := NewDirClient("/tmp/test", nil)
	entries, err := d.RecentCommits(10)
	if err != nil {
		t.Fatalf("RecentCommits() error: %v", err)
	}
	if entries != nil {
		t.Errorf("RecentCommits() = %v, want nil", entries)
	}
}

func TestDirClient_ResolveRef(t *testing.T) {
	d := NewDirClient("/tmp/test", nil)
	_, err := d.ResolveRef("HEAD")
	if err == nil {
		t.Fatal("ResolveRef() expected error, got nil")
	}
}

func TestDirClient_Diff(t *testing.T) {
	dir := t.TempDir()

	// Create some files
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "world.go"), []byte("package main"), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("Diff() returned %d files, want 2", len(files))
	}

	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
		if f.Status != types.FileNone {
			t.Errorf("file %q status = %q, want %q", f.Path, f.Status, types.FileNone)
		}
	}
	if !paths["hello.txt"] {
		t.Error("missing hello.txt")
	}
	if !paths[filepath.Join("sub", "world.go")] {
		t.Errorf("missing sub/world.go")
	}
}

func TestDirClient_Diff_SkipsHidden(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Diff() returned %d files, want 1", len(files))
	}
	if files[0].Path != "visible.txt" {
		t.Errorf("got %q, want %q", files[0].Path, "visible.txt")
	}
}

func TestDirClient_Diff_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "index.js"), []byte("ok"), 0644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("dep"), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Diff() returned %d files, want 1", len(files))
	}
	if files[0].Path != "index.js" {
		t.Errorf("got %q, want %q", files[0].Path, "index.js")
	}
}

func TestDirClient_Diff_SkipsBinary(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "text.txt"), []byte("hello"), 0644)
	// A real blob: control bytes throughout, not one stray byte in a text file.
	os.WriteFile(filepath.Join(dir, "binary.bin"),
		append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("\x00\x00\x00\rIHDR\x00\x00\x01\x90"), 40)...), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Diff() returned %d files, want 1", len(files))
	}
	if files[0].Path != "text.txt" {
		t.Errorf("got %q, want %q", files[0].Path, "text.txt")
	}
}

// A source file can carry a raw control byte — usually the very thing the
// reviewer needs to see. Skipping it as "binary" removes the file from the
// review entirely, which is how the defect stayed invisible.
func TestDirClient_ShowsSourceFileWithStrayNUL(t *testing.T) {
	dir := t.TempDir()
	const src = "const replaced = new Set(staged.map((s) => `${s.document_kind}\x00${s.person_label ?? ''}`))\n"
	os.WriteFile(filepath.Join(dir, "staging.ts"), []byte(src), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "staging.ts" {
		t.Fatalf("Diff() = %+v, want the TypeScript file listed", files)
	}
	if _, err := d.FileContent("", "staging.ts"); err != nil {
		t.Errorf("FileContent() refused a text file: %v", err)
	}
}

func TestDirClient_Diff_RespectsIgnorePatterns(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("ok"), 0644)
	os.MkdirAll(filepath.Join(dir, "vendor"), 0755)
	os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte("dep"), 0644)

	d := NewDirClient(dir, []string{"vendor/"})
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Diff() returned %d files, want 1", len(files))
	}
	if files[0].Path != "keep.go" {
		t.Errorf("got %q, want %q", files[0].Path, "keep.go")
	}
}

func TestDirClient_Diff_IncludesNonTextFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("fakepng"), 0644)

	d := NewDirClient(dir, nil)
	files, err := d.Diff("")
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}

	// Non-text files by extension should still appear in the listing
	if len(files) != 2 {
		t.Fatalf("Diff() returned %d files, want 2", len(files))
	}
}

func TestDirClient_FileContent_NonText(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.pdf"), []byte("%PDF-1.4 fake"), 0644)

	d := NewDirClient(dir, nil)
	_, err := d.FileContent("", "doc.pdf")
	if err == nil {
		t.Fatal("FileContent() expected error for PDF, got nil")
	}
}

func TestDirClient_FileContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content here"), 0644)

	d := NewDirClient(dir, nil)
	content, err := d.FileContent("", "test.txt")
	if err != nil {
		t.Fatalf("FileContent() error: %v", err)
	}
	if content != "content here" {
		t.Errorf("FileContent() = %q, want %q", content, "content here")
	}
}

func TestDirClient_FileDiff(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nline2\n"), 0644)

	d := NewDirClient(dir, nil)
	result, err := d.FileDiff("", "test.txt", 3)
	if err != nil {
		t.Fatalf("FileDiff() error: %v", err)
	}
	if result == nil {
		t.Fatal("FileDiff() returned nil")
	}
	if result.Path != "test.txt" {
		t.Errorf("Path = %q, want %q", result.Path, "test.txt")
	}
	if len(result.Hunks) != 1 {
		t.Fatalf("Hunks count = %d, want 1", len(result.Hunks))
	}
	if len(result.Hunks[0].Lines) != 2 {
		t.Errorf("Lines count = %d, want 2", len(result.Hunks[0].Lines))
	}
}
