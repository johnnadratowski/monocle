package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

type versionPickerModel struct {
	versions  []types.ContentVersion // newest first (descending order)
	contentID string
	cursor    int
	offset    int
	active    bool
	width     int
	height    int
	theme     Theme
}

func newVersionPickerModel(theme Theme) versionPickerModel {
	return versionPickerModel{theme: theme}
}

type openVersionPickerMsg struct {
	contentID string
	versions  []types.ContentVersion // newest first
}

type selectVersionMsg struct {
	contentID   string
	fromVersion int // base version selected by user
	toVersion   int // latest version
}

type cancelVersionPickerMsg struct{}

func (m versionPickerModel) maxCursor() int {
	return len(m.versions) - 1
}

// latestVersion returns the newest version (index 0).
func (m versionPickerModel) latestVersion() types.ContentVersion {
	return m.versions[0]
}

func (m versionPickerModel) Update(msg tea.Msg) (versionPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < m.maxCursor() {
				m.cursor++
				m.ensureVisible()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "enter":
			// Index 0 is the latest version — selecting it would be a no-op diff
			if m.cursor > 0 && m.cursor < len(m.versions) {
				latest := m.latestVersion()
				selected := m.versions[m.cursor]
				return m, func() tea.Msg {
					return selectVersionMsg{
						contentID:   m.contentID,
						fromVersion: selected.Version,
						toVersion:   latest.Version,
					}
				}
			}
		case "esc", "q":
			return m, func() tea.Msg { return cancelVersionPickerMsg{} }
		}
	}
	return m, nil
}

func (m versionPickerModel) View() string {
	if !m.active {
		return ""
	}

	boxW := calcModalWidth(m.width, 70)
	contentW := boxW - 6 // 2 border + 4 padding (2 each side)

	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Render("Select Base Version")
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Diff from selected version to latest") + "\n\n")

	vh := m.viewportHeight()
	end := m.offset + vh
	if end > len(m.versions) {
		end = len(m.versions)
	}

	faintStyle := lipgloss.NewStyle().Faint(true)
	if m.offset > 0 {
		b.WriteString(faintStyle.Render("  \u25b2 more") + "\n")
	}

	// Column widths
	versionColW := 6 // "  v99 " with padding
	timeColW := 12   // "  12h ago  "
	labelW := contentW - versionColW - timeColW
	if labelW < 10 {
		labelW = 10
	}

	versionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Width(versionColW).PaddingLeft(2)
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(timeColW)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Width(labelW)
	plainVersionStyle := lipgloss.NewStyle().Width(versionColW).PaddingLeft(2)
	plainTimeStyle := lipgloss.NewStyle().Width(timeColW)
	plainLabelStyle := lipgloss.NewStyle().Width(labelW)

	for i := m.offset; i < end; i++ {
		v := m.versions[i]
		vLabel := fmt.Sprintf("v%d", v.Version)
		tLabel := relativeTime(v.CreatedAt)
		titleLabel := v.Title
		if i == 0 {
			titleLabel += " (current)"
		}

		var line string
		if m.cursor == i {
			vBlock := plainVersionStyle.Render(vLabel)
			tBlock := plainTimeStyle.Render(tLabel)
			lBlock := plainLabelStyle.Render(titleLabel)
			line = lipgloss.JoinHorizontal(lipgloss.Top, vBlock, tBlock, lBlock)
			parts := strings.Split(line, "\n")
			for j, l := range parts {
				if w := lipgloss.Width(l); w < contentW {
					parts[j] = l + strings.Repeat(" ", contentW-w)
				}
			}
			line = lipgloss.NewStyle().Reverse(true).Render(strings.Join(parts, "\n"))
		} else {
			vBlock := versionStyle.Render(vLabel)
			tBlock := timeStyle.Render(tLabel)
			lBlock := labelStyle.Render(titleLabel)
			line = lipgloss.JoinHorizontal(lipgloss.Top, vBlock, tBlock, lBlock)
		}
		b.WriteString(line + "\n")
	}

	if end < len(m.versions) {
		b.WriteString(faintStyle.Render("  \u25bc more") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  enter:select  esc:cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())
}

// handleClick processes a mouse click at content-relative coordinates.
func (m *versionPickerModel) handleClick(contentY int) (tea.Cmd, bool) {
	// Content layout:
	// Line 0: "Select Base Version"
	// Line 1: subtitle
	// Line 2: blank
	// Line 3+: "▲ more" (if scrolled), then entries

	entryStartLine := 3
	if m.offset > 0 {
		entryStartLine++
	}

	clickedIdx := contentY - entryStartLine + m.offset
	// Index 0 is latest — can't select as base
	if clickedIdx > 0 && clickedIdx < len(m.versions) {
		m.cursor = clickedIdx
		latest := m.latestVersion()
		selected := m.versions[clickedIdx]
		return func() tea.Msg {
			return selectVersionMsg{
				contentID:   m.contentID,
				fromVersion: selected.Version,
				toVersion:   latest.Version,
			}
		}, true
	}

	return nil, false
}

func (m versionPickerModel) viewportHeight() int {
	maxModalH := m.height * 3 / 5
	availableLines := maxModalH - 12 // chrome: title, subtitle, blank, footer, padding, border
	if availableLines < 1 {
		return 1
	}
	vh := availableLines * 2 / 3
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (m *versionPickerModel) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	vh := m.viewportHeight()
	if vh > 0 && m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
}
