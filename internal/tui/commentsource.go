package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Classifying comments from the diff alone only works when a whole comment is on
// screen. A compact diff shows fragments: the middle of a block comment arrives
// without the /** that opened it several lines above, so the lexer sees ordinary
// text and the filter passes over exactly the lines a reviewer most wants faded.
//
// The fix is to classify the file, not the fragment. The new side of the diff is
// a real file the engine can hand over, so it is fetched once per file and
// lexed whole; the line numbers that come back are authoritative no matter how
// little of it the diff happens to show.

// newLineOf returns a row's new-file line number, taking the right side in split
// mode where that is where the new file lives.
func newLineOf(ln diffViewLine) int {
	if ln.rightLineNum > 0 {
		return ln.rightLineNum
	}
	return ln.newLineNum
}

// commentSourceMsg carries a file's full text for classification. Both sides
// arrive the same way: the removed half of an edit needs the file it was removed
// FROM, which the diff shows just as partially as the added half.
type commentSourceMsg struct {
	path    string
	content string
	base    bool
}

// needsCommentSource reports whether the view would benefit from the full file:
// the filter is on, there is a path, and the text is not already loaded.
//
// Whole-file mode is excluded because it already displays every line, so the
// reconstruction it does is the file.
func (m diffViewModel) needsCommentSource() bool {
	if m.commentFilter == commentsShown || m.path == "" || m.contentMode {
		return false
	}
	if m.fullFile || m.style == diffStyleFile {
		return false
	}
	return m.commentSourcePath != m.path
}

// needsBaseCommentSource is the same question for the old side. Asked separately
// because the two fetches are independent — one can arrive, or fail, without the
// other — and a file with no old side at all (newly added) must not keep asking.
func (m diffViewModel) needsBaseCommentSource() bool {
	if m.commentFilter == commentsShown || m.path == "" || m.contentMode {
		return false
	}
	if m.fullFile || m.style == diffStyleFile {
		return false
	}
	return m.baseSourcePath != m.path
}

// requestCommentSource asks the app layer for the current file's full text —
// both revisions, in one command, since the filter needs both to answer for a
// comment that was edited rather than merely added.
func (m diffViewModel) requestCommentSource() tea.Cmd {
	path := m.path
	needNew := m.needsCommentSource()
	needBase := m.needsBaseCommentSource()
	return func() tea.Msg {
		return requestCommentSourceMsg{path: path, wantNew: needNew, wantBase: needBase}
	}
}

type requestCommentSourceMsg struct {
	path     string
	wantNew  bool
	wantBase bool
}

// setCommentSource stores a fetched revision and re-derives the classification.
// A stale answer (the reviewer moved on while it was in flight) is dropped.
func (m *diffViewModel) setCommentSource(path, content string, base bool) bool {
	if path != m.path {
		return false
	}
	if base {
		m.baseSourcePath = path
		m.baseSource = content
	} else {
		m.commentSourcePath = path
		m.commentSource = content
	}
	m.computeCommentLines()
	return true
}

// sourceCommentLines classifies the whole file, or returns nil when its text has
// not been fetched. The map is keyed by new-file line number, which is what the
// rows carry, so no alignment step is needed.
func (m diffViewModel) sourceCommentLines() map[int]bool {
	if m.commentSource == "" || m.commentSourcePath != m.path || m.hl == nil {
		return nil
	}
	return m.hl.commentOnlyLines(m.path, m.commentSource)
}

// baseSourceCommentLines does the same for the old revision, keyed by OLD-file
// line number. This is what lets the removed half of an edited comment fade:
// from the fragment alone a removed middle line is just text, because the /**
// that opened it is above the hunk and was never removed.
func (m diffViewModel) baseSourceCommentLines() map[int]bool {
	if m.baseSource == "" || m.baseSourcePath != m.path || m.hl == nil {
		return nil
	}
	return m.hl.commentOnlyLines(m.path, m.baseSource)
}
