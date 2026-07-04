package tui

import (
	"os"
	"testing"

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
	// Empty falls back to glow.
	if name, args := resolveMarkdownViewer(""); name != "glow" || len(args) != 0 {
		t.Errorf("empty config: got %q %v, want glow with no args", name, args)
	}
	// Whitespace-only also falls back.
	if name, _ := resolveMarkdownViewer("   "); name != "glow" {
		t.Errorf("whitespace config: got %q, want glow", name)
	}
	// Configured command with flags splits into name + args.
	name, args := resolveMarkdownViewer("glow -p -w 100")
	if name != "glow" || len(args) != 3 || args[0] != "-p" {
		t.Errorf("flagged config: got %q %v", name, args)
	}
}

// TestMarkdownViewerTarget_MarkdownFile: a selected .md file in the sidebar
// resolves to its on-disk path; a non-markdown file is rejected.
func TestMarkdownViewerTarget_MarkdownFile(t *testing.T) {
	m := appModel{repoRoot: "/repo", focus: focusSidebar}
	m.sidebar.files = []types.ChangedFile{{Path: "docs/guide.md"}, {Path: "main.go"}}
	m.sidebar.rebuildGroups()

	m.sidebar.cursor = 0 // docs/guide.md
	path, cleanup, ok, why := m.markdownViewerTarget()
	if !ok || path != "/repo/docs/guide.md" || cleanup != nil {
		t.Errorf("markdown file: got path=%q ok=%v why=%q", path, ok, why)
	}

	m.sidebar.cursor = 1 // main.go
	if _, _, ok, why := m.markdownViewerTarget(); ok || why != "not a markdown file" {
		t.Errorf("non-markdown file: ok=%v why=%q", ok, why)
	}
}

// TestMarkdownViewerTarget_Artifact: viewing an artifact writes its body to a
// temp .md file and returns a cleanup func, since artifacts have no on-disk path.
func TestMarkdownViewerTarget_Artifact(t *testing.T) {
	eng := &stubEngine{contentItems: []types.ContentItem{{ID: "plan-1", Content: "# Plan\n"}}}
	m := appModel{repoRoot: "/repo", focus: focusMain, engine: eng}
	m.diffView.contentID = "plan-1"
	m.diffView.path = "content.md" // synthetic content-diff path

	path, cleanup, ok, why := m.markdownViewerTarget()
	if !ok || cleanup == nil {
		t.Fatalf("artifact: ok=%v cleanup=%v why=%q", ok, cleanup != nil, why)
	}
	defer cleanup()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "# Plan\n" {
		t.Errorf("artifact temp body = %q (err %v), want %q", string(got), err, "# Plan\n")
	}
}

func TestWriteArtifactTempMarkdown(t *testing.T) {
	body := "# Title\n\nSome **markdown** body.\n"
	path, cleanup, err := writeArtifactTempMarkdown(body)
	if err != nil {
		t.Fatalf("writeArtifactTempMarkdown: %v", err)
	}
	defer cleanup()

	if got := len(path); got == 0 {
		t.Fatal("expected a temp file path")
	}
	if ext := path[len(path)-3:]; ext != ".md" {
		t.Errorf("temp file should end in .md, got %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(got) != body {
		t.Errorf("temp body = %q, want %q", string(got), body)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup should have removed the temp file, stat err = %v", err)
	}
}
