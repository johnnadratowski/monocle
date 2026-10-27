package tui

import (
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The tour's comment state (John 2026-10-27: "I also want to be able to hide
// comments from the regions. I think maybe this should be a global thing in
// tour, where it hides for both and uses the same key from both windows for the
// comment hiding."). It is one switch, shown or hidden, shared by Monocle and the
// regions explorer beside it: the key (m) in either window turns it, and the
// other follows. Monocle tells the walkthrough through comments_command; the
// walkthrough tells Monocle with `monocle review comments` (:comments). Dimming
// (M's cycle) stays Monocle's own and is never shared.

// commentsMsg is `:comments on|off|toggle` (no argument toggles): on shows the
// comment lines, off hides them.
type commentsMsg struct{ arg string }

// commentsCommand is the configured comments_command, or "" for none.
func (m appModel) commentsCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.CommentsCommand)
	}
	return ""
}

// commentsShownNow is the shared state as this TUI has it: anything but hidden
// is shown (dimmed comments are still on screen).
func (m appModel) commentsShownNow() bool { return m.diffView.commentFilter != commentsHidden }

// setCommentsShown puts the comment lines in the shared state: shown takes a
// hidden view back to shown (a dimmed one stays dimmed), hidden hides them.
// Idempotent, so a state pushed in twice does nothing the second time.
func (m appModel) setCommentsShown(shown bool) (appModel, tea.Cmd) {
	switch {
	case shown && m.diffView.commentFilter == commentsHidden:
		m.diffView.setCommentFilter(commentsShown)
	case !shown && m.diffView.commentFilter != commentsHidden:
		m.diffView.setCommentFilter(commentsHidden)
	}
	if label := m.diffView.CommentFilterLabel(); label != "" {
		m.statusBar.searchInfo = label
	} else {
		m.statusBar.searchInfo = "comments shown"
	}
	// Hiding is the first moment the full file is needed to classify its lines.
	if m.diffView.needsCommentSource() || m.diffView.needsBaseCommentSource() {
		return m, m.diffView.requestCommentSource()
	}
	return m, nil
}

// handleComments is :comments, and the walkthrough's push (the engine's
// comments event): it sets the state and tells no one, so a push never echoes
// back.
func (m appModel) handleComments(arg string) (appModel, tea.Cmd) {
	var shown bool
	switch arg {
	case "", "toggle":
		shown = !m.commentsShownNow()
	case "on":
		shown = true
	case "off":
		shown = false
	default:
		m.statusBar.searchInfo = "usage: :comments [on|off|toggle]"
		return m, nil
	}
	return m.setCommentsShown(shown)
}

// toggleSharedComments is the comments key in a walkthrough: shown and hidden
// in turn, and comments_command told the new state (MONOCLE_COMMENTS on or off)
// so the windows beside Monocle follow. Fire-and-forget: it never blocks the TUI.
func (m appModel) toggleSharedComments() (appModel, tea.Cmd) {
	shown := !m.commentsShownNow()
	m, cmd := m.setCommentsShown(shown)
	command, root := m.commentsCommand(), m.repoRoot
	state := "off"
	if shown {
		state = "on"
	}
	run := func() tea.Msg {
		_ = execHook(command, root, []string{"MONOCLE_COMMENTS=" + state, "MONOCLE_REPO_ROOT=" + root}, cursorTimeout, io.Discard)
		return nil
	}
	return m, tea.Batch(cmd, run)
}
