package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type helpModel struct {
	active         bool
	width          int
	height         int
	scrollOffset   int
	theme          Theme
	keys           *KeyMap
	reviewTracking bool

	// Search within the help text.
	searchMode    bool // currently typing a query
	searchQuery   string
	searchMatches []int // content-line indices that match the query
	searchIndex   int   // position within searchMatches

	// Shared search history (synced from the app before each Update). history[0]
	// is the most recent query. justCommitted signals the app to record the
	// current query into the shared history after this Update.
	history       []string
	historyIdx    int // -1 when not recalling
	justCommitted bool
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
		key := msg.String()
		if m.searchMode {
			return m.handleSearchKey(key), nil
		}
		// The Help key toggles: pressing it again closes the overlay.
		if m.keys != nil && Matches(key, m.keys.Help) {
			m.active = false
			return m, func() tea.Msg { return closeHelpMsg{} }
		}
		switch key {
		case "esc":
			// First esc clears an active search; a second closes help.
			if m.searchQuery != "" {
				m.clearSearch()
				return m, nil
			}
			m.active = false
			return m, func() tea.Msg { return closeHelpMsg{} }
		case "?", "q":
			m.active = false
			return m, func() tea.Msg { return closeHelpMsg{} }
		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIndex = 0
			m.historyIdx = -1
		case "n":
			if len(m.searchMatches) == 0 {
				m.seedFromHistory()
			}
			m.stepMatch(1)
		case "N":
			if len(m.searchMatches) == 0 {
				m.seedFromHistory()
			}
			m.stepMatch(-1)
		case "g":
			m.scrollOffset = 0
		case "G":
			m.scrollOffset = m.maxScroll()
		case "j", "down":
			m.scrollOffset++
			m.clampScroll()
		case "k", "up":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "ctrl+d":
			m.scrollOffset += m.viewportHeight() / 2
			m.clampScroll()
		case "ctrl+u":
			m.scrollOffset -= m.viewportHeight() / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
	}
	return m, nil
}

// handleSearchKey processes keys while typing a search query.
func (m helpModel) handleSearchKey(key string) helpModel {
	switch key {
	case "esc":
		m.searchMode = false
		m.clearSearch()
	case "enter":
		// Confirm: leave the matches highlighted, exit typing mode, and ask the
		// app to record the query in the shared history.
		m.searchMode = false
		if m.searchQuery != "" {
			m.justCommitted = true
		}
	case "up":
		if q := m.recallHistory(1); q != "" {
			m.searchQuery = q
			m.recomputeMatches()
		}
	case "down":
		m.searchQuery = m.recallHistory(-1)
		m.recomputeMatches()
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.recomputeMatches()
		}
	case "space":
		m.searchQuery += " "
		m.recomputeMatches()
	default:
		if len(key) == 1 { // a single printable rune
			m.searchQuery += key
			m.recomputeMatches()
		}
	}
	return m
}

// seedFromHistory loads the most recent shared query so n/N can search the help
// text even though the user never opened the help search prompt.
func (m *helpModel) seedFromHistory() {
	if m.searchQuery != "" || len(m.history) == 0 {
		return
	}
	m.searchQuery = m.history[0]
	m.recomputeMatches()
}

// recallHistory steps through the shared history while typing. dir +1 recalls
// older entries, -1 newer; returns "" past the newest entry.
func (m *helpModel) recallHistory(dir int) string {
	if len(m.history) == 0 {
		return ""
	}
	idx := m.historyIdx + dir
	if idx < 0 {
		idx = -1
	}
	if idx >= len(m.history) {
		idx = len(m.history) - 1
	}
	m.historyIdx = idx
	if idx < 0 {
		return ""
	}
	return m.history[idx]
}

// clearSearch resets all search state.
func (m *helpModel) clearSearch() {
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchIndex = 0
}

// clampScroll keeps scrollOffset within [0, maxScroll].
func (m *helpModel) clampScroll() {
	if max := m.maxScroll(); m.scrollOffset > max {
		m.scrollOffset = max
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// maxScroll is the largest valid scroll offset for the current content.
func (m helpModel) maxScroll() int {
	n := len(m.contentLines()) - m.viewportHeight()
	if n < 0 {
		n = 0
	}
	return n
}

// contentLines returns the rendered help content split into lines.
func (m helpModel) contentLines() []string {
	return strings.Split(m.buildContent(), "\n")
}

// recomputeMatches rebuilds the match list for the current query and jumps to
// the first match.
func (m *helpModel) recomputeMatches() {
	m.searchMatches = nil
	m.searchIndex = 0
	if m.searchQuery == "" {
		return
	}
	q := strings.ToLower(m.searchQuery)
	for i, line := range m.contentLines() {
		if strings.Contains(strings.ToLower(ansi.Strip(line)), q) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	m.scrollToMatch()
}

// stepMatch advances to the next/previous match (wrapping).
func (m *helpModel) stepMatch(dir int) {
	if len(m.searchMatches) == 0 {
		return
	}
	n := len(m.searchMatches)
	m.searchIndex = (m.searchIndex + dir + n) % n
	m.scrollToMatch()
}

// scrollToMatch scrolls so the current match is visible (a couple of lines down
// from the top of the viewport for context).
func (m *helpModel) scrollToMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	target := m.searchMatches[m.searchIndex]
	m.scrollOffset = target - 2
	m.clampScroll()
	if target < m.scrollOffset {
		m.scrollOffset = target
	}
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

// buildContent renders the full (unscrolled) help text.
func (m helpModel) buildContent() string {
	if !m.active {
		return ""
	}

	modalWidth := CalcModalWidth(m.width, 0)

	const indent = 2
	const borderPad = 6 // 2 border + 4 padding

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
		{Label(km.ScrollRight), "Scroll diff right (any pane)"},
		{"/  " + Label(km.SearchBackward), "Search diff fwd/back (diff focused)"},
		{Label(km.SearchNext) + "/" + Label(km.SearchPrev), "Next/previous search match"},
		{Label(km.ScrollHome), "Scroll to column 0 (any pane)"},
		{Label(km.ScrollFirstChar), "Scroll to first non-space (any pane)"},
		{Label(km.ScrollEnd), "Scroll to line end (any pane)"},
		{Label(km.Wrap), "Toggle line wrapping (any pane)"},
		{Label(km.PrevFile) + "/" + Label(km.NextFile), "Previous/next file (any pane)"},
		{Label(km.PrevSection) + "/" + Label(km.NextSection), "Previous/next section (any pane)"},
		{Label(km.Select), "Focus diff pane / toggle dir"},
		{Label(km.FocusSwap) + "/shift+tab", "Switch pane focus (sidebar/diff/doc)"},
		{Label(km.OpenDocRef), "Open/cycle annotation doc links; closes after the last"},
		{Label(km.ToggleSidebar), "Toggle sidebar"},
		{"1/2", "Jump to pane"},
		{Label(km.BaseRef), "Change base ref"},
		{Label(km.TreeMode), "Cycle flat/tree/grouped view"},
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
		{Label(km.YankLine), "Yank line / selection to clipboard"},
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
			{Label(km.ToggleOverlays), "Hide/show inline comments + annotations"},
			{Label(km.HideComments), "Dim/show source-code comment lines"},
			{Label(km.CycleLayout), "Cycle layout (auto/side-by-side/stacked)"},
			{Label(km.Refresh), "Force reload files"},
			{"I", "Connection info"},
			{Label(km.Help), "Show this help"},
			{Label(km.Quit), "Quit"},
		}},
	}

	// Size the key column to the widest key label (plus a gap) so long command
	// keys like "B / :base-artifact-version" never collide with the description.
	keyCol := 0
	for _, section := range sections {
		for _, k := range section.keys {
			if w := lipgloss.Width(k.key); w > keyCol {
				keyCol = w
			}
		}
	}
	keyCol += 2 // gap between key and description
	// Keep at least 20 columns for the description; cap the key column so a very
	// long key can't crowd it out on a narrow screen.
	if maxKey := modalWidth - borderPad - indent - 20; maxKey > 0 && keyCol > maxKey {
		keyCol = maxKey
	}
	descW := modalWidth - borderPad - indent - keyCol
	if descW < 10 {
		descW = 10
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
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press / to search, ? or Esc to close"))

	return b.String()
}

func (m helpModel) View() string {
	if !m.active {
		return ""
	}

	modalWidth := CalcModalWidth(m.width, 0)
	innerW := modalWidth - 6 // 2 border + 4 padding
	if innerW < 1 {
		innerW = 1
	}

	lines := strings.Split(m.buildContent(), "\n")

	// Reserve a row for the search bar when searching.
	vpH := m.viewportHeight()
	searching := m.searchMode || m.searchQuery != ""
	if searching {
		vpH--
		if vpH < 1 {
			vpH = 1
		}
	}

	// Highlight matched lines in place.
	if len(m.searchMatches) > 0 {
		current := -1
		if m.searchIndex < len(m.searchMatches) {
			current = m.searchMatches[m.searchIndex]
		}
		matchStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Width(innerW)
		currentStyle := lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Width(innerW)
		for _, idx := range m.searchMatches {
			if idx < 0 || idx >= len(lines) {
				continue
			}
			plain := ansi.Strip(lines[idx])
			if idx == current {
				lines[idx] = currentStyle.Render(plain)
			} else {
				lines[idx] = matchStyle.Render(plain)
			}
		}
	}

	// Scroll.
	offset := m.scrollOffset
	if max := len(lines) - vpH; offset > max {
		offset = max
	}
	if offset < 0 {
		offset = 0
	}
	var content string
	if len(lines) > vpH {
		end := offset + vpH
		if end > len(lines) {
			end = len(lines)
		}
		visible := make([]string, end-offset)
		copy(visible, lines[offset:end])
		if offset > 0 {
			visible[0] = lipgloss.NewStyle().Faint(true).Render("▲ scroll up")
		}
		if end < len(lines) {
			visible[len(visible)-1] = lipgloss.NewStyle().Faint(true).Render("▼ scroll down")
		}
		content = strings.Join(visible, "\n")
	} else {
		content = strings.Join(lines, "\n")
	}

	if searching {
		content += "\n" + m.renderSearchBar(innerW)
	}

	return m.theme.ModalBorder.Width(modalWidth).Render(content)
}

// renderSearchBar renders the search prompt / match count shown at the bottom of
// the help modal while searching.
func (m helpModel) renderSearchBar(width int) string {
	var label string
	if m.searchMode {
		label = "/" + m.searchQuery + "▏"
	} else {
		label = "/" + m.searchQuery
	}
	count := ""
	if m.searchQuery != "" {
		if len(m.searchMatches) == 0 {
			count = "  no matches"
		} else {
			count = fmt.Sprintf("  %d/%d", m.searchIndex+1, len(m.searchMatches))
		}
	}
	hint := "  (enter: keep · ↑/↓: history · n/N: next/prev · esc: clear)"
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Width(width)
	return style.Render(label + count + hint)
}
