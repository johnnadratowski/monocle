package adapters

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeAdapter is a minimal AgentAdapter used only to drive pickerModel tests.
type fakeAdapter struct {
	name, label string
	paths       []string
}

func (f *fakeAdapter) Name() string             { return f.name }
func (f *fakeAdapter) Label() string            { return f.label }
func (f *fakeAdapter) Detect() bool             { return true }
func (f *fakeAdapter) Register(global bool) error   { return nil }
func (f *fakeAdapter) Unregister(global bool) error { return nil }
func (f *fakeAdapter) HasConfig(global bool) bool   { return false }
func (f *fakeAdapter) ConfigPaths(global bool) []string { return f.paths }
func (f *fakeAdapter) NeedsRegister() bool       { return true }
func (f *fakeAdapter) SetMode(m IntegrationMode) {}

// keyCodes maps the short names used by pickerModel.Update to the Key.Code
// rune that produces the same msg.String() value in bubbletea v2.
var keyCodes = map[string]rune{
	" ":    tea.KeySpace,
	"down": tea.KeyDown,
	"up":   tea.KeyUp,
}

func keyPress(name string) tea.KeyPressMsg {
	if code, ok := keyCodes[name]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	// single-character keys ("j", "k", "a", etc.)
	runes := []rune(name)
	return tea.KeyPressMsg{Code: runes[0]}
}

func newPicker(agents ...AgentAdapter) pickerModel {
	return pickerModel{
		agents:          agents,
		selected:        map[int]bool{},
		planHookEnabled: true,
		title:           "test",
	}
}

func press(m pickerModel, key string) pickerModel {
	next, _ := m.Update(keyPress(key))
	return next.(pickerModel)
}

func TestPicker_SubRowHiddenWhenClaudeNotSelected(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	m := newPicker(claude)

	if m.planHookVisible() {
		t.Fatal("sub-row should be hidden before Claude is selected")
	}
}

func TestPicker_SelectingClaudeRevealsSubRow(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	m := newPicker(claude)
	m = press(m, " ")

	if !m.selected[0] {
		t.Fatal("Claude should be selected after space")
	}
	if !m.planHookVisible() {
		t.Fatal("sub-row should be visible once Claude is selected")
	}
	if !m.planHookEnabled {
		t.Fatal("sub-row should be pre-checked (enabled) by default")
	}
}

func TestPicker_SubRowTogglesIndependently(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	other := &fakeAdapter{name: "other", label: "Other"}
	m := newPicker(claude, other)

	m = press(m, " ")     // select Claude (cursor at 0)
	m = press(m, "down")  // cursor moves onto sub-row
	if !m.cursorOnPlanHook {
		t.Fatal("cursor should land on sub-row after Claude is selected")
	}
	m = press(m, " ") // toggle sub-row off
	if m.planHookEnabled {
		t.Fatal("sub-row should be unchecked after toggle")
	}
	if !m.selected[0] {
		t.Fatal("toggling the sub-row must not affect Claude's selection")
	}
}

func TestPicker_UncheckingClaudeHidesSubRow(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	m := newPicker(claude)

	m = press(m, " ") // select Claude
	if !m.planHookVisible() {
		t.Fatal("precondition: sub-row visible")
	}
	m = press(m, " ") // deselect Claude
	if m.planHookVisible() {
		t.Fatal("sub-row should be hidden after Claude is unchecked")
	}
}

func TestPicker_RecheckingClaudeResetsSubRowDefault(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	m := newPicker(claude)

	m = press(m, " ")    // select Claude
	m = press(m, "down") // onto sub-row
	m = press(m, " ")    // turn sub-row off
	m = press(m, "up")   // back to Claude
	m = press(m, " ")    // uncheck Claude
	m = press(m, " ")    // re-check Claude

	if !m.planHookEnabled {
		t.Fatal("sub-row should reset to enabled when Claude is re-checked")
	}
}

func TestPicker_NavigationSkipsSubRowWhenHidden(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	other := &fakeAdapter{name: "other", label: "Other"}
	m := newPicker(claude, other)

	// Claude is unchecked, so down should go straight to the second agent.
	m = press(m, "down")
	if m.cursorOnPlanHook {
		t.Fatal("cursor should not land on hidden sub-row")
	}
	if m.cursor != 1 {
		t.Fatalf("cursor should be on second agent, got %d", m.cursor)
	}
}

func TestPicker_NavigationTraversesSubRowWhenVisible(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude"}
	other := &fakeAdapter{name: "other", label: "Other"}
	m := newPicker(claude, other)

	m = press(m, " ")    // select Claude → sub-row visible
	m = press(m, "down") // onto sub-row
	if !m.cursorOnPlanHook {
		t.Fatalf("down from Claude should land on sub-row, got cursor=%d onSub=%v", m.cursor, m.cursorOnPlanHook)
	}
	m = press(m, "down") // onto Other
	if m.cursorOnPlanHook || m.cursor != 1 {
		t.Fatalf("down from sub-row should land on Other, got cursor=%d onSub=%v", m.cursor, m.cursorOnPlanHook)
	}
	m = press(m, "up") // back to sub-row
	if !m.cursorOnPlanHook {
		t.Fatal("up from Other should return to sub-row")
	}
	m = press(m, "up") // back to Claude
	if m.cursorOnPlanHook || m.cursor != 0 {
		t.Fatalf("up from sub-row should land on Claude, got cursor=%d onSub=%v", m.cursor, m.cursorOnPlanHook)
	}
}
