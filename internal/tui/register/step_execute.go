package register

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
		res := AgentResult{Name: name, Label: label, Paths: paths}
		if mode == ModeRegister {
			wasRegistered := a.HasConfig(global)
			if err := a.Register(global); err != nil {
				res.Err = err
				return agentFinishedMsg{index: idx, result: res}
			}
			res.Action = "registered"
			if wasRegistered {
				res.Action = "updated"
			}
		} else {
			if !a.HasConfig(global) {
				res.Action = "nothing"
				return agentFinishedMsg{index: idx, result: res}
			}
			if err := a.Unregister(global); err != nil {
				res.Err = err
				return agentFinishedMsg{index: idx, result: res}
			}
			res.Action = "removed"
		}
		return agentFinishedMsg{index: idx, result: res}
	}
}

// viewExecute renders per-agent progress.
func viewExecute(s WizardState) string {
	var b strings.Builder

	title := executeTitleRegister
	if s.mode == ModeUnregister {
		title = executeTitleUnregister
	}
	b.WriteString(styleBold.Render(title) + "\n\n")

	selected := s.selectedAdapters()
	for i, a := range selected {
		var glyph, desc string
		finished := i < len(s.results) && (s.results[i].Action != "" || s.results[i].Err != nil)
		if finished {
			r := s.results[i]
			if r.Err != nil {
				glyph = styleBad.Render("✗")
				desc = styleBad.Render("error: " + r.Err.Error())
			} else {
				glyph = styleOk.Render("✓")
				desc = styleFaint.Render(r.Action)
			}
		} else if i == s.runIndex {
			glyph = styleFaint.Render("…")
			desc = styleFaint.Render("working")
		} else {
			glyph = styleFaint.Render(" ")
			desc = ""
		}
		b.WriteString(fmt.Sprintf("  %s %s  %s\n", glyph, a.Label(), desc))
		if i < len(s.results) && s.results[i].Err == nil && s.results[i].Action != "" && s.results[i].Action != "nothing" {
			for _, p := range s.results[i].Paths {
				b.WriteString("      ")
				b.WriteString(styleFaint.Render("→ " + p))
				b.WriteString("\n")
			}
		}
	}

	if s.runIndex >= len(selected) && len(selected) > 0 {
		b.WriteString("\n")
		b.WriteString(styleFaint.Render(executeDone))
	}
	return b.String()
}
