package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type infoBannerModel struct {
	active  bool
	title   string
	message string
	width   int
	height  int
	theme   Theme
}

func newInfoBannerModel(theme Theme) infoBannerModel {
	return infoBannerModel{theme: theme}
}

type closeInfoBannerMsg struct {
	quit bool
}

func (m *infoBannerModel) open(title, message string) {
	m.active = true
	m.title = title
	m.message = message
}

func (m infoBannerModel) Update(msg tea.Msg) (infoBannerModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.active = false
			return m, func() tea.Msg { return closeInfoBannerMsg{} }
		case "esc":
			m.active = false
			return m, func() tea.Msg { return closeInfoBannerMsg{quit: true} }
		}
	}
	return m, nil
}

func (m infoBannerModel) View() string {
	if !m.active {
		return ""
	}

	modalWidth := CalcModalWidth(m.width, 0)

	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title))
	b.WriteString("\n\n")
	b.WriteString(m.message)
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press Enter to continue or Esc to quit"))

	return m.theme.ModalBorder.Width(modalWidth).Render(b.String())
}
