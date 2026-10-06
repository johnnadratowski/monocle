package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Something outside Monocle can follow what the reviewer is looking at: a view
// beside it that shows what the line under the cursor is about, say. Monocle
// reports where the diff cursor comes to rest through one configured command,
// cursor_command, and knows nothing about what it does with it.

// cursorSettleDelay is how long the cursor has to rest before it is reported:
// long enough that key repeat through a file reports only where it stops.
var cursorSettleDelay = 200 * time.Millisecond

// cursorTimeout bounds one run of cursor_command.
const cursorTimeout = 2 * time.Second

// cursorWatch is where the cursor was last seen and last reported. seq
// numbers the moves, so only the latest one's tick reports.
type cursorWatch struct {
	seen string
	ran  string
	seq  int
}

// cursorSettledMsg fires cursorSettleDelay after a move.
type cursorSettledMsg struct{ seq int }

// cursorCommand is the configured cursor_command, or "" for none.
func (m appModel) cursorCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.CursorCommand)
	}
	return ""
}

// cursorKey is where the diff cursor is, as "file:line", with the file
// repo-relative and the line its new-file number (0 when the row has none).
// It is "" while a modal is open — the comment editor, the command, search or
// shell line, any overlay — or when no file is on screen.
func (m appModel) cursorKey() (key, file string, line int) {
	if m.overlay != overlayNone || m.commandMode || m.searchMode || m.shellMode {
		return "", "", 0
	}
	dv := m.diffView
	if dv.path == "" || dv.contentMode || dv.mediaMode || dv.isViewingContentItem() {
		return "", "", 0
	}
	file = repoRelative(m.repoRoot, dv.path)
	line = dv.lineNumAt(dv.cursor)
	return fmt.Sprintf("%s:%d", file, line), file, line
}

// watchCursor notes a move of the cursor, or the opening or closing of a
// modal, and schedules the settle tick for it.
func (m appModel) watchCursor() (appModel, tea.Cmd) {
	if m.cursorCommand() == "" {
		return m, nil
	}
	key, _, _ := m.cursorKey()
	if key == m.cursor.seen {
		return m, nil
	}
	m.cursor.seen = key
	m.cursor.seq++
	if key == "" {
		return m, nil
	}
	seq := m.cursor.seq
	return m, tea.Tick(cursorSettleDelay, func(time.Time) tea.Msg { return cursorSettledMsg{seq: seq} })
}

// settleCursor runs cursor_command if the cursor has not moved since the tick
// was scheduled and rests somewhere other than the place last reported. The
// run is fire-and-forget: it never blocks the TUI, and its output is dropped.
func (m appModel) settleCursor(msg cursorSettledMsg) (appModel, tea.Cmd) {
	if msg.seq != m.cursor.seq {
		return m, nil
	}
	key, file, line := m.cursorKey()
	command := m.cursorCommand()
	if key == "" || key == m.cursor.ran || command == "" {
		return m, nil
	}
	m.cursor.ran = key
	root := m.repoRoot
	env := []string{"MONOCLE_FILE=" + file, fmt.Sprintf("MONOCLE_LINE=%d", line), "MONOCLE_REPO_ROOT=" + root}
	return m, func() tea.Msg {
		_ = execHook(command, root, env, cursorTimeout, io.Discard)
		return nil
	}
}
