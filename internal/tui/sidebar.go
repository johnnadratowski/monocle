package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

type sidebarModel struct {
	files           []types.ChangedFile
	contentItems    []types.ContentItem
	additionalFiles []types.AdditionalFile
	cursor          int
	offset          int // scroll offset for viewport
	width           int
	height          int
	focused         bool
	recentPaths     map[string]bool
	keys            *KeyMap

	// Tree mode state
	treeMode     bool
	treeRoots    []*fileTreeNode
	collapsed    map[string]bool
	visibleItems []visibleItem

	// Grouped mode state. When groupMode is on, files are displayed in
	// category groups (groupedFiles) with a header drawn before the first file of
	// each group (groupHeaderAt: display index -> header label). The item-index
	// model is unchanged from flat mode — headers are decorations, not items.
	groupMode     bool
	groupedFiles  []types.ChangedFile
	groupHeaderAt map[int]string

	// Filter state: "" = show all, "unreviewed" = hide reviewed, "reviewed" = hide unreviewed
	reviewFilter string

	// reviewTracking: when false, hide all review indicators/counts/filters
	reviewTracking bool
}

func newSidebarModel(keys *KeyMap) sidebarModel {
	return sidebarModel{
		recentPaths: make(map[string]bool),
		collapsed:   make(map[string]bool),
		keys:        keys,
	}
}

type sidebarSelectMsg struct {
	path             string
	isContent        bool
	contentID        string
	isAdditionalFile bool
}

type recentFadeMsg struct {
	path string
}

func (m sidebarModel) Init() tea.Cmd {
	return nil
}

func (m sidebarModel) Update(msg tea.Msg) (sidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		key := msg.String()
		switch {
		case Matches(key, m.keys.Down):
			if m.cursor < m.totalItems()-1 {
				m.cursor++
			}
			m.ensureVisible()
			return m, m.selectCurrent()
		case Matches(key, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			m.ensureVisible()
			return m, m.selectCurrent()
		case Matches(key, m.keys.Top):
			m.cursor = 0
			m.ensureVisible()
			return m, m.selectCurrent()
		case Matches(key, m.keys.Bottom):
			if total := m.totalItems(); total > 0 {
				m.cursor = total - 1
			}
			m.ensureVisible()
			return m, m.selectCurrent()
		case Matches(key, m.keys.Select):
			if m.treeMode {
				idx := m.cursor - len(m.contentItems)
				if idx >= 0 && idx < len(m.visibleItems) && m.visibleItems[idx].isDir {
					path := m.visibleItems[idx].node.Path
					if m.collapsed[path] {
						delete(m.collapsed, path)
					} else {
						m.collapsed[path] = true
					}
					m.visibleItems = flattenTree(m.treeRoots, m.collapsed)
					// Clamp cursor
					if total := m.totalItems(); total > 0 && m.cursor >= total {
						m.cursor = total - 1
					}
					m.ensureVisible()
					return m, nil
				}
			}
			return m, m.selectCurrent()
		case Matches(key, m.keys.TreeMode):
			currentPath := ""
			if f := m.selectedFile(); f != nil {
				currentPath = f.Path
			}
			// Cycle flat -> tree -> grouped -> flat.
			switch {
			case !m.treeMode && !m.groupMode:
				m.treeMode = true
				m.rebuildTree()
			case m.treeMode:
				m.treeMode = false
				m.groupMode = true
				m.rebuildGroups()
			default:
				m.groupMode = false
			}
			if currentPath != "" {
				m.selectPath(currentPath)
			}
			if total := m.totalItems(); total > 0 && m.cursor >= total {
				m.cursor = total - 1
			}
			m.ensureVisible()
			return m, m.selectCurrent()
		case Matches(key, m.keys.CollapseAll):
			if m.treeMode {
				m.collapseAll()
				return m, m.selectCurrent()
			}
		case Matches(key, m.keys.ExpandAll):
			if m.treeMode {
				m.collapsed = make(map[string]bool)
				m.visibleItems = flattenTree(m.treeRoots, m.collapsed)
				if total := m.totalItems(); total > 0 && m.cursor >= total {
					m.cursor = total - 1
				}
				m.ensureVisible()
				return m, m.selectCurrent()
			}
		}
	}
	return m, nil
}

func (m sidebarModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Bold(true).Width(m.width)
	groupHeaderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true).Width(m.width)

	// Render only items within the viewport [offset, offset+viewportHeight)
	contentItemCt := len(m.contentItems)
	totalItems := m.totalItems()
	// viewportHeight() subtracts headers from m.height, but when content items
	// exist the loop already counts headers in linesUsed — use m.height to
	// avoid double-subtracting.
	availableLines := m.height
	if contentItemCt == 0 {
		availableLines = m.viewportHeight()
	}

	linesUsed := 0

	// Artifacts section header (if any content items exist)
	if contentItemCt > 0 {
		var header string
		if m.reviewTracking {
			contentReviewed := 0
			for _, item := range m.contentItems {
				if item.Reviewed {
					contentReviewed++
				}
			}
			header = fmt.Sprintf(" Artifacts  %d / %d", contentReviewed, contentItemCt)
		} else {
			header = fmt.Sprintf(" Artifacts  %d", contentItemCt)
		}
		b.WriteString(sectionStyle.Render(header))
		b.WriteString("\n")
		linesUsed++
	}

	fileItemCt := m.fileItemCount()
	additionalStart := contentItemCt + fileItemCt
	additionalCt := len(m.additionalFiles)

	for idx := m.offset; idx < totalItems && linesUsed < availableLines; idx++ {
		// Files section header (when crossing from content items to files)
		if idx == contentItemCt && contentItemCt > 0 {
			if linesUsed > 0 {
				if linesUsed+1 > availableLines {
					break
				}
				b.WriteString("\n")
				linesUsed++
			}

			fileCount := len(m.files)
			modeIndicator := ""
			if m.treeMode {
				modeIndicator = " "
			} else if m.groupMode {
				modeIndicator = " 󰓫"
			}
			var header string
			if m.reviewTracking {
				reviewedCount := 0
				for _, f := range m.files {
					if f.Reviewed {
						reviewedCount++
					}
				}
				filterIndicator := m.reviewFilterLabel()
				header = fmt.Sprintf(" Files%s%s  %d / %d", modeIndicator, filterIndicator, reviewedCount, fileCount)
			} else {
				header = fmt.Sprintf(" Files%s  %d", modeIndicator, fileCount)
			}
			b.WriteString(sectionStyle.Render(header))
			b.WriteString("\n")
			linesUsed++
			if linesUsed >= availableLines {
				break
			}
		}

		// Additional Files section header
		if idx == additionalStart && additionalCt > 0 {
			if linesUsed > 0 {
				if linesUsed+1 > availableLines {
					break
				}
				b.WriteString("\n")
				linesUsed++
			}

			var header string
			if m.reviewTracking {
				reviewedCount := 0
				for _, af := range m.additionalFiles {
					if af.Reviewed {
						reviewedCount++
					}
				}
				filterIndicator := m.reviewFilterLabel()
				header = fmt.Sprintf(" Additional Files%s  %d / %d", filterIndicator, reviewedCount, additionalCt)
			} else {
				header = fmt.Sprintf(" Additional Files  %d", additionalCt)
			}
			b.WriteString(sectionStyle.Render(header))
			b.WriteString("\n")
			linesUsed++
			if linesUsed >= availableLines {
				break
			}
		}

		// Grouped mode: draw the category header before the first file of its group.
		if m.groupMode && idx >= contentItemCt && idx < additionalStart {
			if hdr, ok := m.groupHeaderAt[idx-contentItemCt]; ok {
				if linesUsed+1 > availableLines {
					break
				}
				b.WriteString(groupHeaderStyle.Render(hdr))
				b.WriteString("\n")
				linesUsed++
			}
		}

		var line string
		if idx < contentItemCt {
			line = m.renderContentItem(m.contentItems[idx], idx == m.cursor)
		} else if idx < additionalStart {
			fileIdx := idx - contentItemCt
			if m.treeMode {
				item := m.visibleItems[fileIdx]
				if item.isDir {
					line = m.renderDirItem(item, idx == m.cursor)
				} else {
					line = m.renderTreeFileItem(item, idx == m.cursor)
				}
			} else {
				line = m.renderFileItem(m.displayFiles()[fileIdx], idx == m.cursor)
			}
		} else {
			additionalIdx := idx - additionalStart
			line = m.renderAdditionalFileItem(m.additionalFiles[additionalIdx], idx == m.cursor)
		}

		b.WriteString(line)
		b.WriteString("\n")
		linesUsed++
	}

	// If no content items, show the Files header at the top
	if contentItemCt == 0 {
		var header strings.Builder
		fileCount := len(m.files)
		modeIndicator := ""
		if m.treeMode {
			modeIndicator = " "
		}
		var headerStr string
		if m.reviewTracking {
			reviewedCount := 0
			for _, f := range m.files {
				if f.Reviewed {
					reviewedCount++
				}
			}
			filterIndicator := m.reviewFilterLabel()
			headerStr = fmt.Sprintf(" Files%s%s  %d / %d", modeIndicator, filterIndicator, reviewedCount, fileCount)
		} else {
			headerStr = fmt.Sprintf(" Files%s  %d", modeIndicator, fileCount)
		}
		header.WriteString(sectionStyle.Render(headerStr))
		header.WriteString("\n")
		header.WriteString(b.String())
		return strings.TrimRight(header.String(), "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m sidebarModel) renderFileItem(f types.ChangedFile, selected bool) string {
	// Status indicator (lazygit-style colors)
	var statusChar, statusColor string
	switch f.Status {
	case types.FileAdded:
		statusChar = "A"
		statusColor = "#2ea043"
	case types.FileModified:
		statusChar = "M"
		statusColor = "#d29922"
	case types.FileDeleted:
		statusChar = "D"
		statusColor = "#f85149"
	case types.FileRenamed:
		statusChar = "R"
		statusColor = "#a371f7"
	case types.FileNone:
		statusChar = " "
		statusColor = "7"
	default:
		statusChar = "?"
		statusColor = "7"
	}
	styledStatus := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true).Render(statusChar)

	// Review status
	reviewChar := ""
	if m.reviewTracking {
		reviewChar = "○"
		if f.Reviewed {
			reviewChar = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("✓")
		}
	}

	// Recent indicator
	recentChar := " "
	if m.recentPaths[f.Path] {
		recentChar = "~"
	}

	// Layout: " {status} {recent}{icon} {name...}  {review} "
	// Icon glyphs render as width 2 in terminals but lipgloss measures them
	// as width 1. We account for this by subtracting iconSlack from nameW
	// and always padding name to a fixed width so alignment is consistent.
	icon := fileIcon(f.Path)
	glyph := iconLookup(f.Path).glyph
	const iconSlack = 2

	if selected && m.focused {
		right := " "
		if m.reviewTracking {
			plainReview := "○"
			if f.Reviewed {
				plainReview = "✓"
			}
			right = " " + plainReview + " "
		}
		prefix := fmt.Sprintf(" %s %s%s ", statusChar, recentChar, glyph)
		nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
		if nameW < 1 {
			nameW = 1
		}
		name := fmt.Sprintf("%-*s", nameW, truncatePath(f.Path, nameW))
		padded := prefix + name + right
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	leftPad := " "
	if selected {
		leftPad = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("▎")
	}

	right := " "
	if m.reviewTracking {
		right = " " + reviewChar + " "
	}
	prefix := fmt.Sprintf("%s%s %s%s ", leftPad, styledStatus, recentChar, icon)
	nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
	if nameW < 1 {
		nameW = 1
	}
	name := fmt.Sprintf("%-*s", nameW, truncatePath(f.Path, nameW))
	return prefix + name + right
}

// renderDirItem renders a directory node in tree mode.
func (m sidebarModel) renderDirItem(item visibleItem, selected bool) string {
	indent := strings.Repeat("  ", item.depth)
	arrow := "▼"
	if m.collapsed[item.node.Path] {
		arrow = "▶"
	}

	// Folder icon
	const folderGlyph = "\uf07b" // nf-fa-folder
	const folderColor = "#e8a838"
	const iconSlack = 2

	if selected && m.focused {
		prefix := fmt.Sprintf(" %s%s %s ", indent, arrow, folderGlyph)
		nameW := m.width - lipgloss.Width(prefix) - iconSlack
		if nameW < 1 {
			nameW = 1
		}
		name := fmt.Sprintf("%-*s", nameW, truncatePath(item.node.Name, nameW))
		padded := prefix + name
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	leftPad := " "
	if selected {
		leftPad = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("▎")
	}

	styledArrow := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(arrow)
	styledFolder := lipgloss.NewStyle().Foreground(lipgloss.Color(folderColor)).Render(folderGlyph)
	prefix := fmt.Sprintf("%s%s%s %s ", leftPad, indent, styledArrow, styledFolder)
	nameW := m.width - lipgloss.Width(prefix) - iconSlack
	if nameW < 1 {
		nameW = 1
	}
	dirStyle := lipgloss.NewStyle().Bold(true)
	name := fmt.Sprintf("%-*s", nameW, truncatePath(item.node.Name, nameW))
	return prefix + dirStyle.Render(name)
}

// renderTreeFileItem renders a file node in tree mode with indentation.
func (m sidebarModel) renderTreeFileItem(item visibleItem, selected bool) string {
	f := item.node.File
	indent := strings.Repeat("  ", item.depth)

	var statusChar, statusColor string
	switch f.Status {
	case types.FileAdded:
		statusChar = "A"
		statusColor = "#2ea043"
	case types.FileModified:
		statusChar = "M"
		statusColor = "#d29922"
	case types.FileDeleted:
		statusChar = "D"
		statusColor = "#f85149"
	case types.FileRenamed:
		statusChar = "R"
		statusColor = "#a371f7"
	case types.FileNone:
		statusChar = " "
		statusColor = "7"
	default:
		statusChar = "?"
		statusColor = "7"
	}

	reviewChar := ""
	if m.reviewTracking {
		reviewChar = "○"
		if f.Reviewed {
			reviewChar = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("✓")
		}
	}

	recentChar := " "
	if m.recentPaths[f.Path] {
		recentChar = "~"
	}

	icon := fileIcon(f.Path)
	glyph := iconLookup(f.Path).glyph
	const iconSlack = 2

	if selected && m.focused {
		right := " "
		if m.reviewTracking {
			plainReview := "○"
			if f.Reviewed {
				plainReview = "✓"
			}
			right = " " + plainReview + " "
		}
		prefix := fmt.Sprintf(" %s%s %s%s ", indent, statusChar, recentChar, glyph)
		nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
		if nameW < 1 {
			nameW = 1
		}
		name := fmt.Sprintf("%-*s", nameW, truncatePath(item.node.Name, nameW))
		padded := prefix + name + right
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	leftPad := " "
	if selected {
		leftPad = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("▎")
	}

	styledStatus := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true).Render(statusChar)
	right := " "
	if m.reviewTracking {
		right = " " + reviewChar + " "
	}
	prefix := fmt.Sprintf("%s%s%s %s%s ", leftPad, indent, styledStatus, recentChar, icon)
	nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
	if nameW < 1 {
		nameW = 1
	}
	name := fmt.Sprintf("%-*s", nameW, truncatePath(item.node.Name, nameW))
	return prefix + name + right
}

func (m sidebarModel) renderContentItem(item types.ContentItem, selected bool) string {
	reviewChar := ""
	if m.reviewTracking {
		reviewChar = "○"
		if item.Reviewed {
			reviewChar = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("✓")
		}
	}

	// Build icon path from content type (e.g. "md" → "content.md")
	iconPath := item.Title
	if item.ContentType != "" {
		ext := item.ContentType
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		iconPath = "content" + ext
	}
	icon := fileIcon(iconPath)
	glyph := iconLookup(iconPath).glyph
	const iconSlack = 2

	if selected && m.focused {
		right := " "
		if m.reviewTracking {
			plainReview := "○"
			if item.Reviewed {
				plainReview = "✓"
			}
			right = " " + plainReview + " "
		}
		prefix := fmt.Sprintf("  %s ", glyph)
		nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
		if nameW < 1 {
			nameW = 1
		}
		name := truncatePath(item.Title, nameW)
		line := fmt.Sprintf("%s%-*s%s", prefix, nameW, name, right)
		return lipgloss.NewStyle().Reverse(true).Render(line)
	}

	leftPad := " "
	if selected {
		leftPad = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("▎")
	}
	right := " "
	if m.reviewTracking {
		right = " " + reviewChar + " "
	}
	prefix := fmt.Sprintf("%s %s ", leftPad, icon)
	nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
	if nameW < 1 {
		nameW = 1
	}
	name := truncatePath(item.Title, nameW)
	return fmt.Sprintf("%s%-*s%s", prefix, nameW, name, right)
}

func (m sidebarModel) renderAdditionalFileItem(af types.AdditionalFile, selected bool) string {
	reviewChar := ""
	if m.reviewTracking {
		reviewChar = "○"
		if af.Reviewed {
			reviewChar = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("✓")
		}
	}

	icon := fileIcon(af.Path)
	glyph := iconLookup(af.Path).glyph
	const iconSlack = 2

	if selected && m.focused {
		right := " "
		if m.reviewTracking {
			plainReview := "○"
			if af.Reviewed {
				plainReview = "✓"
			}
			right = " " + plainReview + " "
		}
		prefix := fmt.Sprintf("  %s ", glyph)
		nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
		if nameW < 1 {
			nameW = 1
		}
		name := fmt.Sprintf("%-*s", nameW, truncatePath(af.Name, nameW))
		padded := prefix + name + right
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	leftPad := " "
	if selected {
		leftPad = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("▎")
	}

	right := " "
	if m.reviewTracking {
		right = " " + reviewChar + " "
	}
	prefix := fmt.Sprintf("%s %s ", leftPad, icon)
	nameW := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - iconSlack
	if nameW < 1 {
		nameW = 1
	}
	name := fmt.Sprintf("%-*s", nameW, truncatePath(af.Name, nameW))
	return prefix + name + right
}

func (m sidebarModel) totalItems() int {
	return m.fileItemCount() + len(m.contentItems) + len(m.additionalFiles)
}

// fileItemCount returns the number of file-related items (files in flat mode,
// visible items in tree mode). Grouped mode has the same count as flat mode —
// group headers are decorations, not items.
func (m sidebarModel) fileItemCount() int {
	if m.treeMode {
		return len(m.visibleItems)
	}
	return len(m.files)
}

// displayFiles returns the files in their current display order: grouped order
// in grouped mode, otherwise the natural order. Used everywhere a flat file index
// is resolved so grouped mode stays consistent with what is rendered.
func (m sidebarModel) displayFiles() []types.ChangedFile {
	if m.groupMode {
		return m.groupedFiles
	}
	return m.files
}

// rebuildGroups recomputes the grouped display order and header positions from
// the current files. Safe to call when groupMode is false (no-op).
func (m *sidebarModel) rebuildGroups() {
	if !m.groupMode {
		return
	}
	m.groupedFiles, m.groupHeaderAt = groupFilesByCategory(m.files)
}

func (m sidebarModel) selectCurrent() tea.Cmd {
	contentCount := len(m.contentItems)

	// Content items come first
	if m.cursor < contentCount {
		item := m.contentItems[m.cursor]
		return func() tea.Msg {
			return sidebarSelectMsg{isContent: true, contentID: item.ID}
		}
	}

	// Then file items
	fileIdx := m.cursor - contentCount
	if fileIdx < m.fileItemCount() {
		if m.treeMode {
			item := m.visibleItems[fileIdx]
			if item.isDir {
				return nil // Don't send selection for directories
			}
			path := item.node.File.Path
			return func() tea.Msg {
				return sidebarSelectMsg{path: path}
			}
		}
		path := m.displayFiles()[fileIdx].Path
		return func() tea.Msg {
			return sidebarSelectMsg{path: path}
		}
	}

	// Then additional files
	additionalIdx := m.cursor - contentCount - m.fileItemCount()
	if additionalIdx >= 0 && additionalIdx < len(m.additionalFiles) {
		af := m.additionalFiles[additionalIdx]
		return func() tea.Msg {
			return sidebarSelectMsg{path: af.Path, isAdditionalFile: true}
		}
	}

	return nil
}

// selectedContentItem returns the ContentItem at the current cursor position,
// or nil if the cursor is on a file or directory.
func (m sidebarModel) selectedContentItem() *types.ContentItem {
	if m.cursor < 0 || m.cursor >= len(m.contentItems) {
		return nil
	}
	return &m.contentItems[m.cursor]
}

// selectedFile returns the ChangedFile at the current cursor position,
// or nil if the cursor is on a directory or content item.
func (m sidebarModel) selectedFile() *types.ChangedFile {
	contentCount := len(m.contentItems)
	if m.cursor < contentCount {
		return nil // content item, not a file
	}
	fileIdx := m.cursor - contentCount
	if fileIdx >= m.fileItemCount() {
		return nil
	}
	if m.treeMode {
		item := m.visibleItems[fileIdx]
		if item.isDir {
			return nil
		}
		return item.node.File
	}
	df := m.displayFiles()
	return &df[fileIdx]
}

// selectedAdditionalFile returns the AdditionalFile at the current cursor position,
// or nil if the cursor is not on an additional file.
func (m sidebarModel) selectedAdditionalFile() *types.AdditionalFile {
	contentCount := len(m.contentItems)
	fileCount := m.fileItemCount()
	additionalIdx := m.cursor - contentCount - fileCount
	if additionalIdx < 0 || additionalIdx >= len(m.additionalFiles) {
		return nil
	}
	return &m.additionalFiles[additionalIdx]
}

// navigateFile moves the cursor to the next (dir=+1) or previous (dir=-1)
// file, skipping directory nodes in tree mode. Returns a selectCurrent()
// command if a file was found, or nil if navigation is not possible.
func (m *sidebarModel) navigateFile(dir int) tea.Cmd {
	total := m.totalItems()
	if total == 0 {
		return nil
	}
	contentCount := len(m.contentItems)
	next := m.cursor + dir
	for next >= 0 && next < total {
		// Skip directory nodes in tree mode (file items start at contentCount)
		fileIdx := next - contentCount
		if m.treeMode && fileIdx >= 0 && fileIdx < m.fileItemCount() {
			if m.visibleItems[fileIdx].isDir {
				next += dir
				continue
			}
		}
		break
	}
	if next < 0 || next >= total {
		return nil
	}
	m.cursor = next
	m.ensureVisible()
	return m.selectCurrent()
}

// nextUnreviewed moves the cursor to the next unreviewed item after the current
// cursor position (wrapping is not performed). Skips directory nodes in tree
// mode. Returns a selectCurrent() command if found, or nil if there are no
// unreviewed items ahead.
func (m *sidebarModel) nextUnreviewed() tea.Cmd {
	if !m.reviewTracking {
		return nil
	}
	total := m.totalItems()
	if total == 0 {
		return nil
	}
	contentCount := len(m.contentItems)
	fileCt := m.fileItemCount()

	for next := m.cursor + 1; next < total; next++ {
		// Content items
		if next < contentCount {
			if !m.contentItems[next].Reviewed {
				m.cursor = next
				m.ensureVisible()
				return m.selectCurrent()
			}
			continue
		}
		// File items
		fileIdx := next - contentCount
		if fileIdx < fileCt {
			if m.treeMode {
				item := m.visibleItems[fileIdx]
				if item.isDir {
					continue
				}
				if !item.node.File.Reviewed {
					m.cursor = next
					m.ensureVisible()
					return m.selectCurrent()
				}
			} else {
				if !m.displayFiles()[fileIdx].Reviewed {
					m.cursor = next
					m.ensureVisible()
					return m.selectCurrent()
				}
			}
			continue
		}
		// Additional files
		additionalIdx := next - contentCount - fileCt
		if additionalIdx >= 0 && additionalIdx < len(m.additionalFiles) {
			if !m.additionalFiles[additionalIdx].Reviewed {
				m.cursor = next
				m.ensureVisible()
				return m.selectCurrent()
			}
		}
	}
	return nil
}

// sectionStarts returns the starting cursor indices of non-empty sections.
func (m sidebarModel) sectionStarts() []int {
	var starts []int
	contentCt := len(m.contentItems)
	fileCt := m.fileItemCount()
	additionalCt := len(m.additionalFiles)
	if contentCt > 0 {
		starts = append(starts, 0)
	}
	if fileCt > 0 {
		starts = append(starts, contentCt)
	}
	if additionalCt > 0 {
		starts = append(starts, contentCt+fileCt)
	}
	return starts
}

// jumpToNextSection moves the cursor to the first item of the next section.
func (m *sidebarModel) jumpToNextSection() tea.Cmd {
	starts := m.sectionStarts()
	if len(starts) == 0 {
		return nil
	}
	for _, s := range starts {
		if s > m.cursor {
			m.cursor = s
			m.ensureVisible()
			return m.selectCurrent()
		}
	}
	return nil
}

// jumpToPrevSection moves the cursor to the first item of the previous section.
func (m *sidebarModel) jumpToPrevSection() tea.Cmd {
	starts := m.sectionStarts()
	if len(starts) == 0 {
		return nil
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.cursor {
			m.cursor = starts[i]
			m.ensureVisible()
			return m.selectCurrent()
		}
	}
	return nil
}

// rebuildTree reconstructs the tree from the current file list and updates
// visible items. Safe to call when treeMode is false (no-op).
func (m *sidebarModel) rebuildTree() {
	if !m.treeMode {
		return
	}
	m.treeRoots = buildFileTree(m.files)
	m.visibleItems = flattenTree(m.treeRoots, m.collapsed)
}

// selectContentByID moves the cursor to the content item matching the given ID.
func (m *sidebarModel) selectContentByID(id string) {
	for i, item := range m.contentItems {
		if item.ID == id {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

// selectPath moves the cursor to the item matching the given file path.
func (m *sidebarModel) selectPath(path string) {
	contentCount := len(m.contentItems)
	if m.treeMode {
		for i, item := range m.visibleItems {
			if !item.isDir && item.node.File != nil && item.node.File.Path == path {
				m.cursor = i + contentCount
				return
			}
		}
	} else {
		for i, f := range m.displayFiles() {
			if f.Path == path {
				m.cursor = i + contentCount
				return
			}
		}
	}
}

// currentItemKey returns a (kind, id) pair identifying the cursor's item.
// Kinds: "content", "dir", "file", "additional", or "" if none.
func (m sidebarModel) currentItemKey() (kind, id string) {
	contentCount := len(m.contentItems)
	if m.cursor < contentCount {
		if m.cursor >= 0 {
			return "content", m.contentItems[m.cursor].ID
		}
		return "", ""
	}
	fileIdx := m.cursor - contentCount
	if fileIdx < m.fileItemCount() {
		if m.treeMode {
			item := m.visibleItems[fileIdx]
			if item.isDir {
				return "dir", item.node.Path
			}
			if item.node.File != nil {
				return "file", item.node.File.Path
			}
			return "", ""
		}
		return "file", m.displayFiles()[fileIdx].Path
	}
	if af := m.selectedAdditionalFile(); af != nil {
		return "additional", af.Path
	}
	return "", ""
}

// selectByKey moves the cursor to the item identified by (kind, id).
// Returns false if no match.
func (m *sidebarModel) selectByKey(kind, id string) bool {
	if kind == "" || id == "" {
		return false
	}
	contentCount := len(m.contentItems)
	switch kind {
	case "content":
		for i, item := range m.contentItems {
			if item.ID == id {
				m.cursor = i
				return true
			}
		}
	case "dir":
		if !m.treeMode {
			return false
		}
		for i, item := range m.visibleItems {
			if item.isDir && item.node.Path == id {
				m.cursor = i + contentCount
				return true
			}
		}
	case "file":
		if m.treeMode {
			for i, item := range m.visibleItems {
				if !item.isDir && item.node.File != nil && item.node.File.Path == id {
					m.cursor = i + contentCount
					return true
				}
			}
		} else {
			for i, f := range m.displayFiles() {
				if f.Path == id {
					m.cursor = i + contentCount
					return true
				}
			}
		}
	case "additional":
		fileCount := m.fileItemCount()
		for i, af := range m.additionalFiles {
			if af.Path == id {
				m.cursor = contentCount + fileCount + i
				return true
			}
		}
	}
	return false
}

// collapseAll collapses all directory nodes in the tree.
func (m *sidebarModel) collapseAll() {
	currentPath := ""
	if f := m.selectedFile(); f != nil {
		currentPath = f.Path
	}

	m.collapsed = make(map[string]bool)
	var markCollapsed func(nodes []*fileTreeNode)
	markCollapsed = func(nodes []*fileTreeNode) {
		for _, n := range nodes {
			if n.File == nil {
				m.collapsed[n.Path] = true
				markCollapsed(n.Children)
			}
		}
	}
	markCollapsed(m.treeRoots)
	m.visibleItems = flattenTree(m.treeRoots, m.collapsed)

	if currentPath != "" {
		m.selectPath(currentPath)
	}
	if total := m.totalItems(); total > 0 && m.cursor >= total {
		m.cursor = total - 1
	}
	m.ensureVisible()
}

// ensureVisible adjusts the scroll offset so the cursor stays within the
// visible viewport, mirroring diffViewModel.ensureVisible.
func (m *sidebarModel) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	vh := m.viewportHeight()
	if vh > 0 && m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
}

// sidebarHeaderLines returns the number of lines consumed by section headers
// and blank separators in the sidebar, given item counts.
func sidebarHeaderLines(contentItemCount, additionalFileCount int) int {
	h := 1 // "Files" header is always present
	if contentItemCount > 0 {
		h += 2 // "Artifacts" header + blank separator before "Files"
	}
	if additionalFileCount > 0 {
		h += 2 // blank separator + "Additional Files" header
	}
	return h
}

// viewportHeight returns how many item lines fit in the sidebar viewport.
// Accounts for section headers and dividers that consume vertical space.
func (m sidebarModel) viewportHeight() int {
	headerLines := sidebarHeaderLines(len(m.contentItems), len(m.additionalFiles))
	h := m.height - headerLines
	if h < 0 {
		h = 0
	}
	return h
}

// itemAtLine maps a visual line index (0-based, relative to sidebar content area)
// to a logical item index, or -1 if the line is a header or separator.
// This mirrors the rendering logic in View() to provide accurate click targeting.
func (m sidebarModel) itemAtLine(lineY int) int {
	contentItemCt := len(m.contentItems)
	totalItems := m.totalItems()
	fileItemCt := m.fileItemCount()
	additionalStart := contentItemCt + fileItemCt
	additionalCt := len(m.additionalFiles)

	line := 0

	// If no content items, "Files" header is prepended at line 0
	if contentItemCt == 0 {
		if lineY == line {
			return -1 // Files header
		}
		line++
	}

	// "Artifacts" header (when content items exist)
	if contentItemCt > 0 {
		if lineY == line {
			return -1
		}
		line++
	}

	for idx := m.offset; idx < totalItems; idx++ {
		// "Files" section header between content items and files
		if idx == contentItemCt && contentItemCt > 0 {
			if line > 0 {
				if lineY == line {
					return -1 // blank separator
				}
				line++
			}
			if lineY == line {
				return -1 // "Files" header
			}
			line++
		}

		// "Additional Files" section header
		if idx == additionalStart && additionalCt > 0 {
			if line > 0 {
				if lineY == line {
					return -1 // blank separator
				}
				line++
			}
			if lineY == line {
				return -1 // "Additional Files" header
			}
			line++
		}

		if lineY == line {
			return idx
		}
		line++
	}

	return -1
}

// clampOffset ensures offset and cursor are within valid bounds after the
// item list changes externally.
func (m *sidebarModel) clampOffset() {
	total := m.totalItems()
	if total == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor >= total {
		m.cursor = total - 1
	}
	if m.offset >= total {
		m.offset = total - 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// applyReviewedFilter builds new slices based on reviewFilter state.
// "" = no filter, "unreviewed" = hide reviewed, "reviewed" = hide unreviewed.
// Call after setting files/contentItems/additionalFiles.
func (m *sidebarModel) applyReviewedFilter() {
	if m.reviewFilter == "" {
		m.rebuildGroups() // keep grouped order in sync when files change
		return
	}
	defer m.rebuildGroups()
	keepReviewed := m.reviewFilter == "reviewed"

	var files []types.ChangedFile
	for _, f := range m.files {
		if f.Reviewed == keepReviewed {
			files = append(files, f)
		}
	}
	m.files = files

	var items []types.ContentItem
	for _, item := range m.contentItems {
		if item.Reviewed == keepReviewed {
			items = append(items, item)
		}
	}
	m.contentItems = items

	var additional []types.AdditionalFile
	for _, af := range m.additionalFiles {
		if af.Reviewed == keepReviewed {
			additional = append(additional, af)
		}
	}
	m.additionalFiles = additional
}

// cycleReviewFilter advances the filter: "" → "unreviewed" → "reviewed" → "".
func (m *sidebarModel) cycleReviewFilter() {
	if !m.reviewTracking {
		return
	}
	switch m.reviewFilter {
	case "":
		m.reviewFilter = "unreviewed"
	case "unreviewed":
		m.reviewFilter = "reviewed"
	default:
		m.reviewFilter = ""
	}
}

// reviewFilterLabel returns the header indicator for the current filter state.
func (m sidebarModel) reviewFilterLabel() string {
	switch m.reviewFilter {
	case "unreviewed":
		return " (unreviewed only)"
	case "reviewed":
		return " (reviewed only)"
	default:
		return ""
	}
}

func truncatePath(path string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(path) <= maxLen {
		return path
	}
	if maxLen <= 3 {
		return path[:maxLen]
	}
	return "..." + path[len(path)-(maxLen-3):]
}
