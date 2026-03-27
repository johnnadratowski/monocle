package adapters

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PickAgents shows an interactive multi-select picker and returns the selected adapters.
// The title is shown at the top of the picker (e.g. "Select agents to register").
func PickAgents(agents []AgentAdapter, title string) ([]AgentAdapter, error) {
	m := pickerModel{
		agents:   agents,
		selected: make(map[int]bool),
		title:    title,
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
	return picked, nil
}

type pickerModel struct {
	agents    []AgentAdapter
	selected  map[int]bool
	cursor    int
	cancelled bool
	title     string
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "space", " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
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
		case "enter":
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title))
	b.WriteString("\n\n")

	for i, a := range m.agents {
		cursor := "  "
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("> ")
		}

		check := "[ ]"
		if m.selected[i] {
			check = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("[x]")
		}

		name := a.Label()
		if i == m.cursor {
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}

		paths := strings.Join(a.ConfigPaths(false), ", ")
		desc := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("(%s)", paths))

		b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, check, name, desc))
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("space: toggle  a: all  enter: confirm  esc: cancel"))

	return tea.NewView(b.String())
}
