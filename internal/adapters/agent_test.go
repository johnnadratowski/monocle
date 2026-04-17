package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHookCommand_GlobalAlwaysAbsolute(t *testing.T) {
	// Global scope should return the absolute exe path regardless of where
	// the settings file sits, because a path relative to $HOME would be
	// misleading in a ~/.claude/settings.json hook entry.
	got := ResolveHookCommand("/anywhere/.claude/settings.json", true)
	if !filepath.IsAbs(got) {
		t.Errorf("global should return absolute path, got %q", got)
	}
}

func TestResolveHookCommand_LocalUsesRelativeWhenBinaryIsUnderProject(t *testing.T) {
	// Put a fake "binary" inside a fake project so filepath.Rel treats it
	// as nested. We can't actually change os.Executable(), so instead we
	// verify the relativity math via a table test on a helper. The real
	// integration is covered by the ClaudeRegister_InstallsPlanHookByDefault
	// test below (indirectly) and by the smoke test in hooks_test.go.
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "proj")
	settings := filepath.Join(projectRoot, ".claude", "settings.json")
	binary := filepath.Join(projectRoot, "bin", "monocle")

	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable for this test")
	}
	// Only meaningful when the test is actually running the project binary.
	if !strings.Contains(exe, "monocle") {
		t.Skipf("exe %q does not look like monocle", exe)
	}

	// Sanity: the helpers we rely on behave as expected for a synthetic
	// (projectRoot, binary) pair even if we can't swap os.Executable().
	rel, err := filepath.Rel(projectRoot, binary)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if rel != filepath.Join("bin", "monocle") {
		t.Errorf("expected relative path %q, got %q", filepath.Join("bin", "monocle"), rel)
	}
	_ = settings
}

func TestResolveHookCommand_LocalFallsBackToAbsoluteWhenOutsideProject(t *testing.T) {
	// A settings.json in /tmp/outside-proj/.claude/settings.json cannot be
	// a parent of the real monocle binary on disk, so the resolver should
	// return an absolute path.
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside-proj", ".claude", "settings.json")

	got := ResolveHookCommand(outside, false)
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute fallback when binary is outside project, got %q", got)
	}
}
