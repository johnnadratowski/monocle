package tui

import (
	"slices"
	"testing"
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
