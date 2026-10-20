package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// related_open hands a related file to something outside Monocle — a popout
// window — instead of the related-files pane beside it, which forces a width
// on Monocle's window. One file at a time: whatever it opens shows one.

// relatedOpenTimeout bounds one run of related_open, which may start a window.
const relatedOpenTimeout = 15 * time.Second

// relatedOpenCommand is the configured related_open, or "" for none.
func (m appModel) relatedOpenCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.RelatedOpen)
	}
	return ""
}

// relatedOpenMsg reports a run of related_open: the file it was run for, and
// how many more were found with it.
type relatedOpenMsg struct {
	file relatedFile
	more int
	err  error
}

// openElsewhere runs related_open for the first of files, its path made
// absolute, in the background.
func (m appModel) openElsewhere(files []relatedFile) (appModel, tea.Cmd) {
	files = absRelated(m.repoRoot, files)
	f, more := files[0], len(files)-1
	template, owner, root := m.relatedOpenCommand(), os.Getenv("TMUX_PANE"), m.repoRoot
	m.statusBar.searchInfo = "opening " + filepath.Base(f.path)
	return m, func() tea.Msg {
		if err := execHook(expandAddCommand(template, f, owner), root, nil, relatedOpenTimeout, io.Discard); err != nil {
			return relatedOpenMsg{file: f, more: more, err: fmt.Errorf("related_open: %w", err)}
		}
		return relatedOpenMsg{file: f, more: more}
	}
}

// handleRelatedOpen says what related_open opened, or why it did not.
func (m appModel) handleRelatedOpen(msg relatedOpenMsg) appModel {
	switch {
	case msg.err != nil:
		m.statusBar.searchInfo = msg.err.Error()
	case msg.more > 0:
		m.statusBar.searchInfo = fmt.Sprintf("opened %s · %d more found", filepath.Base(msg.file.path), msg.more)
	default:
		m.statusBar.searchInfo = "opened " + filepath.Base(msg.file.path)
	}
	return m
}
