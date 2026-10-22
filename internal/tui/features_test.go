package tui

import (
	"slices"
	"testing"

	"github.com/josephschmitt/monocle/internal/core"
)

// What the TUI reports it understands: each feature is a command-line command
// it has, and :doc is one.
func TestFeaturesAreRealCommands(t *testing.T) {
	if !slices.Contains(Features, "doc") {
		t.Errorf("Features = %v, want doc in it", Features)
	}
	for _, f := range Features {
		if !slices.Contains(commandNames, f) {
			t.Errorf("feature %q is not a command the TUI has", f)
		}
	}
}

// onRecorder is an engine that only records which events are subscribed.
type onRecorder struct {
	core.EngineAPI
	kinds []core.EventKind
}

func (r *onRecorder) On(kind core.EventKind, _ core.EventCallback) core.UnsubscribeFunc {
	r.kinds = append(r.kinds, kind)
	return func() {}
}

// The engine's doc_pane event comes into the TUI, where it is :doc.
func TestTheDocPaneEventIsBridged(t *testing.T) {
	r := &onRecorder{}
	BridgeEngineEvents(r, nil)
	if !slices.Contains(r.kinds, core.EventDocPane) {
		t.Errorf("BridgeEngineEvents subscribes %v, not %s", r.kinds, core.EventDocPane)
	}
	m := longNoteApp(t)
	m = updateApp(t, m, docPaneEventMsg(core.EventPayload{Kind: core.EventDocPane, Status: "off"}))
	if m.docShown() {
		t.Error("the doc_pane off event left the doc pane shown")
	}
}
