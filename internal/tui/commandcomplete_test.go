package tui

import "testing"

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"mark-all-reviewed", "mark-all-unreviewed"}, "mark-all-"},
		{[]string{"submit", "submit!"}, "submit"},
		{[]string{"clear"}, "clear"},
		{[]string{"pause", "unpause"}, ""},
	}
	for _, c := range cases {
		if got := longestCommonPrefix(c.in); got != c.want {
			t.Errorf("longestCommonPrefix(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchingCommands(t *testing.T) {
	if got := matchingCommands("base-"); len(got) != 2 {
		t.Errorf("base- should match base-artifact-version and base-ref, got %v", got)
	}
	if got := matchingCommands("cancel"); len(got) != 1 || got[0] != "cancel-feedback" {
		t.Errorf("cancel → %v, want [cancel-feedback]", got)
	}
	if got := matchingCommands("zzz"); len(got) != 0 {
		t.Errorf("no match expected, got %v", got)
	}
}

func TestCompleteCommand(t *testing.T) {
	// Unique prefix completes fully.
	m := appModel{}
	m.commandBuffer = "canc"
	m = m.completeCommand()
	if m.commandBuffer != "cancel-feedback" {
		t.Errorf("unique: got %q", m.commandBuffer)
	}

	// Ambiguous prefix completes to the common prefix, then cycles.
	m = appModel{}
	m.commandBuffer = "mark"
	m = m.completeCommand()
	if m.commandBuffer != "mark-all-" {
		t.Fatalf("common prefix: got %q", m.commandBuffer)
	}
	m = m.completeCommand() // no further prefix → cycle to first match
	first := m.commandBuffer
	m = m.completeCommand() // → second match
	second := m.commandBuffer
	if first == second || first == "mark-all-" {
		t.Errorf("expected cycling through matches, got %q then %q", first, second)
	}
	if m.statusBar.commandHint == "" {
		t.Error("expected a candidate hint when multiple matches")
	}

	// Once an argument is being typed (space present), Tab is a no-op.
	m = appModel{}
	m.commandBuffer = "theme da"
	m = m.completeCommand()
	if m.commandBuffer != "theme da" {
		t.Errorf("should not complete args, got %q", m.commandBuffer)
	}
}
