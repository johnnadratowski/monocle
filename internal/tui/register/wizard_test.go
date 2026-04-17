package register

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/tui"
)

// fakeAdapter lets tests drive the wizard without touching real settings.json
// files. Register/Unregister are no-ops; HasConfig is toggleable.
type fakeAdapter struct {
	name      string
	label     string
	mode      adapters.IntegrationMode
	paths     []string
	hasConfig bool
	registered bool
	unregistered bool
}

func (f *fakeAdapter) Name() string                   { return f.name }
func (f *fakeAdapter) Label() string                  { return f.label }
func (f *fakeAdapter) SetMode(m adapters.IntegrationMode) { f.mode = m }
func (f *fakeAdapter) ConfigPaths(bool) []string      { return f.paths }
func (f *fakeAdapter) HasConfig(bool) bool            { return f.hasConfig }
func (f *fakeAdapter) Register(bool) error            { f.registered = true; return nil }
func (f *fakeAdapter) Unregister(bool) error          { f.unregistered = true; return nil }

func defaultOptsRegister() Options {
	return Options{
		Mode:  ModeRegister,
		Theme: tui.DefaultTheme(),
		Keys:  tui.DefaultKeyMap(),
		Adapters: []adapters.AgentAdapter{
			&fakeAdapter{name: "claude", label: "Claude Code", paths: []string{".mcp.json"}},
			&fakeAdapter{name: "opencode", label: "OpenCode", paths: []string{"opencode.json"}},
		},
	}
}

func pressKey(m Model, key string) Model {
	var msg tea.KeyPressMsg
	// Synthesize a simple single-char key press. The wizard only calls
	// .String() on the msg, so we set Text for multi-char keys.
	switch key {
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space", " ":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		msg = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		msg = tea.KeyPressMsg{Code: tea.KeyRight}
	default:
		if len(key) == 1 {
			msg = tea.KeyPressMsg{Text: key}
		} else {
			msg = tea.KeyPressMsg{Text: key}
		}
	}
	next, cmd := m.Update(msg)
	m = next.(Model)
	for cmd != nil {
		out := cmd()
		if out == nil {
			break
		}
		if _, ok := out.(tea.QuitMsg); ok {
			break
		}
		next, cmd = m.Update(out)
		m = next.(Model)
	}
	return m
}

func windowSize(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestWizard_SelectsAndAdvancesToClaudeStep(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)

	// Cursor starts on the first agent row (claude).
	m = pressKey(m, " ")

	if !m.state.selected["claude"] {
		t.Fatalf("expected claude selected after space: %+v", m.state.selected)
	}

	// Advance to claude step.
	m = pressKey(m, "enter")
	if m.state.step != StepClaude {
		t.Fatalf("expected StepClaude after enter, got %v", m.state.step)
	}
}

func TestWizard_SkipsClaudeStepWhenClaudeNotSelected(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)

	// Move down to opencode and select only it.
	m = pressKey(m, "down")
	m = pressKey(m, " ")

	m = pressKey(m, "enter")
	if m.state.step != StepConfirm {
		t.Fatalf("expected StepConfirm (skipping claude), got %v", m.state.step)
	}
}

func TestWizard_RefusesAdvanceWithoutSelection(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)
	m = pressKey(m, "enter")
	if m.state.step != StepAgents {
		t.Fatalf("expected still on StepAgents when nothing selected, got %v", m.state.step)
	}
}

func TestWizard_BackNavigationPopsHistory(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)

	m = pressKey(m, " ")     // select claude
	m = pressKey(m, "enter") // advance to claude step

	if m.state.step != StepClaude {
		t.Fatalf("setup: expected StepClaude, got %v", m.state.step)
	}
	m = pressKey(m, "shift+tab")
	if m.state.step != StepAgents {
		t.Fatalf("expected StepAgents after shift+tab, got %v", m.state.step)
	}
	if !m.state.selected["claude"] {
		t.Fatal("selection should survive back navigation")
	}
}

func TestWizard_EscCancels(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)
	m = pressKey(m, "esc")
	if !m.state.cancelled {
		t.Fatal("esc should set cancelled=true")
	}
}

func TestWizard_ClaudeToggleFlipsInstalledFlag(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)
	m = pressKey(m, " ")     // select claude (cursor starts here)
	m = pressKey(m, "enter") // to claude step

	if !m.state.planInstalled() {
		t.Fatalf("register-mode default should install plan hooks")
	}
	m = pressKey(m, " ") // toggle plan off
	if m.state.planInstalled() {
		t.Fatal("space should flip plan install off")
	}
	m = pressKey(m, "down") // move to gate
	m = pressKey(m, " ")    // toggle gate off
	if m.state.gateInstalled() {
		t.Fatal("space should flip gate install off")
	}
}

func TestWizard_LockedFlagsAnnotated(t *testing.T) {
	opts := defaultOptsRegister()
	opts.SkipPlanHook = true
	opts.SkipPlanHookLocked = true
	m := NewModel(opts)
	// Wide window so the annotation doesn't word-wrap; this test is about
	// whether the annotation is emitted, not about wrap behavior.
	m = windowSize(m, 300, 40)

	m = pressKey(m, " ")     // select claude (cursor starts here)
	m = pressKey(m, "enter") // to claude step

	if m.state.planInstalled() {
		t.Fatal("locked --no-plan-hook should mean plan NOT installed")
	}
	// toggle should be a no-op under the lock.
	m = pressKey(m, " ")
	if m.state.planInstalled() {
		t.Fatal("locked plan toggle should not flip")
	}

	view := m.View()
	s := collapseBorders(stripANSI(view.Content))
	if !strings.Contains(s, "--no-plan-hook") {
		t.Errorf("view should surface --no-plan-hook lock annotation; got:\n%s", s)
	}
}

// collapseBorders strips lipgloss box-drawing characters and collapses
// whitespace so wrapped/styled text becomes a single flat line suitable for
// substring assertions.
func collapseBorders(s string) string {
	replacer := strings.NewReplacer(
		"│", "",
		"─", "",
		"╭", "",
		"╮", "",
		"╰", "",
		"╯", "",
		"\n", " ",
		"\t", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(s)), " ")
}

func TestWizard_UnregisterMode_KeepToggles(t *testing.T) {
	opts := defaultOptsRegister()
	opts.Mode = ModeUnregister
	opts.Adapters = []adapters.AgentAdapter{
		&fakeAdapter{name: "claude", label: "Claude Code", paths: []string{".mcp.json"}, hasConfig: true},
	}
	m := NewModel(opts)
	m = windowSize(m, 120, 40)

	m = pressKey(m, " ")     // select claude (single agent, cursor on row 0)
	m = pressKey(m, "enter") // advance to claude step

	// Default: nothing "kept" — installed flag means "keep" in unregister mode.
	if m.state.planInstalled() {
		t.Fatal("unregister default should not keep plan hooks")
	}
	m = pressKey(m, " ")
	if !m.state.planInstalled() {
		t.Fatal("toggling in unregister mode should switch to keep")
	}
}

func TestWizard_ExecuteCallsAdapterMethods(t *testing.T) {
	claude := &fakeAdapter{name: "claude", label: "Claude Code", paths: []string{".mcp.json"}}
	opts := defaultOptsRegister()
	opts.Adapters = []adapters.AgentAdapter{claude}
	m := NewModel(opts)
	m = windowSize(m, 120, 40)

	m = pressKey(m, " ")     // select claude
	m = pressKey(m, "enter") // claude step
	m = pressKey(m, "enter") // confirm step
	m = pressKey(m, "enter") // execute step — this triggers registration

	if !claude.registered {
		t.Fatal("adapter.Register should have been called during execute")
	}
}

func TestWizard_ScopeToggle(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 120, 40)
	if m.state.scope {
		t.Fatal("default scope should be project (false)")
	}
	// Scope is a header toggle reachable via Tab, not a selectable row.
	m = pressKey(m, "tab")
	if !m.state.scope {
		t.Fatal("tab should flip scope to user/global")
	}
	m = pressKey(m, "tab")
	if m.state.scope {
		t.Fatal("second tab should flip back to project")
	}
}

// stripANSI removes ANSI escape sequences so test assertions can match on
// rendered strings without fighting escape codes.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' || r == 'K' || r == 'H' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
