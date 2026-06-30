package tui

import (
	"strings"
	"testing"
)

func TestTmuxOpenArgs(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0") // pretend we're inside tmux
	argv := []string{"nvim", "+42", "pkg/util.go"}

	cases := []struct {
		name    string
		mode    string
		focus   bool
		wantSub string   // first tmux arg
		wantHas []string // flags expected
		wantNo  []string // flags that must NOT be present
	}{
		{"vertical focus", "tmux_vertical", true, "split-window", []string{"-h"}, []string{"-d"}},
		{"horizontal nofocus", "tmux_horizontal", false, "split-window", []string{"-v", "-d"}, nil},
		{"window focus", "tmux_window", true, "new-window", nil, []string{"-d", "-h", "-v"}},
		{"window nofocus", "tmux_window", false, "new-window", []string{"-d"}, []string{"-h", "-v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tmuxOpenArgs(tc.mode, tc.focus, argv)
			if !ok {
				t.Fatalf("expected tmux args for mode %q", tc.mode)
			}
			if got[0] != tc.wantSub {
				t.Errorf("subcommand = %q, want %q", got[0], tc.wantSub)
			}
			joined := strings.Join(got, " ")
			for _, f := range tc.wantHas {
				if !containsArg(got, f) {
					t.Errorf("expected flag %q in %v", f, got)
				}
			}
			for _, f := range tc.wantNo {
				if containsArg(got, f) {
					t.Errorf("did not expect flag %q in %v", f, got)
				}
			}
			// The editor invocation is passed as the final shell-command argument.
			last := got[len(got)-1]
			if !strings.Contains(last, "nvim") || !strings.Contains(last, "+42") || !strings.Contains(last, "pkg/util.go") {
				t.Errorf("shell-command arg missing editor invocation: %q (full: %q)", last, joined)
			}
		})
	}
}

func TestTmuxOpenArgs_FallsBack(t *testing.T) {
	t.Setenv("TMUX", "session") // inside tmux, but terminal mode → no tmux args
	if _, ok := tmuxOpenArgs("terminal", true, []string{"vi", "f"}); ok {
		t.Error("terminal mode should not produce tmux args")
	}

	t.Setenv("TMUX", "") // not inside tmux → fall back regardless of mode
	if _, ok := tmuxOpenArgs("tmux_vertical", true, []string{"vi", "f"}); ok {
		t.Error("outside tmux, tmux modes must fall back (ok=false)")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain.go":          "plain.go",
		"pkg/util.go":       "pkg/util.go",
		"+42":               "+42",
		"a file.go":         `'a file.go'`,
		"it's.go":           `'it'\''s.go'`,
		"":                  "''",
		"weird;rm -rf.go":   `'weird;rm -rf.go'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
