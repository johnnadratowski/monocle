package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name  string
		hunks []types.DiffHunk
		want  bool
	}{
		{
			name:  "empty hunks",
			hunks: nil,
			want:  false,
		},
		{
			name: "normal text content",
			hunks: []types.DiffHunk{{
				Lines: []types.DiffLine{
					{Content: "func main() {"},
					{Content: "\tfmt.Println(\"hello\")"},
					{Content: "}"},
				},
			}},
			want: false,
		},
		{
			name: "null byte in content",
			hunks: []types.DiffHunk{{
				Lines: []types.DiffLine{
					{Content: "hello\x00world"},
				},
			}},
			want: true,
		},
		{
			name: "control character 0x01",
			hunks: []types.DiffHunk{{
				Lines: []types.DiffLine{
					{Content: "binary\x01data"},
				},
			}},
			want: true,
		},
		{
			name: "control character 0x1f",
			hunks: []types.DiffHunk{{
				Lines: []types.DiffLine{
					{Content: "data\x1fmore"},
				},
			}},
			want: true,
		},
		{
			name: "tab and newline are not binary",
			hunks: []types.DiffHunk{{
				Lines: []types.DiffLine{
					{Content: "line\twith\ttabs"},
				},
			}},
			want: false,
		},
		{
			name: "binary in second hunk",
			hunks: []types.DiffHunk{
				{Lines: []types.DiffLine{{Content: "normal text"}}},
				{Lines: []types.DiffLine{{Content: "has\x00null"}}},
			},
			want: true,
		},
		{
			name: "binary beyond sampling limit is missed",
			hunks: []types.DiffHunk{
				{Lines: []types.DiffLine{{Content: "normal"}}},
				{Lines: []types.DiffLine{{Content: "normal"}}},
				{Lines: []types.DiffLine{{Content: "has\x00null"}}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryContent(tt.hunks)
			if got != tt.want {
				t.Errorf("isBinaryContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWrapContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		width   int
		want    []string
	}{
		{
			name:    "fits within width",
			content: "hello world",
			width:   20,
			want:    []string{"hello world"},
		},
		{
			name:    "wraps at space boundary",
			content: "hello world foo",
			width:   12,
			want:    []string{"hello world ", "foo"},
		},
		{
			name:    "long word falls back to char wrap",
			content: "abcdefghijklmnop",
			width:   5,
			want:    []string{"abcde", "fghij", "klmno", "p"},
		},
		{
			name:    "mixed word wrap and char fallback",
			content: "hi abcdefghijklmno",
			width:   10,
			want:    []string{"hi ", "abcdefghij", "klmno"},
		},
		{
			name:    "empty string",
			content: "",
			width:   10,
			want:    []string{""},
		},
		{
			name:    "width zero returns as-is",
			content: "hello",
			width:   0,
			want:    []string{"hello"},
		},
		{
			name:    "negative width returns as-is",
			content: "hello",
			width:   -1,
			want:    []string{"hello"},
		},
		{
			name:    "exactly at width",
			content: "abcde",
			width:   5,
			want:    []string{"abcde"},
		},
		{
			name:    "break at last possible space",
			content: "aaa bbb ccc",
			width:   8,
			want:    []string{"aaa bbb ", "ccc"},
		},
		{
			name:    "leading indentation preserved",
			content: "    return nil",
			width:   10,
			want:    []string{"    return ", "nil"},
		},
		{
			name:    "multiple consecutive spaces",
			content: "a  b  c",
			width:   4,
			want:    []string{"a  b ", " c"},
		},
		{
			name:    "single character width",
			content: "abc",
			width:   1,
			want:    []string{"a", "b", "c"},
		},
		{
			name:    "space at exact boundary",
			content: "abcd efgh",
			width:   5,
			want:    []string{"abcd ", "efgh"},
		},
		{
			name:    "multiple wraps at word boundaries",
			content: "the quick brown fox jumps",
			width:   10,
			want:    []string{"the quick ", "brown fox ", "jumps"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapContent(tt.content, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("wrapContent(%q, %d) returned %d chunks, want %d\ngot:  %q\nwant: %q",
					tt.content, tt.width, len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("chunk[%d] = %q, want %q\nfull got:  %q\nfull want: %q",
						i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

func TestScreenLinesForConsistency(t *testing.T) {
	m := diffViewModel{
		wrap:  true,
		width: 50,
		lines: []diffViewLine{
			{content: "short line"},
			{content: "this is a longer line that should wrap at word boundaries when displayed"},
			{content: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"},
			{content: ""},
			{content: "    indented content with some extra words to wrap around"},
		},
	}

	for i, line := range m.lines {
		cw := m.contentWidthFor(line)
		expected := len(wrapContent(line.content, cw))
		got := m.screenLinesFor(i)
		if got != expected {
			t.Errorf("line %d: screenLinesFor=%d but len(wrapContent)=%d (content=%q, width=%d)",
				i, got, expected, line.content, cw)
		}
	}
}

func TestRenderWrappedLineMarkdownContent(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme:       &theme,
		hl:          newHighlighter(),
		mdStyler:    newMarkdownStyler(theme),
		contentMode: true,
		path:        "some-plan-id", // extensionless — content mode treats as markdown
		wrap:        true,
		width:       80,
	}

	tests := []struct {
		name    string
		content string
		// wantRaw is the raw markdown marker that should NOT appear in styled output
		wantRaw string
		// wantStyled is a substring that should appear in the styled output
		wantStyled string
	}{
		{
			name:       "header is styled",
			content:    "# Hello World",
			wantRaw:    "# ",
			wantStyled: "Hello World",
		},
		{
			name:       "bullet is styled",
			content:    "- list item",
			wantRaw:    "- ",
			wantStyled: "list item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := diffViewLine{content: tt.content, newLineNum: 1}
			result := m.renderWrappedLine("1   ", tt.content, 4, 76,
				nil, nil, false, &line)
			if strings.Contains(result, tt.wantRaw) {
				t.Errorf("expected raw markdown %q to be styled away, got: %s", tt.wantRaw, result)
			}
			if !strings.Contains(result, tt.wantStyled) {
				t.Errorf("expected styled output to contain %q, got: %s", tt.wantStyled, result)
			}
		})
	}
}

func TestRenderWrappedLineMarkdownFile(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme:       &theme,
		hl:          newHighlighter(),
		mdStyler:    newMarkdownStyler(theme),
		contentMode: false,
		path:        "README.md",
		wrap:        true,
		width:       80,
	}

	line := diffViewLine{content: "# Header", newLineNum: 1}
	result := m.renderWrappedLine("1   ", "# Header", 4, 76,
		nil, nil, false, &line)

	if strings.Contains(result, "# ") {
		t.Errorf("expected markdown header to be styled, got raw: %s", result)
	}
	if !strings.Contains(result, "Header") {
		t.Errorf("expected output to contain 'Header', got: %s", result)
	}
}

func TestRenderWrappedLineNonMarkdown(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme:       &theme,
		hl:          newHighlighter(),
		mdStyler:    newMarkdownStyler(theme),
		contentMode: false,
		path:        "main.go",
		wrap:        true,
		width:       80,
	}

	// "# comment" in a Go file should NOT be styled as a markdown header
	line := diffViewLine{content: "# comment", newLineNum: 1}
	result := m.renderWrappedLine("1   ", "# comment", 4, 76,
		nil, nil, false, &line)

	// The raw content should pass through (not transformed into a styled header)
	if !strings.Contains(result, "#") {
		t.Errorf("non-markdown file should preserve raw content, got: %s", result)
	}
}

func TestExtractSuggestionCode(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
		wantOK   bool
	}{
		{
			name:     "simple suggestion",
			body:     "```suggestion\nfoo := bar()\n```",
			wantCode: "foo := bar()",
			wantOK:   true,
		},
		{
			name:     "multi-line suggestion",
			body:     "```suggestion\nline1\nline2\nline3\n```",
			wantCode: "line1\nline2\nline3",
			wantOK:   true,
		},
		{
			name:     "suggestion with surrounding text",
			body:     "Consider this change:\n```suggestion\nnewCode()\n```\nThis is better.",
			wantCode: "newCode()",
			wantOK:   true,
		},
		{
			name:   "no suggestion block",
			body:   "This is a regular comment",
			wantOK: false,
		},
		{
			name:     "empty suggestion",
			body:     "```suggestion\n\n```",
			wantCode: "",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := extractSuggestionCode(tt.body)
			if ok != tt.wantOK {
				t.Fatalf("extractSuggestionCode() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && code != tt.wantCode {
				t.Errorf("extractSuggestionCode() code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestFormatExpandedCommentSuggestionDiff(t *testing.T) {
	comment := &types.ReviewComment{
		Type: types.CommentSuggestion,
		Body: "```suggestion\nnewFunc()\n```",
	}

	// With original code: should render diff lines
	result := formatExpandedComment(comment, 80, "oldFunc()", true)
	if !strings.Contains(result, "- oldFunc()") {
		t.Errorf("expected diff to contain removed line '- oldFunc()', got:\n%s", result)
	}
	if !strings.Contains(result, "+ newFunc()") {
		t.Errorf("expected diff to contain added line '+ newFunc()', got:\n%s", result)
	}
	// Should NOT contain the raw fence markers
	if strings.Contains(result, "```suggestion") {
		t.Errorf("expected suggestion fence to be replaced by diff, got:\n%s", result)
	}

	// Without original code: should fall back to raw body
	resultNoOrig := formatExpandedComment(comment, 80, "", true)
	if !strings.Contains(resultNoOrig, "```suggestion") {
		t.Errorf("expected raw fence when no original code, got:\n%s", resultNoOrig)
	}
}

func TestFormatExpandedCommentSuggestionDiffWithSurroundingText(t *testing.T) {
	comment := &types.ReviewComment{
		Type: types.CommentSuggestion,
		Body: "Consider this:\n```suggestion\nnewFunc()\n```\nBetter approach.",
	}

	result := formatExpandedComment(comment, 80, "oldFunc()", true)
	if !strings.Contains(result, "Consider this:") {
		t.Errorf("expected text before suggestion, got:\n%s", result)
	}
	if !strings.Contains(result, "- oldFunc()") {
		t.Errorf("expected removed line, got:\n%s", result)
	}
	if !strings.Contains(result, "+ newFunc()") {
		t.Errorf("expected added line, got:\n%s", result)
	}
	if !strings.Contains(result, "Better approach.") {
		t.Errorf("expected text after suggestion, got:\n%s", result)
	}
}

func TestOriginalCodeForComment(t *testing.T) {
	m := diffViewModel{
		lines: []diffViewLine{
			{newLineNum: 1, content: "line one"},
			{newLineNum: 2, content: "line two"},
			{newLineNum: 3, content: "line three"},
			{newLineNum: 4, content: "line four"},
			{isComment: true, comment: &types.ReviewComment{ID: "c1"}},
			{newLineNum: 5, content: "line five"},
		},
	}

	comment := &types.ReviewComment{LineStart: 2, LineEnd: 4}
	got := m.originalCodeForComment(comment)
	want := "line two\nline three\nline four"
	if got != want {
		t.Errorf("originalCodeForComment() = %q, want %q", got, want)
	}

	// Single line
	comment2 := &types.ReviewComment{LineStart: 3, LineEnd: 0}
	got2 := m.originalCodeForComment(comment2)
	if got2 != "line three" {
		t.Errorf("single line: got %q, want %q", got2, "line three")
	}

	// File-level comment (LineStart=0)
	comment3 := &types.ReviewComment{LineStart: 0}
	got3 := m.originalCodeForComment(comment3)
	if got3 != "" {
		t.Errorf("file-level comment: got %q, want empty", got3)
	}
}

func TestRenderContentLineWrapModeMarkdown(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme:       &theme,
		hl:          newHighlighter(),
		mdStyler:    newMarkdownStyler(theme),
		contentMode: true,
		path:        "plan-id",
		wrap:        true,
		width:       80,
		focused:     true,
	}

	line := diffViewLine{content: "## Section Title", newLineNum: 1}
	result := m.renderContentLine(line, 0, 76, false, false)

	if strings.Contains(result, "## ") {
		t.Errorf("expected markdown header to be styled in wrap mode, got raw: %s", result)
	}
	if !strings.Contains(result, "Section Title") {
		t.Errorf("expected output to contain 'Section Title', got: %s", result)
	}
}

func TestLineNumAtSplitMode(t *testing.T) {
	m := diffViewModel{
		lines: []diffViewLine{
			// Split-mode added line: new-file number lives in rightLineNum.
			{rightLineNum: 42, newLineNum: 0, content: "added"},
			// Unified/file-mode line: new-file number lives in newLineNum.
			{rightLineNum: 0, newLineNum: 7, content: "context"},
			// Removed-only split line: neither side has a new-file number.
			{rightLineNum: 0, newLineNum: 0, content: "removed"},
		},
	}

	// Split line resolves to the right (new-file) line number, so the comment
	// editor opens against line 42 instead of silently no-opping.
	if got := m.lineNumAt(0); got != 42 {
		t.Errorf("lineNumAt(0) = %d, want 42", got)
	}
	m.cursor = 0
	if got := m.currentDiffLine(); got != 42 {
		t.Errorf("currentDiffLine() = %d, want 42", got)
	}

	// Unified line still resolves via newLineNum (regression guard).
	if got := m.lineNumAt(1); got != 7 {
		t.Errorf("lineNumAt(1) = %d, want 7", got)
	}

	// Removed-only split line stays non-commentable.
	if got := m.lineNumAt(2); got != 0 {
		t.Errorf("lineNumAt(2) = %d, want 0", got)
	}
}

// TestIsSelectableSplitInPlaceChange verifies that an in-place change in split
// mode — built as a removed line (kind == DiffLineRemoved, newLineNum == 0)
// paired with an added right side (rightLineNum > 0) — is selectable so the
// cursor can land on it and `c` opens the comment editor. A pure removed split
// line (rightLineNum == 0) must stay non-selectable.
func TestIsSelectableSplitInPlaceChange(t *testing.T) {
	m := diffViewModel{
		lines: []diffViewLine{
			// In-place split change: removed left, added right.
			{isSplit: true, kind: types.DiffLineRemoved, newLineNum: 0, rightLineNum: 11, content: "old", rightContent: "new"},
			// Pure removed split line: no right side.
			{isSplit: true, kind: types.DiffLineRemoved, newLineNum: 0, rightLineNum: 0, rightEmpty: true, content: "gone"},
			// Plain added split line.
			{isSplit: true, kind: types.DiffLineContext, leftEmpty: true, newLineNum: 0, rightLineNum: 12, rightContent: "added"},
		},
	}

	// The in-place change is selectable and resolves to its new-file line number.
	if !m.isSelectable(0) {
		t.Errorf("isSelectable(0) = false, want true for in-place split change")
	}
	if got := m.lineNumAt(0); got != 11 {
		t.Errorf("lineNumAt(0) = %d, want 11", got)
	}
	m.cursor = 0
	if got := m.currentDiffLine(); got != 11 {
		t.Errorf("currentDiffLine() = %d, want 11", got)
	}

	// A pure removed line (no right-side line number) stays non-selectable.
	if m.isSelectable(1) {
		t.Errorf("isSelectable(1) = true, want false for pure removed split line")
	}

	// nextSelectable from the in-place change skips the pure removed line and
	// lands on the next line that has a right-side number.
	if got := m.nextSelectable(0, 1); got != 2 {
		t.Errorf("nextSelectable(0, 1) = %d, want 2 (skipping pure removed line)", got)
	}
}

// TestIndexForNewLine verifies the line lookup used to re-anchor the viewport
// after a diff-style toggle, including the split-mode rightLineNum path.
func TestIndexForNewLine(t *testing.T) {
	m := diffViewModel{
		lines: []diffViewLine{
			{newLineNum: 10},
			{rightLineNum: 11}, // split-mode new-file number
			{newLineNum: 12},
		},
	}
	if got := m.indexForNewLine(11); got != 1 {
		t.Errorf("indexForNewLine(11) = %d, want 1 (rightLineNum match)", got)
	}
	if got := m.indexForNewLine(12); got != 2 {
		t.Errorf("indexForNewLine(12) = %d, want 2", got)
	}
	if got := m.indexForNewLine(999); got != -1 {
		t.Errorf("indexForNewLine(999) = %d, want -1", got)
	}
	if got := m.indexForNewLine(0); got != -1 {
		t.Errorf("indexForNewLine(0) = %d, want -1", got)
	}
}

// TestReanchorTo verifies that re-anchoring after a style toggle centers the
// cursor on the matching source line and falls back to the top when absent.
func TestReanchorTo(t *testing.T) {
	lines := make([]diffViewLine, 40)
	for i := range lines {
		lines[i] = diffViewLine{newLineNum: i + 1, content: "x"}
	}
	m := diffViewModel{lines: lines, height: 10}

	m.reanchorTo(20) // line 20 -> index 19
	if m.cursor != 19 {
		t.Fatalf("cursor = %d, want 19", m.cursor)
	}
	// Centered: offset ~ cursor - height/2, and cursor stays on screen.
	if m.offset != 19-5 {
		t.Errorf("offset = %d, want %d (centered)", m.offset, 19-5)
	}
	if m.isCursorOffScreen() {
		t.Error("cursor should be visible after reanchor")
	}

	// Missing line falls back to the top.
	m.reanchorTo(9999)
	if m.cursor != 0 || m.offset != 0 {
		t.Errorf("fallback: cursor=%d offset=%d, want 0/0", m.cursor, m.offset)
	}
}

// TestFitToWidth verifies the split-diff column normalizer truncates over-wide
// content and pads under-wide content to an exact visual width.
func TestFitToWidth(t *testing.T) {
	if got := fitToWidth("abc", 5); lipgloss.Width(got) != 5 {
		t.Errorf("pad: width = %d, want 5 (%q)", lipgloss.Width(got), got)
	}
	if got := fitToWidth("abcdefgh", 4); lipgloss.Width(got) != 4 {
		t.Errorf("truncate: width = %d, want 4 (%q)", lipgloss.Width(got), got)
	}
	if got := fitToWidth("abcd", 4); got != "abcd" {
		t.Errorf("exact: got %q, want unchanged", got)
	}
}

func TestMatchRanges(t *testing.T) {
	// Smartcase: an all-lowercase query matches case-insensitively.
	if got := matchRanges("Foo foo FOO", "foo"); len(got) != 3 {
		t.Fatalf("case-insensitive: want 3 matches, got %d: %+v", len(got), got)
	}
	// A query with an uppercase letter is case-sensitive.
	got := matchRanges("Foo foo", "Foo")
	if len(got) != 1 || got[0].start != 0 || got[0].end != 3 {
		t.Errorf("case-sensitive: want one match [0,3), got %+v", got)
	}
	if matchRanges("abc", "") != nil {
		t.Error("empty query should yield no ranges")
	}
	if matchRanges("abc", "zzz") != nil {
		t.Error("no occurrence should yield no ranges")
	}
}

func TestDiffSearchNavigation(t *testing.T) {
	lines := []diffViewLine{
		{kind: types.DiffLineContext, newLineNum: 1, content: "alpha"},
		{kind: types.DiffLineContext, newLineNum: 2, content: "beta needle"},
		{kind: types.DiffLineContext, newLineNum: 3, content: "gamma"},
		{kind: types.DiffLineContext, newLineNum: 4, content: "delta needle"},
	}
	m := diffViewModel{lines: lines, height: 10}

	if n := m.RunSearch("needle", false, 0); n != 2 {
		t.Fatalf("want 2 matches, got %d", n)
	}
	if m.cursor != 1 {
		t.Errorf("forward search from top should land on first match (index 1), got %d", m.cursor)
	}
	if cur, total := m.SearchStatus(); cur != 1 || total != 2 {
		t.Errorf("status: want 1/2, got %d/%d", cur, total)
	}

	// Next wraps 1 -> 3 -> 1.
	m.StepMatch(false)
	if m.cursor != 3 {
		t.Errorf("next match should be index 3, got %d", m.cursor)
	}
	m.StepMatch(false)
	if m.cursor != 1 {
		t.Errorf("next match should wrap to index 1, got %d", m.cursor)
	}
	// Previous wraps 1 -> 3.
	m.StepMatch(true)
	if m.cursor != 3 {
		t.Errorf("prev match should wrap to index 3, got %d", m.cursor)
	}

	// No matches: cursor returns to the origin and search is inactive.
	if n := m.RunSearch("zzz", false, 2); n != 0 {
		t.Errorf("want 0 matches, got %d", n)
	}
	if m.SearchActive() {
		t.Error("search should be inactive with no matches")
	}
	if m.cursor != 2 {
		t.Errorf("no-match search should restore origin cursor 2, got %d", m.cursor)
	}

	// Backward search picks the nearest match before the origin.
	m.RunSearch("needle", true, 3)
	if m.cursor != 1 {
		t.Errorf("backward search from index 3 should land on match index 1, got %d", m.cursor)
	}

	m.ClearSearch()
	if m.SearchActive() || m.searchQuery != "" {
		t.Error("ClearSearch should reset search state")
	}
}

func TestHalfPageScrollMovesCursor(t *testing.T) {
	// 100 selectable context lines, viewport height 20 (half page = 10).
	lines := make([]diffViewLine, 100)
	for i := range lines {
		lines[i] = diffViewLine{kind: types.DiffLineContext, newLineNum: i + 1, content: "x"}
	}
	m := diffViewModel{lines: lines, height: 20, cursor: 0, offset: 0}

	// Down: cursor and viewport both advance ~half a page, cursor stays visible.
	m.ScrollDownHalfPage()
	if m.cursor != 10 {
		t.Errorf("after half-page down, cursor = %d, want 10", m.cursor)
	}
	if m.isCursorOffScreen() {
		t.Error("cursor should remain visible after half-page down")
	}

	m.ScrollDownHalfPage()
	if m.cursor != 20 {
		t.Errorf("after second half-page down, cursor = %d, want 20", m.cursor)
	}

	// Up: cursor follows back.
	m.ScrollUpHalfPage()
	if m.cursor != 10 {
		t.Errorf("after half-page up, cursor = %d, want 10", m.cursor)
	}
	if m.isCursorOffScreen() {
		t.Error("cursor should remain visible after half-page up")
	}

	// At the bottom the cursor still advances to the last line even though the
	// viewport can't scroll further.
	m.cursor = 95
	m.offset = 80
	m.ScrollDownHalfPage()
	if m.cursor != len(m.lines)-1 {
		t.Errorf("near bottom, cursor = %d, want %d", m.cursor, len(m.lines)-1)
	}
	if m.isCursorOffScreen() {
		t.Error("cursor should remain visible near the bottom")
	}
}

func TestScreenLinesForCollapsedComment(t *testing.T) {
	c := &types.ReviewComment{Type: types.CommentNote, Body: "looks good"}
	content := formatInlineComment(c) // collapsed 3-line box
	want := strings.Count(content, "\n") + 1
	if want != 3 {
		t.Fatalf("sanity: collapsed comment box should be 3 lines, got %d", want)
	}
	m := diffViewModel{
		lines: []diffViewLine{
			{kind: types.DiffLineContext, newLineNum: 1, content: "code"},
			{isComment: true, comment: c, content: content},
		},
	}
	// screenLinesFor must report the comment's true rendered height (3), not 1,
	// or the scroll/cursor math desyncs and content runs off the viewport.
	if got := m.screenLinesFor(1); got != want {
		t.Errorf("collapsed comment screenLinesFor = %d, want %d", got, want)
	}
	if got := m.screenLinesFor(0); got != 1 {
		t.Errorf("plain code line screenLinesFor = %d, want 1", got)
	}
}

func TestEditorTargetLine(t *testing.T) {
	lines := make([]diffViewLine, 100)
	for i := range lines {
		lines[i] = diffViewLine{kind: types.DiffLineContext, newLineNum: i + 1, content: "x"}
	}
	m := diffViewModel{lines: lines, height: 20}

	// Cursor visible -> use the cursor's line.
	m.offset = 0
	m.cursor = 5
	if got := m.EditorTargetLine(); got != 6 {
		t.Errorf("visible cursor: EditorTargetLine = %d, want 6 (cursor line)", got)
	}

	// Cursor scrolled off-screen -> use the top-of-window line.
	m.offset = 50
	m.cursor = 5
	if got := m.EditorTargetLine(); got != 51 {
		t.Errorf("off-screen cursor: EditorTargetLine = %d, want 51 (top of window)", got)
	}
}

func TestYankText(t *testing.T) {
	m := diffViewModel{
		lines: []diffViewLine{
			{kind: types.DiffLineContext, newLineNum: 1, content: "line one"},
			{kind: types.DiffLineAdded, newLineNum: 2, content: "line two"},
			{kind: types.DiffLineAdded, newLineNum: 3, content: "line three"},
		},
	}
	// Single line under the cursor.
	m.cursor = 1
	if got := m.YankText(); got != "line two" {
		t.Errorf("single-line yank = %q, want %q", got, "line two")
	}
	// Visual selection yanks the range joined by newlines.
	m.visualMode = true
	m.visualStart = 0
	m.cursor = 2
	if got := m.YankText(); got != "line one\nline two\nline three" {
		t.Errorf("visual yank = %q, want the three lines joined", got)
	}
	// Split-mode added line yanks the right side.
	sm := diffViewModel{lines: []diffViewLine{
		{isSplit: true, leftEmpty: true, rightContent: "added right"},
	}}
	if got := sm.YankText(); got != "added right" {
		t.Errorf("split added yank = %q, want %q", got, "added right")
	}
}

// TestFormatExpandedCommentNoTrailingBlankLine verifies a comment body that ends
// in a newline (or trailing blank paragraph) does not render a stray empty box
// line between the last line of text and the footer.
func TestFormatExpandedCommentNoTrailingBlankLine(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"trailing newline", "a note\n"},
		{"trailing blank paragraph", "a note\n\n"},
		{"multiple trailing blanks", "a note\n\n\n"},
		{"no trailing newline", "a note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &types.ReviewComment{Type: types.CommentNote, Body: tc.body}
			out := formatExpandedComment(c, 80, "", false)
			lines := strings.Split(out, "\n")
			if len(lines) < 2 {
				t.Fatalf("expected at least header+footer, got %d lines: %q", len(lines), out)
			}
			footer := lines[len(lines)-1]
			if !strings.Contains(footer, "╚") {
				t.Errorf("last line is not the footer: %q", footer)
			}
			// The line just above the footer must contain body text, not be an
			// empty box line (bare prefix).
			beforeFooter := strings.TrimRight(lines[len(lines)-2], " ")
			if beforeFooter == "  ║" {
				t.Errorf("stray blank box line before footer:\n%s", out)
			}
			if !strings.Contains(beforeFooter, "a note") {
				t.Errorf("line before footer should hold body text, got %q (full:\n%s)", beforeFooter, out)
			}
		})
	}
}

// TestFormatExpandedCommentKeepsInteriorBlankLines verifies the trailing-blank
// trim does not strip blank lines that separate paragraphs in the middle.
func TestFormatExpandedCommentKeepsInteriorBlankLines(t *testing.T) {
	c := &types.ReviewComment{Type: types.CommentNote, Body: "first paragraph\n\nsecond paragraph"}
	out := formatExpandedComment(c, 80, "", false)
	if !strings.Contains(out, "first paragraph") || !strings.Contains(out, "second paragraph") {
		t.Fatalf("missing paragraphs:\n%s", out)
	}
	// There should be an interior empty box line between the two paragraphs.
	lines := strings.Split(out, "\n")
	var firstIdx, secondIdx int
	for i, l := range lines {
		if strings.Contains(l, "first paragraph") {
			firstIdx = i
		}
		if strings.Contains(l, "second paragraph") {
			secondIdx = i
		}
	}
	if secondIdx-firstIdx < 2 {
		t.Errorf("expected a blank separator line between paragraphs:\n%s", out)
	}
}

// TestOffScreenAccountsForCommentRows verifies isCursorOffScreen and
// lastVisibleLine count comment rows (3 for a collapsed comment) in non-wrap
// mode. Previously they assumed one screen row per logical line, so the cursor
// could be reported on-screen while it had actually scrolled off the bottom past
// a comment — letting content run out of the viewport.
func TestOffScreenAccountsForCommentRows(t *testing.T) {
	lines := make([]diffViewLine, 10)
	for i := range lines {
		lines[i] = diffViewLine{kind: types.DiffLineContext, newLineNum: i + 1, content: "x"}
	}
	// Index 5 is a collapsed 3-row comment box.
	lines[5] = diffViewLine{isComment: true, content: "a\nb\nc"}

	m := diffViewModel{lines: lines, height: 10, offset: 0, wrap: false}

	// Screen rows 0..8 sum to 11 (the comment adds 2 extra), so index 8 sits below
	// a height-10 viewport.
	m.cursor = 8
	if !m.isCursorOffScreen() {
		t.Errorf("cursor=8 should be off-screen (comment pushes it past the viewport)")
	}
	// Index 6 is the first line after the comment, still on screen (rows 0..6 = 9).
	m.cursor = 6
	if m.isCursorOffScreen() {
		t.Errorf("cursor=6 should be on-screen")
	}
	// Only indices 0..7 fit in 10 rows once the comment's extra rows are counted.
	if got := m.lastVisibleLine(); got != 7 {
		t.Errorf("lastVisibleLine = %d, want 7", got)
	}
}

func TestAnnotationRenderingAndToggle(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 30, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 3, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "line one"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "line two"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "line three"},
			},
		}},
		annotations: []types.Annotation{
			{TargetRef: "main.go", LineStart: 1, LineEnd: 2, Summary: "guards the deposit",
				Refs: []types.DocRef{{Kind: types.DocRefFile, Doc: "TODO.md", Label: "srv-008", StartLine: 40, EndLine: 52}}},
		},
	}
	m.buildLines()

	// An annotation box line exists; lines 1-2 are flagged annotated, line 3 is not.
	var annLine *diffViewLine
	annotatedCount := 0
	for i := range m.lines {
		if m.lines[i].isAnnotation {
			annLine = &m.lines[i]
		}
		if m.lines[i].annotated {
			annotatedCount++
		}
	}
	if annLine == nil {
		t.Fatal("expected an annotation line in the built lines")
	}
	if annotatedCount != 2 {
		t.Errorf("annotated code lines = %d, want 2 (lines 1-2)", annotatedCount)
	}
	out := m.View()
	if !strings.Contains(out, "guards the deposit") {
		t.Error("rendered view should contain the annotation summary")
	}
	if !strings.Contains(out, "srv-008") {
		t.Error("rendered view should contain the ref label")
	}

	// Hiding overlays removes the annotation box and the range flags.
	m.ToggleOverlays()
	for _, ln := range m.lines {
		if ln.isAnnotation {
			t.Error("annotation line should be gone when overlays hidden")
		}
		if ln.annotated {
			t.Error("annotated flag should be cleared when overlays hidden")
		}
	}
	if strings.Contains(m.View(), "guards the deposit") {
		t.Error("summary should not render when overlays hidden")
	}
}

// TestAnnotationRailInWrapAndFileModes covers two regressions: the cyan range
// rail must render in wrap mode (renderWrappedLine) and in current-file view, and
// switching to file view must keep annotations (not clear them).
func TestAnnotationRailInWrapAndFileModes(t *testing.T) {
	theme := DefaultTheme()
	railCount := func(s string) int { return strings.Count(s, "46m▌") }

	// Wrap mode, unified: a long annotated line wraps and every row keeps the rail.
	wm := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 28, height: 40, style: diffStyleUnified, wrap: true,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 1, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "a long line that will wrap across this narrow pane several times over"},
			},
		}},
		annotations: []types.Annotation{{TargetRef: "main.go", LineStart: 1, LineEnd: 1, Summary: "n"}},
	}
	wm.buildLines()
	if railCount(wm.View()) < 2 {
		t.Error("wrap mode should draw the range rail on every wrapped row")
	}

	// File view: annotations present and railed.
	fm := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 60, height: 40, style: diffStyleFile,
		annotations: []types.Annotation{{TargetRef: "main.go", LineStart: 2, LineEnd: 3, Summary: "n"}},
	}
	fm.buildFileViewLines("a\nb\nc\nd\n")
	got := 0
	for _, ln := range fm.lines {
		if ln.annotated {
			got++
		}
	}
	if got != 2 {
		t.Errorf("file view should flag the 2 in-range lines, got %d", got)
	}
	if railCount(fm.View()) != 2 {
		t.Errorf("file view should draw 2 rails, got %d", railCount(fm.View()))
	}
}

// TestAnnotationBoxWrapsWithRightBorder verifies a long annotation wraps to the
// pane width with a right border, and that screenLinesFor matches the rows drawn.
func TestAnnotationBoxWrapsWithRightBorder(t *testing.T) {
	theme := DefaultTheme()
	const width = 40
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: width, height: 40, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 1, Header: "f",
			Lines: []types.DiffLine{{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "x"}},
		}},
		annotations: []types.Annotation{{TargetRef: "main.go", LineStart: 1, LineEnd: 1,
			Summary: "a really long annotation summary that must wrap across several rows inside the box"}},
	}
	m.buildLines()

	annIdx := -1
	for i := range m.lines {
		if m.lines[i].isAnnotation {
			annIdx = i
		}
	}
	if annIdx < 0 {
		t.Fatal("no annotation box")
	}
	rows := m.annotationBoxRows(m.lines[annIdx].content)
	if len(rows) < 2 {
		t.Fatalf("expected the long summary to wrap, got %d rows", len(rows))
	}
	if m.screenLinesFor(annIdx) != len(rows) {
		t.Errorf("screenLinesFor=%d but %d rows drawn", m.screenLinesFor(annIdx), len(rows))
	}

	// Box rows are the ones with the right border; each is exactly pane width and
	// starts with the left bar.
	boxRows := 0
	for _, line := range strings.Split(stripANSISeq(m.View()), "\n") {
		if !strings.HasSuffix(line, "│") {
			continue
		}
		boxRows++
		if w := len([]rune(line)); w != width {
			t.Errorf("box row width = %d, want %d: %q", w, width, line)
		}
		if !strings.HasPrefix(line, "▌") {
			t.Errorf("box row should start with the left bar: %q", line)
		}
	}
	if boxRows != len(rows) {
		t.Errorf("rendered %d bordered box rows, want %d", boxRows, len(rows))
	}
}

// TestAnchorLineForCursorOnBox verifies the re-anchor source line falls back to
// the nearest code line when the cursor sits on an annotation/comment box, so
// toggles (t / a) don't jump the viewport to the top.
func TestAnchorLineForCursorOnBox(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 40, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 3, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "a"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "b"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "c"},
			},
		}},
		annotations: []types.Annotation{{TargetRef: "main.go", LineStart: 2, LineEnd: 3, Summary: "n"}},
	}
	m.buildLines()
	boxIdx := -1
	for i := range m.lines {
		if m.lines[i].isAnnotation {
			boxIdx = i
		}
	}
	if boxIdx < 0 {
		t.Fatal("no annotation box built")
	}
	m.cursor = boxIdx
	if ln := m.anchorLineForCursor(); ln != 3 {
		t.Errorf("anchorLineForCursor on a box = %d, want 3 (nearest code line)", ln)
	}
}

// TestJumpToMark verifies < / > navigation lands on comment and annotation
// boxes and stops at the ends (no wrap-around).
func TestJumpToMark(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 40, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 4, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "a"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "b"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "c"},
				{Kind: types.DiffLineAdded, NewLineNum: 4, Content: "d"},
			},
		}},
		comments: []types.ReviewComment{
			{ID: "c1", TargetRef: "main.go", LineStart: 2, LineEnd: 2, Type: types.CommentIssue, Body: "fix"},
		},
		annotations: []types.Annotation{
			{TargetRef: "main.go", LineStart: 4, LineEnd: 4, Summary: "note"},
		},
	}
	m.buildLines()
	m.cursor = 0

	if !m.JumpToMark(1) || !m.lines[m.cursor].isComment {
		t.Fatalf("first > should land on the comment box (cursor=%d)", m.cursor)
	}
	if !m.JumpToMark(1) || !m.lines[m.cursor].isAnnotation {
		t.Fatalf("second > should land on the annotation box (cursor=%d)", m.cursor)
	}
	if m.JumpToMark(1) {
		t.Error("> past the last mark should be a no-op")
	}
	if !m.JumpToMark(-1) || !m.lines[m.cursor].isComment {
		t.Errorf("< should return to the comment box (cursor=%d)", m.cursor)
	}
}

func TestJumpToChange(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 40, style: diffStyleUnified,
		hunks: []types.DiffHunk{
			{
				OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 2, Header: "@@ hunk1 @@",
				Lines: []types.DiffLine{
					{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "a"},
					{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "b"},
				},
			},
			{
				OldStart: 10, OldCount: 0, NewStart: 10, NewCount: 2, Header: "@@ hunk2 @@",
				Lines: []types.DiffLine{
					{Kind: types.DiffLineAdded, NewLineNum: 10, Content: "c"},
					{Kind: types.DiffLineAdded, NewLineNum: 11, Content: "d"},
				},
			},
		},
	}
	m.buildLines()
	m.cursor = m.nearestSelectable(0, 1) // first changed line of block 1

	firstBlockCursor := m.cursor
	if m.lines[m.cursor].isHunk {
		t.Fatal("cursor should start on a changed line, not a hunk header")
	}

	if !m.JumpToChange(1) {
		t.Fatal("] should jump forward to the next change block")
	}
	if m.lines[m.cursor].isHunk || m.lines[m.cursor].newLineNum != 10 {
		t.Fatalf("] should land on block 2's first changed line (got cursor=%d, line=%+v)", m.cursor, m.lines[m.cursor])
	}

	if m.JumpToChange(1) {
		t.Error("] past the last change should be a no-op")
	}

	if !m.JumpToChange(-1) {
		t.Fatal("[ should jump back to the previous change block")
	}
	if m.cursor != firstBlockCursor {
		t.Errorf("[ should return to block 1's first line (got cursor=%d, want %d)", m.cursor, firstBlockCursor)
	}

	if m.JumpToChange(-1) {
		t.Error("[ at the first change should be a no-op")
	}
}

// TestJumpToChangeFullFile is the case the hunk-header approach got wrong: in
// full-file mode the whole file is a single git hunk, so jumping must key off
// runs of changed lines, not hunk headers. Here one hunk holds two separate
// edits divided by context — [ / ] must stop at each.
func TestJumpToChangeFullFile(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 40, style: diffStyleUnified, fullFile: true,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 7, NewStart: 1, NewCount: 7, Header: "@@ whole file @@",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineContext, OldLineNum: 1, NewLineNum: 1, Content: "ctx1"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "edit A1"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "edit A2"},
				{Kind: types.DiffLineContext, OldLineNum: 2, NewLineNum: 4, Content: "ctx2"},
				{Kind: types.DiffLineContext, OldLineNum: 3, NewLineNum: 5, Content: "ctx3"},
				{Kind: types.DiffLineAdded, NewLineNum: 6, Content: "edit B1"},
				{Kind: types.DiffLineContext, OldLineNum: 4, NewLineNum: 7, Content: "ctx4"},
			},
		}},
	}
	m.buildLines()
	// Start at the top (first context line).
	m.cursor = m.nearestSelectable(0, 1)

	// ] stops at the first edit block (line 2), then the second (line 6).
	if !m.JumpToChange(1) || m.lines[m.cursor].newLineNum != 2 {
		t.Fatalf("] should stop at the first edit (line 2), got cursor=%d line=%+v", m.cursor, m.lines[m.cursor])
	}
	if !m.JumpToChange(1) || m.lines[m.cursor].newLineNum != 6 {
		t.Fatalf("] should stop at the second edit (line 6), got cursor=%d line=%+v", m.cursor, m.lines[m.cursor])
	}
	if m.JumpToChange(1) {
		t.Error("] past the last edit should be a no-op even in full-file mode")
	}
	// [ walks back: from edit B to edit A.
	if !m.JumpToChange(-1) || m.lines[m.cursor].newLineNum != 2 {
		t.Fatalf("[ should return to the first edit (line 2), got cursor=%d line=%+v", m.cursor, m.lines[m.cursor])
	}
}

// TestAnnotationRenderingSplit covers the gap that split (and full-file) modes
// previously rendered no annotations at all: the box, the annotated flags, and
// the cyan range bar must all appear in split mode too.
func TestAnnotationRenderingSplit(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 100, height: 30, style: diffStyleSplit,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 3, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "line one"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "line two"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "line three"},
			},
		}},
		annotations: []types.Annotation{
			{TargetRef: "main.go", LineStart: 1, LineEnd: 2, Summary: "guards the deposit"},
		},
	}
	m.buildLines()

	annotatedCount := 0
	hasBox := false
	for i := range m.lines {
		if m.lines[i].isAnnotation {
			hasBox = true
		}
		if m.lines[i].annotated {
			annotatedCount++
		}
	}
	if !hasBox {
		t.Error("split mode should insert the annotation box")
	}
	if annotatedCount != 2 {
		t.Errorf("split annotated code lines = %d, want 2", annotatedCount)
	}
	out := m.View()
	if !strings.Contains(out, "guards the deposit") {
		t.Error("split view should render the annotation summary")
	}
	if !strings.Contains(out, annotationRangeBar) {
		t.Error("split view should render the cyan range bar")
	}
}

// TestCommentFilterCycle covers the three-state comment filter: shown → dimmed →
// hidden → shown. Dim flags the comment-only line; hide removes it from the view
// and from navigation; the third press restores everything.
func TestCommentFilterCycle(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 30, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 3, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "// explain"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "doWork()"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "x := 1 // trailing"},
			},
		}},
	}
	m.buildLines()
	if m.commentFilter != commentsShown || len(m.commentLines) != 0 {
		t.Fatalf("should start shown with no comment lines, got filter=%d lines=%v", m.commentFilter, m.commentLines)
	}

	// 1st press → dimmed: comment-only line flagged, still present in the view.
	m.CycleCommentFilter()
	if m.commentFilter != commentsDimmed {
		t.Fatalf("after 1 press filter=%d, want dimmed", m.commentFilter)
	}
	if !m.commentLines[1] || m.commentLines[2] || m.commentLines[3] {
		t.Errorf("only line 1 should be comment-only, got %v", m.commentLines)
	}
	commentIdx := -1
	for i, ln := range m.lines {
		if ln.newLineNum == 1 {
			commentIdx = i
		}
	}
	if commentIdx < 0 {
		t.Fatal("comment line should still be in m.lines when dimmed")
	}
	if m.screenLinesFor(commentIdx) != 1 {
		t.Error("dimmed comment line should still occupy a screen row")
	}

	// 2nd press → hidden: the comment line occupies no rows and is unselectable.
	m.CycleCommentFilter()
	if m.commentFilter != commentsHidden {
		t.Fatalf("after 2 presses filter=%d, want hidden", m.commentFilter)
	}
	if m.screenLinesFor(commentIdx) != 0 {
		t.Error("hidden comment line should occupy zero screen rows")
	}
	if m.isSelectable(commentIdx) {
		t.Error("hidden comment line should not be selectable")
	}

	// 3rd press → shown: everything restored.
	m.CycleCommentFilter()
	if m.commentFilter != commentsShown || len(m.commentLines) != 0 {
		t.Errorf("after 3 presses should be shown with no comment lines, got filter=%d lines=%v", m.commentFilter, m.commentLines)
	}
	if m.screenLinesFor(commentIdx) != 1 {
		t.Error("comment line should occupy a row again when shown")
	}
}
