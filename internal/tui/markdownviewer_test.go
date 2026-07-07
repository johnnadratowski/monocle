package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestIsMarkdownPath(t *testing.T) {
	cases := map[string]bool{
		"README.md":           true,
		"docs/guide.markdown": true,
		"notes.MD":            true, // case-insensitive
		"a.mdown":             true,
		"a.mkd":               true,
		"main.go":             false,
		"content.txt":         false,
		"noext":               false,
	}
	for path, want := range cases {
		if got := isMarkdownPath(path); got != want {
			t.Errorf("isMarkdownPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestResolveMarkdownViewer(t *testing.T) {
	// Empty falls back to glow's pager (glow -p).
	if name, args := resolveMarkdownViewer(""); name != "glow" || len(args) != 1 || args[0] != "-p" {
		t.Errorf("empty config: got %q %v, want glow -p", name, args)
	}
	// Whitespace-only also falls back.
	if name, args := resolveMarkdownViewer("   "); name != "glow" || len(args) != 1 {
		t.Errorf("whitespace config: got %q %v, want glow -p", name, args)
	}
	// Configured GUI launcher splits into name + args.
	name, args := resolveMarkdownViewer("open -a MacDown")
	if name != "open" || len(args) != 2 || args[0] != "-a" || args[1] != "MacDown" {
		t.Errorf("configured launcher: got %q %v", name, args)
	}
	// A quoted app name with a space stays a single argument.
	name, args = resolveMarkdownViewer(`open -a "Google Chrome"`)
	if name != "open" || len(args) != 2 || args[1] != "Google Chrome" {
		t.Errorf("quoted launcher: got %q %v", name, args)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Implementation Plan":   "Implementation Plan",
		"feat/add-thing":        "feat-add-thing",
		"  spaced  ":            "spaced",
		"weird:*?<>|chars":      "weird------chars",
		"":                      "artifact",
		"---":                   "artifact",
		strings.Repeat("x", 80): strings.Repeat("x", 60),
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReapOldArtifactTemps(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.md")
	fresh := filepath.Join(dir, "fresh.md")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Backdate the "old" file well past the max age.
	past := now.Add(-2 * artifactTempMaxAge)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	reapOldArtifactTemps(dir, artifactTempMaxAge, now)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old temp should have been reaped, stat err = %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh temp should survive, stat err = %v", err)
	}
}

// TestViewerTarget_MarkdownFile: a selected .md file in the sidebar resolves to
// its on-disk path as a markdown target; a non-markdown/non-media file is rejected.
func TestViewerTarget_MarkdownFile(t *testing.T) {
	m := appModel{repoRoot: "/repo", focus: focusSidebar}
	m.sidebar.files = []types.ChangedFile{{Path: "docs/guide.md"}, {Path: "main.go"}}
	m.sidebar.rebuildGroups()

	m.sidebar.cursor = 0 // docs/guide.md
	path, kind, ok, why := m.viewerTarget()
	if !ok || path != "/repo/docs/guide.md" || kind != viewerMarkdown {
		t.Errorf("markdown file: got path=%q kind=%v ok=%v why=%q", path, kind, ok, why)
	}

	m.sidebar.cursor = 1 // main.go
	if _, _, ok, why := m.viewerTarget(); ok || why != "not a markdown or media file" {
		t.Errorf("non-markdown file: ok=%v why=%q", ok, why)
	}
}

// TestViewerTarget_MediaFile: a selected image file routes to the media viewer.
func TestViewerTarget_MediaFile(t *testing.T) {
	m := appModel{repoRoot: "/repo", focus: focusSidebar}
	m.sidebar.files = []types.ChangedFile{{Path: "assets/logo.png"}}
	m.sidebar.rebuildGroups()

	path, kind, ok, why := m.viewerTarget()
	if !ok || path != "/repo/assets/logo.png" || kind != viewerMedia {
		t.Errorf("media file: got path=%q kind=%v ok=%v why=%q", path, kind, ok, why)
	}
}

// TestViewerTarget_Artifact: a text artifact writes its body to a temp .md file
// for the markdown viewer; a media artifact resolves to its stored path directly.
func TestViewerTarget_Artifact(t *testing.T) {
	eng := &stubEngine{contentItems: []types.ContentItem{
		{ID: "plan-1", Content: "# Plan\n"},
		{ID: "shot-1", MediaPath: "/store/shot.png", MediaType: "image"},
	}}
	m := appModel{repoRoot: "/repo", focus: focusMain, engine: eng}

	m.diffView.contentID = "plan-1"
	m.diffView.path = "content.md"
	path, kind, ok, why := m.viewerTarget()
	if !ok || kind != viewerMarkdown {
		t.Fatalf("text artifact: kind=%v ok=%v why=%q", kind, ok, why)
	}
	defer os.Remove(path)
	if got, err := os.ReadFile(path); err != nil || string(got) != "# Plan\n" {
		t.Errorf("artifact temp body = %q (err %v)", string(got), err)
	}

	m.diffView.contentID = "shot-1"
	path, kind, ok, why = m.viewerTarget()
	if !ok || kind != viewerMedia || path != "/store/shot.png" {
		t.Errorf("media artifact: got path=%q kind=%v ok=%v why=%q", path, kind, ok, why)
	}
}

func TestWriteArtifactTempMarkdown(t *testing.T) {
	body := "# Title\n\nSome **markdown** body.\n"
	path, err := writeArtifactTempMarkdown("My Plan", body)
	if err != nil {
		t.Fatalf("writeArtifactTempMarkdown: %v", err)
	}
	defer os.Remove(path)

	if filepath.Base(path) != "My Plan.md" {
		t.Errorf("temp file should be named after the title, got %q", filepath.Base(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(got) != body {
		t.Errorf("temp body = %q, want %q", string(got), body)
	}
}
