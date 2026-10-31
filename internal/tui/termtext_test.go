package tui

import (
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// bidiControls are the Unicode bidi controls (the Bidi_Control property):
// they reorder what a terminal draws, so a line can read other than it is
// (Trojan Source, CVE-2021-42574).
var bidiControls = []rune{0x061C, 0x200E, 0x200F, 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2066, 0x2067, 0x2068, 0x2069}

// hostile is what a change's file name or text can carry: a CSI (clear the
// screen), an OSC 52 (set the clipboard) ended by a BEL, a raw BEL, a C1 CSI,
// a DEL, and every bidi control.
var hostile = "\x1b[2J\x1b]52;c;ZXZpbA==\x07\x07\u009b\x7f" + string(bidiControls)

// assertTerminalSafe fails when out holds anything a terminal would obey
// besides the TUI's own SGR styling and line breaks.
func assertTerminalSafe(t *testing.T, what, out string) {
	t.Helper()
	rest := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")
	for i, r := range rest {
		if (r != '\n' && unicode.Is(unicode.Cc, r)) || unicode.Is(unicode.Bidi_Control, r) {
			t.Fatalf("%s: %U at byte %d:\n%q", what, r, i, rest)
		}
	}
}

func TestTermTextStripsBidiControls(t *testing.T) {
	for _, r := range bidiControls {
		if got := termText("a" + string(r) + "b"); got != "ab" {
			t.Errorf("termText(a %U b) = %q, want ab", r, got)
		}
	}
	// What is not a control stays: a zero-width joiner holds an emoji together.
	if got := termText("héllo 世界 👩\u200d💻"); got != "héllo 世界 👩\u200d💻" {
		t.Errorf("termText changed printable text: %q", got)
	}
}

// The card: a bidi control in any field goes too (the control bytes are
// TestTheCardDrawsNoControlBytes's).
func TestTheCardDrawsNoBidiControls(t *testing.T) {
	c := generatedCard{
		path: "api/x" + hostile + ".gen.ts", reason: types.GeneratedHeader, own: true, new: true,
		head: "// a" + hostile + "b\n// c" + hostile + "d", note: "n" + hostile + "o",
		by: "g" + hostile + "t", from: "s" + hostile + ".yaml",
		changes: []types.PackageChange{{Name: "p" + hostile + "k", From: "1" + hostile, To: "2" + hostile}},
		adds:    1, unfold: "Z", review: "r",
	}
	assertTerminalSafe(t, "the card", c.render(200, 60))
}

// The file list draws the change's paths: a file's name (flat, grouped and
// tree rows, selected or not), a directory's, an added file's and an
// artifact's title carry nothing a terminal obeys, and keep their text.
func TestTheFileListDrawsNoControlBytes(t *testing.T) {
	m, _ := generatedApp(t)
	sb := m.sidebar
	sb.reviewTracking = true
	sb.width = 120
	evil := types.ChangedFile{Path: "src" + hostile + "/ev" + hostile + "il.go", Status: types.FileModified}
	gen := types.ChangedFile{Path: "api/g" + hostile + "en.gen.ts", Status: types.FileAdded, Generated: types.GeneratedName}
	dir := &fileTreeNode{Name: "src" + hostile, Path: "src" + hostile}
	leaf := &fileTreeNode{Name: "ev" + hostile + "il.go", Path: evil.Path, File: &evil}
	for _, selected := range []bool{false, true} {
		rows := map[string]string{
			"file row":           sb.renderFileItem(evil, selected),
			"generated file row": sb.renderFileItem(gen, selected),
			"tree file row":      sb.renderTreeFileItem(visibleItem{node: leaf, depth: 1}, selected),
			"tree dir row":       sb.renderDirItem(visibleItem{node: dir, isDir: true}, selected),
			"added file row":     sb.renderAdditionalFileItem(types.AdditionalFile{Name: "x" + hostile + ".md", Path: "/x.md"}, selected),
			"artifact row":       sb.renderContentItem(types.ContentItem{ID: "a", Title: "t" + hostile + "itle"}, selected),
		}
		for what, row := range rows {
			assertTerminalSafe(t, what, row)
		}
		for what, want := range map[string]string{"file row": "evil.go", "tree file row": "evil.go", "tree dir row": "src",
			"added file row": "x.md", "artifact row": "title"} {
			if got := ansi.Strip(rows[what]); !strings.Contains(got, want) {
				t.Errorf("%s %q lost its text %q", what, got, want)
			}
		}
	}
}
