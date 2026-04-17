package adapters

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PickAgents shows an interactive multi-select picker and returns the selected adapters.
// The title is shown at the top of the picker (e.g. "Select agents to register").
//
// Side effect: when the Claude adapter is included and its nested "install plan
// review hooks" sub-option is unchecked, the adapter's SkipPlanHook field is set
// to true on the returned instance so that Register() skips hook installation.
func PickAgents(agents []AgentAdapter, title string) ([]AgentAdapter, error) {
	m := pickerModel{
		agents:          agents,
		selected:        make(map[int]bool),
		title:           title,
		planHookEnabled: true,
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("picker: %w", err)
	}
	result := final.(pickerModel)
	if result.cancelled {
		return nil, nil
	}

	var picked []AgentAdapter
	for i, a := range agents {
		if result.selected[i] {
			picked = append(picked, a)
		}
	}

	// Apply the nested plan-hook toggle back to the Claude adapter.
	if !result.planHookEnabled {
		for _, a := range picked {
			if claude, ok := a.(*ClaudeAdapter); ok {
				claude.SkipPlanHook = true
			}
		}
	}
	return picked, nil
}

type pickerModel struct {
	agents    []AgentAdapter
	selected  map[int]bool
	cursor    int
	cancelled bool
	title     string

	// planHookEnabled tracks the nested "install plan review hooks" sub-option
	// that appears under the Claude row when Claude is selected. Default true;
	// re-checking Claude after unchecking resets this to true.
	planHookEnabled bool

	// cursorOnPlanHook is true when the cursor is positioned on the sub-row
	// rather than on an agent row.
	cursorOnPlanHook bool
}

func (m pickerModel) Init() tea.Cmd { return nil }

// claudeIndex returns the index of the Claude adapter in m.agents, or -1.
func (m pickerModel) claudeIndex() int {
	for i, a := range m.agents {
		if a.Name() == "claude" {
			return i
		}
	}
	return -1
}

// planHookVisible reports whether the nested sub-row is currently part of
// the navigable list (i.e. Claude exists and is checked).
func (m pickerModel) planHookVisible() bool {
	idx := m.claudeIndex()
	return idx >= 0 && m.selected[idx]
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			m = m.moveCursor(+1)
		case "k", "up":
			m = m.moveCursor(-1)
		case "space", " ":
			if m.cursorOnPlanHook {
				m.planHookEnabled = !m.planHookEnabled
			} else {
				m.selected[m.cursor] = !m.selected[m.cursor]
				// Re-checking Claude resets the sub-option to its default.
				if m.cursor == m.claudeIndex() && m.selected[m.cursor] {
					m.planHookEnabled = true
				}
				// Unchecking Claude hides the sub-row; pull cursor back if it was there.
				if !m.planHookVisible() && m.cursorOnPlanHook {
					m.cursorOnPlanHook = false
				}
			}
		case "a":
			allSelected := true
			for i := range m.agents {
				if !m.selected[i] {
					allSelected = false
					break
				}
			}
			for i := range m.agents {
				m.selected[i] = !allSelected
			}
			if !m.planHookVisible() && m.cursorOnPlanHook {
				m.cursorOnPlanHook = false
			}
		case "enter":
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// moveCursor advances the logical cursor by delta through the visible rows
// (agent rows + the optional plan-hook sub-row directly after Claude).
func (m pickerModel) moveCursor(delta int) pickerModel {
	claudeIdx := m.claudeIndex()
	subVisible := m.planHookVisible()

	if delta > 0 {
		if m.cursorOnPlanHook {
			if m.cursor < len(m.agents)-1 {
				m.cursor++
				m.cursorOnPlanHook = false
			}
			return m
		}
		if m.cursor == claudeIdx && subVisible {
			m.cursorOnPlanHook = true
			return m
		}
		if m.cursor < len(m.agents)-1 {
			m.cursor++
		}
		return m
	}

	// delta < 0
	if m.cursorOnPlanHook {
		m.cursorOnPlanHook = false
		return m
	}
	if m.cursor == claudeIdx+1 && subVisible {
		m.cursor = claudeIdx
		m.cursorOnPlanHook = true
		return m
	}
	if m.cursor > 0 {
		m.cursor--
	}
	return m
}

func (m pickerModel) View() tea.View {
	dim := lipgloss.NewStyle().Faint(true)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title))
	b.WriteString("\n\n")

	claudeIdx := m.claudeIndex()

	for i, a := range m.agents {
		onAgent := i == m.cursor && !m.cursorOnPlanHook
		cursor := "  "
		if onAgent {
			cursor = cursorStyle.Render("> ")
		}

		check := "[ ]"
		if m.selected[i] {
			check = checkStyle.Render("[x]")
		}

		name := a.Label()
		if onAgent {
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}

		paths := a.ConfigPaths(false)

		if onAgent {
			// Expanded: show agent name, then each path on its own line
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, name))
			for _, p := range paths {
				b.WriteString(fmt.Sprintf("       %s\n", dim.Render("→ "+p)))
			}
		} else {
			// Compact: agent name + summary
			var desc string
			if len(paths) == 1 {
				desc = dim.Render(fmt.Sprintf("(%s)", paths[0]))
			} else {
				desc = dim.Render(fmt.Sprintf("(%s + %d more)", paths[0], len(paths)-1))
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, check, name, desc))
		}

		// Render the nested plan-hook sub-row directly below Claude when selected.
		if i == claudeIdx && m.selected[i] {
			subCursor := "    "
			if m.cursorOnPlanHook {
				subCursor = "  " + cursorStyle.Render("> ")
			}
			subCheck := "[ ]"
			if m.planHookEnabled {
				subCheck = checkStyle.Render("[x]")
			}
			label := "Install plan review hooks"
			if m.cursorOnPlanHook {
				label = lipgloss.NewStyle().Bold(true).Render(label)
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s\n", subCursor, subCheck, label, dim.Render("(ExitPlanMode → Monocle + pre-plan context)")))
		}
	}

	b.WriteString("\n")
	b.WriteString(dim.Render("space: toggle  a: all  enter: confirm  esc: cancel"))

	return tea.NewView(b.String())
}
