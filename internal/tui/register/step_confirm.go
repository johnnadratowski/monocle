package register

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

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
func viewConfirm(s WizardState) string {
	var b strings.Builder

	title := confirmTitleRegister
	help := confirmHelpRegister
	prefix := "+ "
	if s.mode == ModeUnregister {
		title = confirmTitleUnregister
		help = confirmHelpUnregister
		prefix = "- "
	}
	b.WriteString(styleBold.Render(title) + "\n\n")
	b.WriteString(styleFaint.Render(help) + "\n\n")

	scope := "project"
	if s.scope {
		scope = "user"
	}
	fmt.Fprintf(&b, "Scope: %s\n\n", styleBold.Render(scope))

	for _, a := range s.selectedAdapters() {
		applyAdapterConfiguration(s, a)
		b.WriteString(styleBold.Render(a.Label()))
		if s.mode == ModeRegister {
			b.WriteString("  ")
			fmt.Fprintf(&b, "%s", styleFaint.Render("("+describeMode(s, a)+")"))
		}
		b.WriteString("\n")
		for _, p := range a.ConfigPaths(s.scope) {
			b.WriteString("  ")
			b.WriteString(styleFaint.Render(prefix + p))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(styleFaint.Render("Press enter to proceed."))
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
