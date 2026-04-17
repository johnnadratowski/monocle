package register

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// enterExecute applies wizard state to each selected adapter and kicks off
// the first registration.
func enterExecute(s *WizardState) tea.Cmd {
	// Prepare adapters before the first run (ConfigPaths is rendered in the
	// execute view from the same adapter state).
	for _, a := range s.selectedAdapters() {
		applyAdapterConfiguration(*s, a)
	}
	s.runIndex = 0
	return func() tea.Msg { return runNextMsg{} }
}

// runAgent invokes adapter.Register or adapter.Unregister for s.runIndex and
// emits an agentFinishedMsg with the outcome.
func runAgent(s *WizardState) tea.Cmd {
	selected := s.selectedAdapters()
	if s.runIndex >= len(selected) {
		return func() tea.Msg { return executeDoneMsg{} }
	}
	idx := s.runIndex
	a := selected[idx]
	global := s.scope
	mode := s.mode
	label := a.Label()
	name := a.Name()
	paths := a.ConfigPaths(global)

	return func() tea.Msg {
		res := agentResult{name: name, label: label, paths: paths}
		if mode == ModeRegister {
			wasRegistered := a.HasConfig(global)
			if err := a.Register(global); err != nil {
				res.err = err
				return agentFinishedMsg{index: idx, result: res}
			}
			res.action = "registered"
			if wasRegistered {
				res.action = "updated"
			}
		} else {
			if !a.HasConfig(global) {
				res.action = "nothing"
				return agentFinishedMsg{index: idx, result: res}
			}
			if err := a.Unregister(global); err != nil {
				res.err = err
				return agentFinishedMsg{index: idx, result: res}
			}
			res.action = "removed"
		}
		return agentFinishedMsg{index: idx, result: res}
	}
}

// viewExecute renders per-agent progress.
func viewExecute(s WizardState, width int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)
	bold := lipgloss.NewStyle().Bold(true)
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	bad := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	title := executeTitleRegister
	if s.mode == ModeUnregister {
		title = executeTitleUnregister
	}
	b.WriteString(bold.Render(title) + "\n\n")

	selected := s.selectedAdapters()
	for i, a := range selected {
		var glyph, desc string
		if i < len(s.results) && s.results[i].action != "" || (i < len(s.results) && s.results[i].err != nil) {
			r := s.results[i]
			if r.err != nil {
				glyph = bad.Render("✗")
				desc = bad.Render("error: " + r.err.Error())
			} else {
				glyph = ok.Render("✓")
				desc = faint.Render(r.action)
			}
		} else if i == s.runIndex {
			glyph = faint.Render("…")
			desc = faint.Render("working")
		} else {
			glyph = faint.Render(" ")
			desc = ""
		}
		b.WriteString(fmt.Sprintf("  %s %s  %s\n", glyph, a.Label(), desc))
		if i < len(s.results) && s.results[i].err == nil && s.results[i].action != "" && s.results[i].action != "nothing" {
			for _, p := range s.results[i].paths {
				b.WriteString("      ")
				b.WriteString(faint.Render("→ " + p))
				b.WriteString("\n")
			}
		}
	}

	if s.runIndex >= len(selected) && len(selected) > 0 {
		b.WriteString("\n")
		if s.mode == ModeUnregister {
			b.WriteString(faint.Render(executeDoneUnregister))
		} else {
			b.WriteString(faint.Render(executeDoneRegister))
		}
	}
	return b.String()
}
