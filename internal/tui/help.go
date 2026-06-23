package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type helpModel struct {
	active         bool
	width          int
	height         int
	scrollOffset   int
	theme          Theme
	keys           *KeyMap
	reviewTracking bool
}

func newHelpModel(theme Theme, keys *KeyMap) helpModel {
	return helpModel{theme: theme, keys: keys}
}

type closeHelpMsg struct{}

func (m helpModel) Update(msg tea.Msg) (helpModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "?", "q":
			m.active = false
			return m, func() tea.Msg { return closeHelpMsg{} }
		case "j", "down":
			m.scrollOffset++
		case "k", "up":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "ctrl+d":
			m.scrollOffset += m.viewportHeight() / 2
		case "ctrl+u":
			m.scrollOffset -= m.viewportHeight() / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
	}
	return m, nil
}

// viewportHeight returns how many content lines fit inside the modal.
// Accounts for overlay topPad (2), border (2), and padding (2).
func (m helpModel) viewportHeight() int {
	const chrome = 8 // 2*topPad + 2 border + 2 padding
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

func (m helpModel) View() string {
	if !m.active {
		return ""
	}

	modalWidth := CalcModalWidth(m.width, 0)

	// Inner content width: modalWidth minus border (2) and padding (4)
	const keyCol = 20
	const indent = 2
	const borderPad = 6 // 2 border + 4 padding
	descW := modalWidth - borderPad - indent - keyCol
	if descW < 10 {
		descW = 10
	}

	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Keybindings"))
	b.WriteString("\n\n")

	km := m.keys

	navKeys := []struct{ key, desc string }{
		{Label(km.Down) + "/" + Label(km.Up), "Move up/down"},
		{Label(km.HalfDown) + "/" + Label(km.HalfUp), "Scroll diff half page (any pane)"},
		{Label(km.Top) + "/" + Label(km.Bottom), "Top/bottom"},
		{Label(km.ScrollDown) + "/" + Label(km.ScrollUp), "Scroll diff up/down (any pane)"},
		{"h/l", "Scroll diff left/right"},
		{Label(km.ScrollLeft) + "/" + Label(km.ScrollRight), "Scroll diff left/right (any pane)"},
		{Label(km.ScrollHome), "Scroll to column 0 (any pane)"},
		{Label(km.ScrollFirstChar), "Scroll to first non-space (any pane)"},
		{Label(km.ScrollEnd), "Scroll to line end (any pane)"},
		{Label(km.Wrap), "Toggle line wrapping (any pane)"},
		{Label(km.PrevFile) + "/" + Label(km.NextFile), "Previous/next file (any pane)"},
		{Label(km.PrevSection) + "/" + Label(km.NextSection), "Previous/next section (any pane)"},
		{Label(km.Select), "Focus diff pane / toggle dir"},
		{Label(km.FocusSwap), "Switch pane focus"},
		{Label(km.ToggleSidebar), "Toggle sidebar"},
		{"1/2", "Jump to pane"},
		{Label(km.BaseRef), "Change base ref"},
		{Label(km.TreeMode), "Toggle flat/tree view"},
		{Label(km.CollapseAll) + "/" + Label(km.ExpandAll), "Collapse/expand all (tree)"},
	}
	if m.reviewTracking {
		navKeys = append(navKeys, struct{ key, desc string }{Label(km.FilterReviewed), "Hide/show reviewed files"})
	}

	reviewKeys := []struct{ key, desc string }{
		{Label(km.Comment), "Add comment at cursor"},
		{Label(km.Suggest), "Suggest edit at cursor"},
		{Label(km.FileComment), "Add file comment"},
		{Label(km.Visual), "Visual select mode"},
		{"x", "Toggle comment resolved (on comment)"},
		{Label(km.DismissArtifact), "Dismiss artifact / remove added file (in sidebar)"},
		{"d", "Delete comment (on comment)"},
	}
	if m.reviewTracking {
		reviewKeys = append(reviewKeys, struct{ key, desc string }{Label(km.Reviewed), "Toggle file reviewed"})
	}
	reviewKeys = append(reviewKeys, []struct{ key, desc string }{
		{Label(km.Submit) + " / :submit", "Submit review"},
		{"Ctrl+g", "Open external editor (comment/submit modal)"},
		{"Ctrl+y", "Copy review to clipboard"},
		{Label(km.Pause) + " / :pause", "Toggle pause (ask Claude Code to wait)"},
		{Label(km.ClearReview) + " / :clear", "Clear review (comments, plans, added files, reviewed)"},
		{Label(km.ToggleFocusMode), "Toggle focus mode"},
	}...)
	if m.reviewTracking {
		reviewKeys = append(reviewKeys, []struct{ key, desc string }{
			{":mark-all-reviewed", "Mark all files as reviewed"},
			{":mark-all-unreviewed", "Mark all files as unreviewed"},
		}...)
	}
	reviewKeys = append(reviewKeys, []struct{ key, desc string }{
		{":discard", "Discard all pending comments"},
		{":history", "View submission history"},
		{Label(km.ArtifactVersions) + " / :base-artifact-version", "Base artifact version to diff against"},
		{":base-ref", "Base ref to diff against (same as " + Label(km.BaseRef) + ")"},
	}...)

	sections := []struct {
		title string
		keys  []struct{ key, desc string }
	}{
		{"Navigation", navKeys},
		{"Review", reviewKeys},
		{"Text Editing (comment/submit)", []struct{ key, desc string }{
			{"←/→ or Ctrl+B/F", "Move cursor left/right"},
			{"↑/↓ or Ctrl+P/N", "Move cursor up/down"},
			{"Home/Ctrl+A", "Line start (smart toggle)"},
			{"End/Ctrl+E", "Line end"},
			{"Alt+← or Alt+B", "Move back one word"},
			{"Alt+→ or Alt+F", "Move forward one word"},
			{"Ctrl+D / Delete", "Delete char at cursor"},
			{"Ctrl+K", "Kill to end of line"},
			{"Ctrl+U", "Kill to start of line"},
			{"Ctrl+W / Alt+Bksp", "Delete word before cursor"},
			{"Alt+D", "Delete word after cursor"},
			{"Shift+Enter", "Insert newline"},
			{"Ctrl+G", "Open in external editor"},
		}},
		{"General", []struct{ key, desc string }{
			{Label(km.OpenInEditor), "Open file in editor at cursor"},
			{Label(km.ToggleDiff), "Cycle diff style (unified/split/file) (any pane)"},
			{Label(km.ToggleFullDiff), "Toggle full-file diff (whole file vs. changed lines)"},
			{Label(km.CycleLayout), "Cycle layout (auto/side-by-side/stacked)"},
			{Label(km.Refresh), "Force reload files"},
			{"I", "Connection info"},
			{Label(km.Help), "Show this help"},
			{Label(km.Quit), "Quit"},
		}},
	}

	indentStyle := lipgloss.NewStyle().Width(indent)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true).Width(keyCol)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Width(descW)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)

	for i, section := range sections {
		b.WriteString(sectionStyle.Render(section.title))
		b.WriteString("\n")
		for _, k := range section.keys {
			row := lipgloss.JoinHorizontal(lipgloss.Top,
				indentStyle.Render(""),
				keyStyle.Render(k.key),
				descStyle.Render(k.desc),
			)
			b.WriteString(row + "\n")
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press ? or Esc to close"))

	content := b.String()

	// Apply scrolling if content is taller than viewport
	vpH := m.viewportHeight()
	lines := strings.Split(content, "\n")
	if len(lines) > vpH {
		// Clamp scroll offset
		maxOffset := len(lines) - vpH
		if m.scrollOffset > maxOffset {
			m.scrollOffset = maxOffset
		}

		end := m.scrollOffset + vpH
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[m.scrollOffset:end]

		// Add scroll indicators
		if m.scrollOffset > 0 {
			indicator := lipgloss.NewStyle().Faint(true).Render("▲ scroll up")
			visible[0] = indicator
		}
		if end < len(lines) {
			indicator := lipgloss.NewStyle().Faint(true).Render("▼ scroll down")
			visible[len(visible)-1] = indicator
		}

		content = strings.Join(visible, "\n")
	}

	return m.theme.ModalBorder.Width(modalWidth).Render(content)
}
