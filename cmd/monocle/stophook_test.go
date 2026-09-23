package main

import "testing"

// TestStopHookDelivers pins which verdicts the Stop hook may commit. It drains
// the queue at the end of every turn, so any verdict it does not emit has to be
// left recoverable — an acked-but-unemitted approval is consumed with nobody
// having seen it, which is how three approvals went missing.
func TestStopHookDelivers(t *testing.T) {
	cases := []struct {
		action string
		want   bool
		why    string
	}{
		{"request_changes", true, "blocks the stop and injects the feedback, so the agent sees it"},
		{"approve", false, "ends the turn silently; the agent must pull it itself"},
		{"questions", false, "ends the turn silently, like approve"},
		{"", false, "activity with no verdict — there is nothing to commit"},
		{"something_new", false, "an action this hook does not handle must not be consumed"},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			if got := stopHookDelivers(c.action); got != c.want {
				t.Errorf("stopHookDelivers(%q) = %v, want %v — %s", c.action, got, c.want, c.why)
			}
		})
	}
}
