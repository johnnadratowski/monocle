package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/anthropics/monocle/internal/types"
)

func TestPaneRegionContains(t *testing.T) {
	r := paneRegion{x: 10, y: 5, w: 20, h: 10}

	tests := []struct {
		name string
		x, y int
		want bool
	}{
		{"inside", 15, 8, true},
		{"top-left corner", 10, 5, true},
		{"bottom-right edge excluded", 30, 15, false},
		{"just inside bottom-right", 29, 14, true},
		{"left of region", 9, 8, false},
		{"above region", 15, 4, false},
		{"below region", 15, 15, false},
		{"right of region", 30, 8, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.contains(tt.x, tt.y); got != tt.want {
				t.Errorf("contains(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestPaneRegionTranslate(t *testing.T) {
	r := paneRegion{x: 10, y: 5, w: 20, h: 10}

	rx, ry := r.translate(15, 8)
	if rx != 5 || ry != 3 {
		t.Errorf("translate(15, 8) = (%d, %d), want (5, 3)", rx, ry)
	}

	rx, ry = r.translate(10, 5)
	if rx != 0 || ry != 0 {
		t.Errorf("translate(10, 5) = (%d, %d), want (0, 0)", rx, ry)
	}
}

func TestComputePaneLayoutHorizontal(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutHorizontal {
		t.Fatalf("expected horizontal layout")
	}

	layout := computePaneLayout(&app)

	// Sidebar content starts at x=1 (border left), y=2 (title + border top)
	if layout.sidebar.x != 1 {
		t.Errorf("sidebar.x = %d, want 1", layout.sidebar.x)
	}
	if layout.sidebar.y != 2 {
		t.Errorf("sidebar.y = %d, want 2", layout.sidebar.y)
	}
	if layout.sidebar.w != app.sidebar.width {
		t.Errorf("sidebar.w = %d, want %d", layout.sidebar.w, app.sidebar.width)
	}
	if layout.sidebar.h != app.sidebar.height {
		t.Errorf("sidebar.h = %d, want %d", layout.sidebar.h, app.sidebar.height)
	}

	// Diff content starts after sidebar outer width + diff border left
	sidebarOuterW := app.sidebar.width + 2
	expectedDiffX := sidebarOuterW + 1
	if layout.diff.x != expectedDiffX {
		t.Errorf("diff.x = %d, want %d", layout.diff.x, expectedDiffX)
	}
	if layout.diff.y != 2 {
		t.Errorf("diff.y = %d, want 2", layout.diff.y)
	}
}

func TestComputePaneLayoutStacked(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutStacked {
		t.Fatalf("expected stacked layout at width 80")
	}

	layout := computePaneLayout(&app)

	// Both panes start at x=1
	if layout.sidebar.x != 1 {
		t.Errorf("sidebar.x = %d, want 1", layout.sidebar.x)
	}
	if layout.diff.x != 1 {
		t.Errorf("diff.x = %d, want 1", layout.diff.x)
	}

	// Diff starts after sidebar outer height
	expectedDiffY := titleHeight + (app.sidebar.height + borderH) + 1
	if layout.diff.y != expectedDiffY {
		t.Errorf("diff.y = %d, want %d", layout.diff.y, expectedDiffY)
	}
}

func TestOverlayRegion(t *testing.T) {
	r := overlayRegion(120, 40, 60, 20)

	// Centered: left = (120-60)/2 = 30, top = (40-20)/2 = 10
	if r.x != 30 {
		t.Errorf("overlay.x = %d, want 30", r.x)
	}
	if r.y != 10 {
		t.Errorf("overlay.y = %d, want 10", r.y)
	}
	if r.w != 60 {
		t.Errorf("overlay.w = %d, want 60", r.w)
	}
	if r.h != 20 {
		t.Errorf("overlay.h = %d, want 20", r.h)
	}
}

func TestOverlayRegionMinTopPad(t *testing.T) {
	// When overlay is almost as tall as screen, topPad should be clamped to 2
	r := overlayRegion(80, 20, 60, 19)
	if r.y != 2 {
		t.Errorf("overlay.y = %d, want 2 (minimum top pad)", r.y)
	}
}

func TestMouseClickFocusSidebar(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	// Start with focus on diff
	app.focus = focusMain
	app.sidebar.focused = false
	app.diffView.focused = true

	layout := computePaneLayout(&app)

	// Click inside sidebar content area
	clickX := layout.sidebar.x + 2
	clickY := layout.sidebar.y + 2
	result, _ := app.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	resultApp := result.(appModel)

	if resultApp.focus != focusSidebar {
		t.Errorf("after sidebar click: focus = %d, want focusSidebar(%d)", resultApp.focus, focusSidebar)
	}
	if !resultApp.sidebar.focused {
		t.Error("after sidebar click: sidebar.focused = false, want true")
	}
}

func TestMouseClickFocusDiff(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	// Start with focus on sidebar (default)
	if app.focus != focusSidebar {
		t.Fatalf("expected initial focus on sidebar")
	}

	layout := computePaneLayout(&app)

	// Click inside diff content area
	clickX := layout.diff.x + 5
	clickY := layout.diff.y + 5
	result, _ := app.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	resultApp := result.(appModel)

	if resultApp.focus != focusMain {
		t.Errorf("after diff click: focus = %d, want focusMain(%d)", resultApp.focus, focusMain)
	}
	if !resultApp.diffView.focused {
		t.Error("after diff click: diffView.focused = false, want true")
	}
}

func TestMouseWheelScrollDiff(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	// Set up some lines so we can scroll
	app.diffView.lines = make([]diffViewLine, 100)
	for i := range app.diffView.lines {
		app.diffView.lines[i] = diffViewLine{content: "test line", newLineNum: i + 1}
	}
	app.diffView.offset = 0

	layout := computePaneLayout(&app)

	// Scroll down in diff area
	clickX := layout.diff.x + 5
	clickY := layout.diff.y + 5
	result, _ := app.Update(tea.MouseWheelMsg{X: clickX, Y: clickY, Button: tea.MouseWheelDown})
	resultApp := result.(appModel)

	if resultApp.diffView.offset != mouseScrollLines {
		t.Errorf("after wheel down: offset = %d, want %d", resultApp.diffView.offset, mouseScrollLines)
	}
}

func TestMouseWheelScrollSidebar(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	// Set up enough files to scroll
	app.sidebar.files = make([]types.ChangedFile, 50)
	for i := range app.sidebar.files {
		app.sidebar.files[i] = types.ChangedFile{Path: "file.go", Status: types.FileModified}
	}
	app.sidebar.offset = 0

	layout := computePaneLayout(&app)

	// Scroll down in sidebar area
	clickX := layout.sidebar.x + 2
	clickY := layout.sidebar.y + 2
	result, _ := app.Update(tea.MouseWheelMsg{X: clickX, Y: clickY, Button: tea.MouseWheelDown})
	resultApp := result.(appModel)

	if resultApp.sidebar.offset != mouseScrollLines {
		t.Errorf("after wheel down: offset = %d, want %d", resultApp.sidebar.offset, mouseScrollLines)
	}
}

func TestSidebarItemAtLineNoContentItems(t *testing.T) {
	s := sidebarModel{
		files: []types.ChangedFile{
			{Path: "a.go", Status: types.FileModified},
			{Path: "b.go", Status: types.FileModified},
			{Path: "c.go", Status: types.FileModified},
		},
		width:  40,
		height: 20,
	}

	// Line 0: "Files" header
	if got := s.itemAtLine(0); got != -1 {
		t.Errorf("line 0 (Files header) = %d, want -1", got)
	}
	// Line 1: first file (item 0)
	if got := s.itemAtLine(1); got != 0 {
		t.Errorf("line 1 (first file) = %d, want 0", got)
	}
	// Line 2: second file (item 1)
	if got := s.itemAtLine(2); got != 1 {
		t.Errorf("line 2 (second file) = %d, want 1", got)
	}
	// Line 3: third file (item 2)
	if got := s.itemAtLine(3); got != 2 {
		t.Errorf("line 3 (third file) = %d, want 2", got)
	}
	// Line 4: beyond items
	if got := s.itemAtLine(4); got != -1 {
		t.Errorf("line 4 (beyond) = %d, want -1", got)
	}
}

func TestSidebarItemAtLineWithContentItems(t *testing.T) {
	s := sidebarModel{
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan"},
		},
		files: []types.ChangedFile{
			{Path: "a.go", Status: types.FileModified},
			{Path: "b.go", Status: types.FileModified},
		},
		width:  40,
		height: 20,
	}

	// Line 0: "Review Items" header
	if got := s.itemAtLine(0); got != -1 {
		t.Errorf("line 0 (Review Items header) = %d, want -1", got)
	}
	// Line 1: first content item (item 0)
	if got := s.itemAtLine(1); got != 0 {
		t.Errorf("line 1 (content item) = %d, want 0", got)
	}
	// Line 2: blank separator
	if got := s.itemAtLine(2); got != -1 {
		t.Errorf("line 2 (blank separator) = %d, want -1", got)
	}
	// Line 3: "Files" header
	if got := s.itemAtLine(3); got != -1 {
		t.Errorf("line 3 (Files header) = %d, want -1", got)
	}
	// Line 4: first file (item 1)
	if got := s.itemAtLine(4); got != 1 {
		t.Errorf("line 4 (first file) = %d, want 1", got)
	}
	// Line 5: second file (item 2)
	if got := s.itemAtLine(5); got != 2 {
		t.Errorf("line 5 (second file) = %d, want 2", got)
	}
}

func TestDiffViewScreenLineToIndex(t *testing.T) {
	dv := diffViewModel{
		lines: []diffViewLine{
			{content: "line 0", newLineNum: 1},
			{content: "line 1", newLineNum: 2},
			{content: "line 2", newLineNum: 3},
			{content: "line 3", newLineNum: 4},
		},
		offset: 0,
		height: 10,
		width:  80,
	}

	// Non-wrap mode: 1:1 mapping
	if got := dv.screenLineToIndex(0); got != 0 {
		t.Errorf("screenLineToIndex(0) = %d, want 0", got)
	}
	if got := dv.screenLineToIndex(2); got != 2 {
		t.Errorf("screenLineToIndex(2) = %d, want 2", got)
	}
	if got := dv.screenLineToIndex(-1); got != -1 {
		t.Errorf("screenLineToIndex(-1) = %d, want -1", got)
	}
	if got := dv.screenLineToIndex(4); got != -1 {
		t.Errorf("screenLineToIndex(4) = %d, want -1", got)
	}
}

func TestDiffViewScreenLineToIndexWithOffset(t *testing.T) {
	dv := diffViewModel{
		lines: []diffViewLine{
			{content: "line 0", newLineNum: 1},
			{content: "line 1", newLineNum: 2},
			{content: "line 2", newLineNum: 3},
			{content: "line 3", newLineNum: 4},
		},
		offset: 2,
		height: 10,
		width:  80,
	}

	// With offset 2, screen line 0 maps to lines[2]
	if got := dv.screenLineToIndex(0); got != 2 {
		t.Errorf("screenLineToIndex(0) with offset=2 = %d, want 2", got)
	}
	if got := dv.screenLineToIndex(1); got != 3 {
		t.Errorf("screenLineToIndex(1) with offset=2 = %d, want 3", got)
	}
}

func TestDiffViewScreenLineToIndexWithComment(t *testing.T) {
	comment := &types.ReviewComment{
		Type: types.CommentIssue,
		Body: "fix this",
	}
	dv := diffViewModel{
		lines: []diffViewLine{
			{content: "line 0", newLineNum: 1},
			// Comment renders as 3 screen lines (header + body + footer)
			{content: formatInlineComment(comment), isComment: true, comment: comment},
			{content: "line 2", newLineNum: 2},
			{content: "line 3", newLineNum: 3},
		},
		offset: 0,
		height: 20,
		width:  80,
	}

	// Screen line 0 -> lines[0] (regular line)
	if got := dv.screenLineToIndex(0); got != 0 {
		t.Errorf("screenLineToIndex(0) = %d, want 0", got)
	}
	// Screen lines 1-3 -> lines[1] (comment, 3 screen lines)
	if got := dv.screenLineToIndex(1); got != 1 {
		t.Errorf("screenLineToIndex(1) = %d, want 1 (comment start)", got)
	}
	if got := dv.screenLineToIndex(3); got != 1 {
		t.Errorf("screenLineToIndex(3) = %d, want 1 (comment end)", got)
	}
	// Screen line 4 -> lines[2] (after the 3-line comment)
	if got := dv.screenLineToIndex(4); got != 2 {
		t.Errorf("screenLineToIndex(4) = %d, want 2 (first line after comment)", got)
	}
	// Screen line 5 -> lines[3]
	if got := dv.screenLineToIndex(5); got != 3 {
		t.Errorf("screenLineToIndex(5) = %d, want 3", got)
	}
}

func TestMouseEnabledDefault(t *testing.T) {
	m := NewApp(nil)
	if !m.mouseEnabled {
		t.Error("mouseEnabled should default to true")
	}
}

func TestMouseRightClickIgnored(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	// Right-click should be ignored
	result, _ := app.Update(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseRight})
	resultApp := result.(appModel)

	// Focus should not change from default
	if resultApp.focus != focusSidebar {
		t.Errorf("right-click changed focus to %d, expected no change", resultApp.focus)
	}
}

func TestCommentEditorClickTypeLabel(t *testing.T) {
	m := &commentEditorModel{
		active:      true,
		commentType: types.CommentIssue,
	}

	// Click on SUGGESTION label (starts at x=8: ISSUE(7) + separator(1))
	if !m.handleClick(8, 4) {
		t.Error("click on SUGGESTION label should return true")
	}
	if m.commentType != types.CommentSuggestion {
		t.Errorf("commentType = %v, want CommentSuggestion", m.commentType)
	}

	// Click on NOTE label (starts at x=21: ISSUE(7) + sep(1) + SUGGESTION(12) + sep(1))
	if !m.handleClick(21, 4) {
		t.Error("click on NOTE label should return true")
	}
	if m.commentType != types.CommentNote {
		t.Errorf("commentType = %v, want CommentNote", m.commentType)
	}

	// Click on wrong line should not change
	if m.handleClick(0, 3) {
		t.Error("click on wrong line should return false")
	}
}

func TestReviewSummaryClickActionLabel(t *testing.T) {
	m := &reviewSummaryModel{
		active: true,
		action: types.ActionApprove,
	}

	// Click on REQUEST CHANGES label (starts at x=10: APPROVE(9) + sep(1))
	if !m.handleClick(10, 2) {
		t.Error("click on REQUEST CHANGES label should return true")
	}
	if m.action != types.ActionRequestChanges {
		t.Errorf("action = %v, want ActionRequestChanges", m.action)
	}

	// Click back on APPROVE (starts at x=0)
	if !m.handleClick(0, 2) {
		t.Error("click on APPROVE label should return true")
	}
	if m.action != types.ActionApprove {
		t.Errorf("action = %v, want ActionApprove", m.action)
	}
}

func TestConfirmClickCheckbox(t *testing.T) {
	m := &confirmModel{
		active:      true,
		showDontAsk: true,
		dontAsk:     false,
	}

	// Click on checkbox position
	if !m.handleClick(1, 4) {
		t.Error("click on checkbox should return true")
	}
	if !m.dontAsk {
		t.Error("dontAsk should be toggled to true")
	}

	// Click again to toggle off
	m.handleClick(1, 4)
	if m.dontAsk {
		t.Error("dontAsk should be toggled back to false")
	}

	// Without showDontAsk, clicks should be ignored
	m.showDontAsk = false
	if m.handleClick(1, 4) {
		t.Error("click should be ignored when showDontAsk is false")
	}
}
