package tui

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// The tour's files are all new: no base side, as an added file's answer is.
func (e *tourEngine) GetBaseFileContent(path string) (string, error) { return "", nil }

// commentsApp is tourApp with a comments_command recording each run's state.
func commentsApp(t *testing.T) (appModel, string) {
	t.Helper()
	skipWithoutSh(t)
	out := filepath.Join(t.TempDir(), "runs")
	m, _ := tourAppWith(t, testTour(), &types.Config{CommentsCommand: `printf '%s\n' "$MONOCLE_COMMENTS" >> ` + out})
	m.repoRoot = t.TempDir()
	return m, out
}

// pressWait is pressKey, waiting long enough for the comments_command it may run.
func pressWait(t *testing.T, m appModel, key string) appModel {
	t.Helper()
	m = pressKey(t, m, key)
	time.Sleep(300 * time.Millisecond)
	return m
}

// In a walkthrough m is the tour's comment switch (John 2026-10-27: "a global
// thing in tour, where it hides for both and uses the same key from both
// windows"): shown and hidden in turn, the walkthrough told each time.
func TestTheCommentsKeyTogglesTheSharedStateAndSaysSo(t *testing.T) {
	m, out := commentsApp(t)
	m = pressWait(t, m, "m")
	if m.diffView.commentFilter != commentsHidden {
		t.Fatalf("m in a walkthrough: filter %d, want hidden", m.diffView.commentFilter)
	}
	m = pressWait(t, m, "m")
	if m.diffView.commentFilter != commentsShown {
		t.Fatalf("m again: filter %d, want shown", m.diffView.commentFilter)
	}
	time.Sleep(200 * time.Millisecond)
	if got := runsIn(t, out); !reflect.DeepEqual(got, []string{"off", "on"}) {
		t.Errorf("comments_command runs %q, want off then on", got)
	}
	// M keeps the three-state cycle, Monocle's own: dimming is never shared.
	m = pressWait(t, m, "M")
	if m.diffView.commentFilter != commentsHidden {
		t.Errorf("M from shown: filter %d, want hidden (the cycle backwards)", m.diffView.commentFilter)
	}
	if got := runsIn(t, out); len(got) != 2 {
		t.Errorf("M ran comments_command: %q", got)
	}
}

// Without comments_command (no walkthrough sharing it), m cycles shown → dimmed
// → hidden as before.
func TestTheCommentsKeyCyclesWithoutAWalkthrough(t *testing.T) {
	m, _ := tourApp(t)
	m = pressKey(t, m, "m")
	if m.diffView.commentFilter != commentsDimmed {
		t.Errorf("m with no comments_command: filter %d, want dimmed", m.diffView.commentFilter)
	}
}

// The walkthrough's push (`monocle review comments`, the engine's comments
// event) sets the state and tells no one back: no echo of a push.
func TestAPushedStateIsSetAndNotEchoed(t *testing.T) {
	m, out := commentsApp(t)
	m = updateApp(t, m, commentsEventMsg(core.EventPayload{Kind: core.EventComments, Status: "off"}))
	if m.diffView.commentFilter != commentsHidden {
		t.Errorf("pushed off: filter %d, want hidden", m.diffView.commentFilter)
	}
	m = updateApp(t, m, commentsEventMsg(core.EventPayload{Kind: core.EventComments, Status: "off"}))
	if m.diffView.commentFilter != commentsHidden {
		t.Errorf("pushed off twice: filter %d, want hidden still", m.diffView.commentFilter)
	}
	m = updateApp(t, m, commentsMsg{arg: "toggle"})
	if m.diffView.commentFilter != commentsShown {
		t.Errorf(":comments toggle from hidden: filter %d, want shown", m.diffView.commentFilter)
	}
	// on leaves a dimmed view dimmed: dimmed comments are shown.
	m.diffView.setCommentFilter(commentsDimmed)
	m = updateApp(t, m, commentsMsg{arg: "on"})
	if m.diffView.commentFilter != commentsDimmed {
		t.Errorf("on while dimmed: filter %d, want dimmed", m.diffView.commentFilter)
	}
	time.Sleep(200 * time.Millisecond)
	if got := runsIn(t, out); len(got) != 0 {
		t.Errorf("a pushed state ran comments_command: %q", got)
	}
}

// The state holds across stops.
func TestHiddenCommentsHoldAcrossStops(t *testing.T) {
	m, _ := commentsApp(t)
	m = updateApp(t, m, commentsMsg{arg: "off"})
	m = updateApp(t, m, gotoLineMsg{path: "b.go", line: 12})
	if m.diffView.commentFilter != commentsHidden {
		t.Errorf("another file: filter %d, want hidden still", m.diffView.commentFilter)
	}
}

// The TUI says it understands :comments, and the engine's event comes into it.
func TestTheCommentsEventIsBridgedAndReported(t *testing.T) {
	if !slices.Contains(Features, "comments") || !slices.Contains(commandNames, "comments") {
		t.Errorf("Features %v / commands: comments missing", Features)
	}
	r := &onRecorder{}
	BridgeEngineEvents(r, nil)
	if !slices.Contains(r.kinds, core.EventComments) {
		t.Errorf("BridgeEngineEvents subscribes %v, not %s", r.kinds, core.EventComments)
	}
}
