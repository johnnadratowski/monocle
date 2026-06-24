package tui

import (
	"strings"
	"testing"
)

// TestHighlightLineNoSpuriousNewline guards against syntax highlighting emitting
// a trailing newline (some chroma lexers include one in the final token via
// EnsureNL). A diff line must render as exactly one visual row, or the row count
// desyncs from screenLinesFor — a blank line appears under the row and content
// runs off the viewport. TypeScript comments are the canonical trigger.
func TestHighlightLineNoSpuriousNewline(t *testing.T) {
	h := newHighlighter()
	cases := []struct{ name, path, content string }{
		{"ts line comment", "main.ts", "// a comment here"},
		{"tsx line comment", "App.tsx", "// note"},
		{"ts code", "main.ts", "const x = 1"},
		{"go line comment", "main.go", "// a comment here"},
		{"js comment", "app.js", "// hi"},
		{"empty", "main.ts", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := h.highlightLine(c.path, c.content, nil, nil, nil, 40)
			if strings.Contains(out, "\n") {
				t.Errorf("highlightLine(%q) emitted a newline: %q", c.content, out)
			}
		})
	}
}
