package tui

import (
	"os"
	"testing"

	"github.com/josephschmitt/monocle/internal/testenv"
)

// Every test in this package runs against a throwaway config dir, data dir and
// database, and outside tmux. Tour tests run configured shell commands and a
// stop's side effects split and kill tmux panes, so a run from inside the
// developer's own tmux must reach neither their Monocle state nor their panes.
// Tests that need TMUX set it themselves (t.Setenv).
func TestMain(m *testing.M) {
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	testenv.Main("tui", m.Run)
}
