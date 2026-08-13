package tui

import (
	"strings"
	"testing"
)

func TestTruncateMiddle(t *testing.T) {
	t.Run("a name that fits is untouched", func(t *testing.T) {
		if got := truncateMiddle("a.go", 10); got != "a.go" {
			t.Errorf("got %q, want %q", got, "a.go")
		}
		if got := truncateMiddle("exactly-ten", 11); got != "exactly-ten" {
			t.Errorf("an exact fit should not be truncated, got %q", got)
		}
	})

	t.Run("both ends survive", func(t *testing.T) {
		got := truncateMiddle("internal/core/engine_impl.go", 24)
		if len([]rune(got)) != 24 {
			t.Errorf("width = %d, want 24 (%q)", len([]rune(got)), got)
		}
		if !strings.HasPrefix(got, "intern") {
			t.Errorf("the head should survive, got %q", got)
		}
		if !strings.HasSuffix(got, "engine_impl.go") {
			t.Errorf("the filename should survive, got %q", got)
		}
	})

	// The case that prompted this: an artifact title's subject is at the front,
	// so dropping the head told you nothing about which artifact it was.
	t.Run("an artifact title keeps its subject", func(t *testing.T) {
		got := truncateMiddle("pseudocode: fix the minOut rounding in the swap path", 30)
		if !strings.HasPrefix(got, "pseudocode:") {
			t.Errorf("the subject should survive, got %q", got)
		}
		if !strings.Contains(got, "…") {
			t.Errorf("expected an ellipsis, got %q", got)
		}
	})

	t.Run("the tail gets the larger share", func(t *testing.T) {
		got := truncateMiddle(strings.Repeat("a", 40)+"/"+strings.Repeat("b", 40), 21)
		head := strings.Split(got, "…")[0]
		tail := strings.Split(got, "…")[1]
		if len(tail) <= len(head) {
			t.Errorf("tail (%d) should be longer than head (%d): %q", len(tail), len(head), got)
		}
	})

	t.Run("never exceeds the budget", func(t *testing.T) {
		for _, max := range []int{1, 2, 3, 4, 5, 8, 13, 40} {
			got := truncateMiddle("internal/core/engine_impl.go", max)
			if n := len([]rune(got)); n > max {
				t.Errorf("max %d produced %d cells: %q", max, n, got)
			}
		}
	})

	t.Run("degenerate widths are safe", func(t *testing.T) {
		if got := truncateMiddle("abc", 0); got != "" {
			t.Errorf("zero width should render nothing, got %q", got)
		}
		if got := truncateMiddle("abc", -1); got != "" {
			t.Errorf("negative width should render nothing, got %q", got)
		}
		if got := truncateMiddle("abcdef", 1); got != "…" {
			t.Errorf("one cell should be just the ellipsis, got %q", got)
		}
	})

	// The byte-sliced version this replaces could cut a multi-byte rune in half.
	t.Run("multi-byte names are cut on rune boundaries", func(t *testing.T) {
		got := truncateMiddle("日本語のファイル名がとても長い場合.go", 12)
		if strings.ContainsRune(got, '�') {
			t.Errorf("a rune was split: %q", got)
		}
		if n := len([]rune(got)); n > 12 {
			t.Errorf("width = %d, want at most 12: %q", n, got)
		}
	})
}
