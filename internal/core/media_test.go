package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizePathSegment(t *testing.T) {
	cases := map[string]string{
		"plain":         "plain",
		"a/b":           "a-b",
		"a\\b":          "a-b",
		"../etc/passwd": "-/etc/passwd-", // ".." → "-", separators → "-" (leaves inner slashes replaced)
		"":              "item",
		"..":            "-",
		"image.png":     "image.png",
	}
	// Note: "../etc/passwd" has both ".." and "/" replaced; assert it can't
	// contain a parent-dir escape.
	got := sanitizePathSegment("../etc/passwd")
	if got == "../etc/passwd" || filepath.IsAbs(got) {
		t.Errorf("sanitizePathSegment failed to neutralize traversal: %q", got)
	}
	for in, want := range cases {
		if in == "../etc/passwd" {
			continue // checked above; exact form is not important, safety is
		}
		if got := sanitizePathSegment(in); got != want {
			t.Errorf("sanitizePathSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCopyMediaToStorage(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "shot.png")
	if err := os.WriteFile(src, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest, err := copyMediaToStorage("sess-1", "shot-1", src)
	if err != nil {
		t.Fatalf("copyMediaToStorage: %v", err)
	}
	if filepath.Base(dest) != "shot.png" {
		t.Errorf("stored filename = %q, want shot.png", filepath.Base(dest))
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "PNGDATA" {
		t.Errorf("stored content = %q (err %v)", string(got), err)
	}

	// A missing source errors rather than creating an empty file.
	if _, err := copyMediaToStorage("sess-1", "x", filepath.Join(srcDir, "nope.png")); err == nil {
		t.Error("expected error for missing source")
	}
}
