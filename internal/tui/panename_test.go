package tui

import "testing"

// Six Monocle panes tiled in one window all said "monocle". The header's job is
// to say which one you are looking at.
func TestPaneName(t *testing.T) {
	cases := []struct {
		name     string
		label    string
		repoRoot string
		want     string
	}{
		{"the agent's label wins", "ott (2)", "/w/feature-2", "ott (2)"},
		{"no label falls back to the worktree", "", "/w/feature-2", "feature-2"},
		{"no label and no repo is the product name", "", "", "monocle"},
		{"a root path is no name at all", "", "/", "monocle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := appModel{agentLabel: tc.label, repoRoot: tc.repoRoot}
			if got := m.paneName(); got != tc.want {
				t.Errorf("paneName = %q, want %q", got, tc.want)
			}
		})
	}
}
