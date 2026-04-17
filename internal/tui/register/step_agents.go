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
	case tui.Matches(key, s.keys.Up) || key == "up":
		if s.agentCursor > 0 {
			s.agentCursor--
		}
	case tui.Matches(key, s.keys.Down) || key == "down":
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
	case key == "enter":
		if s.anySelected() {
			m.state = s
			return m, advanceCmd()
		}
	}
	m.state = s
	return m, nil
}

// viewAgents renders the Agents + Scope step.
func viewAgents(s WizardState, width int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)
	bold := lipgloss.NewStyle().Bold(true)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

	intro := agentsIntroRegister
	if s.mode == ModeUnregister {
		intro = agentsIntroUnregister
	}
	b.WriteString(intro + "\n\n")

	// Scope row.
	b.WriteString(renderScopeRow(s, cursorStyle, bold, faint))
	b.WriteString("\n\n")

	if len(s.adapters) == 0 && s.mode == ModeUnregister {
		b.WriteString("\n")
		b.WriteString(faint.Render(agentsEmptyUnregister))
		b.WriteString("\n")
		return b.String()
	}

	// Agent rows.
	for i, a := range s.adapters {
		onRow := s.agentCursor == i
		b.WriteString(renderAgentRow(s, i, a, onRow, cursorStyle, bold, faint))
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

func renderScopeRow(s WizardState, cursorStyle, bold, faint lipgloss.Style) string {
	tab := func(label string, active bool, tint color.Color) string {
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
	userStr := tab(scopeUser, s.scope, scopeUserColor)
	projStr := tab(scopeProject, !s.scope, scopeProjectColor)

	var b strings.Builder
	b.WriteString(bold.Render(scopeLabel))
	b.WriteString(" ")
	b.WriteString(projStr + " " + userStr)
	if s.opts.GlobalLocked {
		b.WriteString("  " + lockNote("--global"))
	} else {
		b.WriteString("  " + faint.Render("(Tab)"))
	}
	b.WriteString("\n")
	b.WriteString(faint.Render(scopeHelp))
	return b.String()
}

func renderAgentRow(s WizardState, i int, a interface {
	Name() string
	Label() string
	ConfigPaths(bool) []string
}, onRow bool, cursorStyle, bold, faint lipgloss.Style) string {
	cur := rowCursor(onRow)
	label := highlightLabel(a.Label(), onRow)
	paths := a.ConfigPaths(s.scope)
	var pathSummary string
	if len(paths) > 0 {
		if len(paths) == 1 {
			pathSummary = faint.Render("(" + paths[0] + ")")
		} else {
			pathSummary = faint.Render(fmt.Sprintf("(%s + %d more)", paths[0], len(paths)-1))
		}
	}

	checked := s.selected[a.Name()]
	line := fmt.Sprintf("%s%s %s %s", cur, checkbox(checked, false), label, pathSummary)
	b := strings.Builder{}
	b.WriteString(line)
	b.WriteString("\n")

	// Full path list on the focused row so users see exactly what's touched.
	if onRow {
		for _, p := range paths {
			b.WriteString("       ")
			b.WriteString(faint.Render("→ " + p))
			b.WriteString("\n")
		}
	}
	return b.String()
}
