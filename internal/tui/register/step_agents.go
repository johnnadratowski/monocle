package register

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/tui"
)

// updateAgents handles key input for the Agents + Scope step. The cursor
// moves only through the agent rows (0..N-1); the scope row is a header
// toggle manipulated via Tab, not a selectable row.
//
// Tab: toggle scope (Project ↔ User).
// Enter: advance to the next step when at least one agent is selected.
// Space: toggle the agent row under the cursor.
//
// Integration mode (MCP tools vs skills) is resolved from the --integration-
// mode flag or per-adapter defaults — not exposed in the TUI.
func updateAgents(m Model, key string) (tea.Model, tea.Cmd) {
	s := m.state

	switch {
	case tui.Matches(key, s.keys.Up):
		if s.agentCursor > 0 {
			s.agentCursor--
		}
	case tui.Matches(key, s.keys.Down):
		if s.agentCursor < len(s.adapters)-1 {
			s.agentCursor++
		}
	case key == "tab":
		if !s.opts.GlobalLocked {
			s.scope = !s.scope
		}
	case tui.Matches(key, s.keys.WizardToggle):
		if s.agentCursor >= 0 && s.agentCursor < len(s.adapters) {
			name := s.adapters[s.agentCursor].Name()
			s.selected[name] = !s.selected[name]
		}
	case tui.Matches(key, s.keys.WizardAdvance):
		if s.anySelected() {
			m.state = s
			return m, advanceCmd()
		}
	}
	m.state = s
	return m, nil
}

// viewAgents renders the Agents + Scope step.
func viewAgents(s WizardState) string {
	var b strings.Builder

	intro := agentsIntroRegister
	if s.mode == ModeUnregister {
		intro = agentsIntroUnregister
	}
	b.WriteString(intro + "\n\n")

	// Scope row.
	b.WriteString(renderScopeRow(s))
	b.WriteString("\n\n")

	if len(s.adapters) == 0 && s.mode == ModeUnregister {
		b.WriteString("\n")
		b.WriteString(styleFaint.Render(agentsEmptyUnregister))
		b.WriteString("\n")
		return b.String()
	}

	// Agent rows.
	for i, a := range s.adapters {
		onRow := s.agentCursor == i
		b.WriteString(renderAgentRow(s, a, onRow))
	}
	return b.String()
}

// Scope tab colors. Use the same 16-color ANSI palette the main TUI uses
// for its submission modal tabs (see reviewsummary.go) so the wizard looks
// native. Blue + green read as "two distinct choices" without either
// suggesting approval/rejection.
var (
	scopeUserColor    color.Color = lipgloss.Color("4") // blue
	scopeProjectColor color.Color = lipgloss.Color("2") // green
)

func renderScopeRow(s WizardState) string {
	userStr := scopeTab(scopeUser, s.scope, scopeUserColor)
	projStr := scopeTab(scopeProject, !s.scope, scopeProjectColor)

	var b strings.Builder
	b.WriteString(styleBold.Render(scopeLabel))
	b.WriteString(" ")
	b.WriteString(projStr + " " + userStr)
	if s.opts.GlobalLocked {
		b.WriteString("  " + lockNote("--global"))
	} else {
		b.WriteString("  " + styleFaint.Render("(Tab)"))
	}
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(scopeHelp))
	return b.String()
}

// scopeTab renders one of the scope-picker tab chips in its colored style.
func scopeTab(label string, active bool, tint color.Color) string {
	if active {
		return lipgloss.NewStyle().
			Background(tint).
			Foreground(lipgloss.Color("0")).
			Bold(true).
			Padding(0, 1).
			Render(label)
	}
	return lipgloss.NewStyle().
		Foreground(tint).
		Padding(0, 1).
		Render(label)
}

func renderAgentRow(s WizardState, a interface {
	Name() string
	Label() string
	ConfigPaths(bool) []string
}, onRow bool) string {
	label := highlightLabel(a.Label(), onRow)
	paths := a.ConfigPaths(s.scope)
	var pathSummary string
	if len(paths) > 0 {
		if len(paths) == 1 {
			pathSummary = styleFaint.Render("(" + paths[0] + ")")
		} else {
			pathSummary = styleFaint.Render(fmt.Sprintf("(%s + %d more)", paths[0], len(paths)-1))
		}
	}

	checked := s.selected[a.Name()]
	b := strings.Builder{}
	fmt.Fprintf(&b, "%s%s %s %s\n", rowCursor(onRow), checkbox(checked, false), label, pathSummary)

	// Full path list on the focused row so users see exactly what's touched.
	if onRow {
		for _, p := range paths {
			b.WriteString("       ")
			b.WriteString(styleFaint.Render("→ " + p))
			b.WriteString("\n")
		}
	}
	return b.String()
}
