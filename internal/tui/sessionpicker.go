package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/monocle/internal/types"
)

type sessionPickerModel struct {
	sessions []types.SessionSummary
	cursor   int
	offset   int
	active   bool
	width    int
	height   int
	theme    Theme
}

func newSessionPickerModel(theme Theme) sessionPickerModel {
	return sessionPickerModel{theme: theme}
}

type openSessionPickerMsg struct {
	sessions []types.SessionSummary
}

type selectSessionMsg struct {
	id string // empty = new session
}

type cancelSessionPickerMsg struct{}

func (m sessionPickerModel) Update(msg tea.Msg) (sessionPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.sessions) {
				m.cursor++
				m.ensureVisible()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "enter":
			if m.cursor == 0 {
				// "New session" selected
				return m, func() tea.Msg { return selectSessionMsg{} }
			}
			idx := m.cursor - 1
			if idx < len(m.sessions) {
				id := m.sessions[idx].ID
				return m, func() tea.Msg { return selectSessionMsg{id: id} }
			}
		case "esc", "q":
			return m, func() tea.Msg { return cancelSessionPickerMsg{} }
		}
	}
	return m, nil
}

func (m sessionPickerModel) View() string {
	if !m.active {
		return ""
	}

	boxW := calcModalWidth(m.width, 80)
	contentW := boxW - 6 // 2 border + 4 padding (2 each side)

	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Render("Resume Session")
	b.WriteString(title + "\n\n")

	// "New session" option
	newLabel := "  + New session"
	if m.cursor == 0 {
		padded := newLabel + strings.Repeat(" ", max(0, contentW-lipgloss.Width(newLabel)))
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(padded))
	} else {
		b.WriteString(newLabel)
	}
	b.WriteString("\n\n")

	// Session entries with scrolling
	vh := m.viewportHeight()
	end := m.offset + vh
	if end > len(m.sessions) {
		end = len(m.sessions)
	}

	faintStyle := lipgloss.NewStyle().Faint(true)
	if m.offset > 0 {
		b.WriteString(faintStyle.Render("  ▲ more") + "\n")
	}

	idColW := 10 // "  " + 8 char hash
	detailW := contentW - idColW - 1
	if detailW < 10 {
		detailW = 10
	}

	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Width(idColW).
		PaddingLeft(2)
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Width(detailW).
		PaddingLeft(1)
	plainIDStyle := lipgloss.NewStyle().Width(idColW).PaddingLeft(2)
	plainDetailStyle := lipgloss.NewStyle().Width(detailW).PaddingLeft(1)

	for i := m.offset; i < end; i++ {
		s := m.sessions[i]
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}

		detail := fmt.Sprintf("R%d  %d files  %d comments  %s",
			s.ReviewRound, s.FileCount, s.CommentCount, relativeTime(s.UpdatedAt))

		var line string
		if m.cursor == i+1 {
			idBlock := plainIDStyle.Render(id)
			detailBlock := plainDetailStyle.Render(detail)
			line = lipgloss.JoinHorizontal(lipgloss.Top, idBlock, detailBlock)
			parts := strings.Split(line, "\n")
			for j, l := range parts {
				if w := lipgloss.Width(l); w < contentW {
					parts[j] = l + strings.Repeat(" ", contentW-w)
				}
			}
			line = lipgloss.NewStyle().Reverse(true).Render(strings.Join(parts, "\n"))
		} else {
			idBlock := idStyle.Render(id)
			detailBlock := detailStyle.Render(detail)
			line = lipgloss.JoinHorizontal(lipgloss.Top, idBlock, detailBlock)
		}
		b.WriteString(line + "\n")
	}

	if end < len(m.sessions) {
		b.WriteString(faintStyle.Render("  ▼ more") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(faintStyle.Render("  enter:select  esc:cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())
}

// viewportHeight returns how many session entries fit in the picker modal.
func (m sessionPickerModel) viewportHeight() int {
	// Chrome: title(1) + blank(1) + new session(1) + blank(1) + footer(2) + padding(2) + border(2) = 10
	maxModalH := m.height * 3 / 5
	availableLines := maxModalH - 10
	if availableLines < 1 {
		return 1
	}
	vh := availableLines * 2 / 3
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (m *sessionPickerModel) ensureVisible() {
	if m.cursor <= 0 {
		m.offset = 0
		return
	}
	entryIdx := m.cursor - 1
	if entryIdx < m.offset {
		m.offset = entryIdx
	}
	vh := m.viewportHeight()
	if vh > 0 && entryIdx >= m.offset+vh {
		m.offset = entryIdx - vh + 1
	}
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
