package register

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/tui"
)

// updateConfirm handles input on the confirm step. Enter runs execute.
func updateConfirm(m Model, key string) (tea.Model, tea.Cmd) {
	s := m.state
	if tui.Matches(key, s.keys.WizardAdvance) {
		m.state = s
		return m, advanceCmd()
	}
	return m, nil
}

// viewConfirm renders the pre-execute summary.
func viewConfirm(s WizardState, width int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)
	bold := lipgloss.NewStyle().Bold(true)

	title := confirmTitleRegister
	help := confirmHelpRegister
	if s.mode == ModeUnregister {
		title = confirmTitleUnregister
		help = confirmHelpUnregister
	}
	b.WriteString(bold.Render(title) + "\n\n")
	b.WriteString(faint.Render(help) + "\n\n")

	scope := "project"
	if s.scope {
		scope = "user"
	}
	b.WriteString(fmt.Sprintf("Scope: %s\n\n", bold.Render(scope)))

	for _, a := range s.selectedAdapters() {
		applyAdapterConfiguration(s, a)
		b.WriteString(bold.Render(a.Label()))
		if s.mode == ModeRegister {
			b.WriteString("  ")
			b.WriteString(faint.Render(fmt.Sprintf("(%s)", describeMode(s, a))))
		}
		b.WriteString("\n")
		paths := a.ConfigPaths(s.scope)
		if s.mode == ModeRegister {
			for _, p := range paths {
				b.WriteString("  ")
				b.WriteString(faint.Render("+ " + p))
				b.WriteString("\n")
			}
		} else {
			for _, p := range paths {
				b.WriteString("  ")
				b.WriteString(faint.Render("- " + p))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(faint.Render("Press enter to proceed."))
	return b.String()
}

func describeMode(s WizardState, a adapters.AgentAdapter) string {
	mode := s.resolveIntegrationModeForAgent(a)
	if mode == adapters.ModeMCPTools {
		return "mcp tools"
	}
	return "skills"
}

// applyAdapterConfiguration mutates `a` to match the wizard state so that
// ConfigPaths reflects the right output. For ClaudeAdapter this sets Mode
// and the Skip*/Keep* booleans before we render its paths.
func applyAdapterConfiguration(s WizardState, a adapters.AgentAdapter) {
	a.SetMode(s.resolveIntegrationModeForAgent(a))
	if claude, ok := a.(*adapters.ClaudeAdapter); ok {
		if s.mode == ModeRegister {
			claude.SkipPlanHook = s.planToggle
			claude.SkipReviewGate = s.gateToggle
		} else {
			claude.KeepPlanHook = s.planToggle
			claude.KeepReviewGate = s.gateToggle
		}
	}
}
