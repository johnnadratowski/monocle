package tui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// ansiCodes extracts the SGR sequences from a rendered string, which is what
// "is this syntax highlighted" reduces to once the text is stripped away.
var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func codesIn(s string) []string { return ansiCodes.FindAllString(s, -1) }

func highlightModel(t *testing.T, wrap bool, width int) diffViewModel {
	t.Helper()
	theme := DefaultTheme()
	m := diffViewModel{
		path: "x.go", theme: &theme, hl: newHighlighter(),
		mdStyler: newMarkdownStyler(theme),
		wrap:     wrap, width: width, height: 40, tabSize: 4,
	}
	return m
}

// The reported bug: each wrapped chunk was highlighted on its own, so chroma
// was handed a fragment it could not tokenise — a piece starting inside a
// string, a comment or an identifier — and the continuation rows came out
// mis-coloured or plain.
func TestWrappedLinesKeepSyntaxHighlighting(t *testing.T) {
	const code = `func handle(ctx context.Context, req *Request) error { return fmt.Errorf("bad request: %w", err) }`

	t.Run("continuation rows are styled, not plain", func(t *testing.T) {
		m := highlightModel(t, true, 60)
		line := diffViewLine{content: code, newLineNum: 1}
		out := m.renderWrappedLine("  1 ", code, 4, 40, nil, nil, false, &line)
		rows := strings.Split(out, "\n")
		if len(rows) < 2 {
			t.Fatalf("expected the line to wrap, got %d row(s)", len(rows))
		}
		for i, r := range rows[1:] {
			if len(codesIn(r)) <= 1 {
				// 1 code would be just the gutter's colour.
				t.Errorf("continuation row %d carries no syntax colour: %q", i+1, stripANSISeq(r))
			}
		}
	})

	// The strongest statement of correctness: wrapping must not change what
	// colour anything is, only where the rows break.
	t.Run("wrapped and unwrapped colour the same text identically", func(t *testing.T) {
		m := highlightModel(t, true, 60)
		line := diffViewLine{content: code, newLineNum: 1}

		wrapped := m.renderWrappedLine("  1 ", code, 4, 40, nil, nil, false, &line)
		wide := m.hl.highlightLine("x.go", code, nil, nil, nil, 0)

		gotCodes := codesIn(strings.ReplaceAll(wrapped, "\n", ""))
		wantCodes := codesIn(wide)
		// The wrapped render adds gutter and padding codes, so compare the set
		// of distinct colours rather than the exact sequence.
		distinct := func(in []string) map[string]bool {
			out := map[string]bool{}
			for _, c := range in {
				out[c] = true
			}
			return out
		}
		got, want := distinct(gotCodes), distinct(wantCodes)
		var missing []string
		for c := range want {
			if !got[c] {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			t.Errorf("wrapping dropped %d colour(s) the unwrapped line has: %q", len(missing), missing)
		}
	})

	t.Run("the visible text survives wrapping unchanged", func(t *testing.T) {
		m := highlightModel(t, true, 60)
		line := diffViewLine{content: code, newLineNum: 1}
		out := m.renderWrappedLine("  1 ", code, 4, 40, nil, nil, false, &line)
		var joined strings.Builder
		for _, r := range strings.Split(stripANSISeq(out), "\n") {
			joined.WriteString(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r), "1")))
		}
		flat := strings.ReplaceAll(joined.String(), " ", "")
		if !strings.Contains(flat, strings.ReplaceAll("fmt.Errorf", " ", "")) {
			t.Errorf("wrapped text lost content: %q", joined.String())
		}
	})
}

func TestWrappedSplitKeepsSyntaxHighlighting(t *testing.T) {
	const code = `func handle(ctx context.Context, req *Request) error { return fmt.Errorf("bad: %w", err) }`
	m := highlightModel(t, true, 96)
	m.style = diffStyleSplit
	m.lines = []diffViewLine{{
		isSplit: true, kind: types.DiffLineContext, oldLineNum: 1, content: code,
		rightKind: types.DiffLineContext, rightLineNum: 1, rightContent: code,
	}}

	rows := strings.Split(m.View(), "\n")
	if len(rows) < 2 {
		t.Fatalf("expected the split line to wrap, got %d row(s)", len(rows))
	}
	for i, r := range rows[1:] {
		if len(codesIn(r)) <= 2 { // two gutters' worth at most
			t.Errorf("split continuation row %d carries no syntax colour: %q", i+1, stripANSISeq(r))
		}
	}
}

// The first fix compared wrapped colouring against a plain highlight of the
// same text, which misses everything the DIFF adds on top: the line background
// and the brighter intra-line change spans. Comparing a wrapped render against
// an unwrapped render of the same line is the assertion that actually holds
// "wrapping changes where rows break and nothing else".
func TestWrappingPreservesEveryDiffColour(t *testing.T) {
	const oldC = `func handle(ctx context.Context, req *Request) error { return fmt.Errorf("bad: %w", err) }`
	const newC = `func handle(ctx context.Context, req *Response) error { return fmt.Errorf("BAD: %w", err) }`

	colourSet := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, c := range codesIn(s) {
			out[c] = true
		}
		return out
	}

	build := func(style diffStyle, wrap bool, width int) diffViewModel {
		theme := DefaultTheme()
		line := diffViewLine{
			isSplit: true, kind: types.DiffLineRemoved, oldLineNum: 1, content: oldC,
			rightKind: types.DiffLineAdded, rightLineNum: 1, rightContent: newC,
		}
		if style != diffStyleSplit {
			line = diffViewLine{
				kind: types.DiffLineAdded, newLineNum: 1, content: newC, pairContent: oldC,
			}
		}
		return diffViewModel{
			path: "x.go", theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
			wrap: wrap, width: width, height: 40, tabSize: 4, style: style,
			lines: []diffViewLine{line},
		}
	}

	for _, tc := range []struct {
		name  string
		style diffStyle
	}{
		{"split", diffStyleSplit},
		{"unified", diffStyleUnified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unwrapped := colourSet(build(tc.style, false, 220).View())
			wrapped := colourSet(build(tc.style, true, 96).View())

			var missing []string
			for c := range unwrapped {
				if !wrapped[c] {
					missing = append(missing, strings.ReplaceAll(c, "\x1b", "ESC"))
				}
			}
			if len(missing) > 0 {
				t.Errorf("wrapping dropped %d colour(s) the unwrapped line has: %q", len(missing), missing)
			}
		})
	}
}

// The specific loss that prompted this: the brighter background marking the
// words that actually changed disappeared once the line wrapped.
func TestWrappedLinesKeepIntraLineChangeHighlight(t *testing.T) {
	theme := DefaultTheme()
	const oldC = `result := computeTheValue(alpha, beta, gamma) // an old trailing comment here`
	const newC = `result := computeTheValue(alpha, DELTA, gamma) // an old trailing comment here`

	m := diffViewModel{
		path: "x.go", theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		wrap: true, width: 50, height: 40, tabSize: 4,
		lines: []diffViewLine{{
			kind: types.DiffLineAdded, newLineNum: 1, content: newC, pairContent: oldC,
		}},
	}
	out := m.View()
	if n := len(strings.Split(out, "\n")); n < 2 {
		t.Fatalf("expected the line to wrap, got %d row(s)", n)
	}
	changeBg, ok := colorToRGBSeq(theme.AddedChangeBg)
	if !ok {
		t.Skip("theme's change background is not an RGB colour")
	}
	if !strings.Contains(out, changeBg) {
		t.Errorf("the changed word lost its background once wrapped; expected %q in the render", changeBg)
	}
}

// colorToRGBSeq renders a colour as the SGR background sequence lipgloss emits
// for it, so a test can look for that exact background in the output.
func colorToRGBSeq(c color.Color) (string, bool) {
	if c == nil {
		return "", false
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8), true
}
