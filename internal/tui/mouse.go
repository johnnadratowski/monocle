package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// paneRegion describes a rectangular area in terminal coordinates.
type paneRegion struct {
	x, y, w, h int
}

// contains returns true if the given terminal coordinates fall within this region.
func (r paneRegion) contains(mx, my int) bool {
	return mx >= r.x && mx < r.x+r.w && my >= r.y && my < r.y+r.h
}

// translate converts absolute terminal coordinates to region-relative coordinates.
func (r paneRegion) translate(mx, my int) (int, int) {
	return mx - r.x, my - r.y
}

// paneLayout holds the computed regions for all major content panes.
type paneLayout struct {
	sidebar paneRegion
	diff    paneRegion
}

const (
	borderW     = 2 // left + right border
	borderH     = 2 // top + bottom border
	titleHeight = 1
)

// computePaneLayout calculates pane content regions from the app's layout state.
// Coordinates are the content area inside each pane's border (where items render).
//
// Bubble Tea v2's alt-screen rendering via ultraviolet introduces a 1-row offset
// between the View string content and the terminal mouse coordinates. The
// mouseOriginY constant accounts for this.
func computePaneLayout(m *appModel) paneLayout {
	const mouseOriginY = 1 // empirical offset for Bubble Tea v2 alt-screen rendering

	// Title bar occupies 1 row. Border top occupies 1 row.
	// Content starts after: mouseOriginY + titleHeight + borderTop(1).
	bodyY := mouseOriginY + titleHeight

	if m.layout == layoutStacked {
		// Sidebar: full width, above diff
		sidebarContentX := 1 // 1 char border left
		sidebarContentY := bodyY + 1 // 1 char border top
		sidebarContentW := m.sidebar.width
		sidebarContentH := m.sidebar.height

		// Diff: below sidebar (sidebar outer height = content + border top + border bottom)
		sidebarOuterH := m.sidebar.height + borderH
		diffContentX := 1
		diffContentY := bodyY + sidebarOuterH + 1 // after sidebar outer + diff border top
		diffContentW := m.diffView.width
		diffContentH := m.diffView.height

		return paneLayout{
			sidebar: paneRegion{sidebarContentX, sidebarContentY, sidebarContentW, sidebarContentH},
			diff:    paneRegion{diffContentX, diffContentY, diffContentW, diffContentH},
		}
	}

	// Horizontal layout: sidebar on left, diff on right
	sidebarContentX := 1
	sidebarContentY := bodyY + 1
	sidebarContentW := m.sidebar.width
	sidebarContentH := m.sidebar.height

	sidebarOuterW := m.sidebar.width + borderW
	diffContentX := sidebarOuterW + 1 // after sidebar outer + diff border left
	diffContentY := bodyY + 1
	diffContentW := m.diffView.width
	diffContentH := m.diffView.height

	return paneLayout{
		sidebar: paneRegion{sidebarContentX, sidebarContentY, sidebarContentW, sidebarContentH},
		diff:    paneRegion{diffContentX, diffContentY, diffContentW, diffContentH},
	}
}

// overlayRegion computes the bounding box for a centered overlay,
// mirroring the same centering logic used by overlayOn().
func overlayRegion(screenW, screenH, overlayW, overlayH int) paneRegion {
	topPad := (screenH - overlayH) / 2
	if topPad < 2 {
		topPad = 2
	}
	leftPad := (screenW - overlayW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	return paneRegion{leftPad, topPad, overlayW, overlayH}
}

// overlayContentRegion returns the content region inside a modal overlay,
// accounting for the RoundedBorder (1 char each side) and Padding(1, 2).
// The ModalBorder style uses: Border(RoundedBorder()) + Padding(1, 2).
// So content starts at: border(1) + paddingLeft(2) = 3 from left,
// border(1) + paddingTop(1) = 2 from top.
func overlayContentRegion(overlay paneRegion) paneRegion {
	const (
		modalBorderX  = 1 // border left
		modalPaddingX = 2 // padding left
		modalBorderY  = 1 // border top
		modalPaddingY = 1 // padding top
		offsetX       = modalBorderX + modalPaddingX
		offsetY       = modalBorderY + modalPaddingY
	)
	return paneRegion{
		x: overlay.x + offsetX,
		y: overlay.y + offsetY,
		w: overlay.w - 2*(modalBorderX+modalPaddingX),
		h: overlay.h - 2*(modalBorderY+modalPaddingY),
	}
}

// computeOverlayDimensions measures the rendered overlay to determine its size.
// Only called on mouse click events, not every frame.
func computeOverlayDimensions(m *appModel) (int, int) {
	var content string
	switch m.overlay {
	case overlayComment:
		content = m.commentEditor.View()
	case overlayReview:
		content = m.reviewSummary.View()
	case overlayHelp:
		content = m.help.View()
	case overlayRefPicker:
		content = m.refPicker.View()
	case overlayConfirm:
		content = m.confirm.View()
	case overlayRegisterPrompt:
		content = m.registerPrompt.View()
	case overlayConnectionInfo:
		content = m.connectionInfo.View()
	case overlayHistory:
		content = m.history.View()
	default:
		return 0, 0
	}
	return lipgloss.Width(content), lipgloss.Height(content)
}

// mouseScrollLines is the number of lines to scroll per wheel tick.
const mouseScrollLines = 3

// handleMouseClick processes left-click events for pane focus, sidebar selection,
// diff cursor positioning, and overlay interactions.
func (m appModel) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	// If overlay is active, route click there
	if m.overlay != overlayNone {
		return m.handleOverlayClick(msg.X, msg.Y)
	}

	layout := computePaneLayout(&m)

	if layout.sidebar.contains(msg.X, msg.Y) {
		// Focus sidebar
		m.focus = focusSidebar
		m.sidebar.focused = true
		m.diffView.focused = false

		// Select clicked item
		_, relY := layout.sidebar.translate(msg.X, msg.Y)
		return m.handleSidebarClick(relY)
	}

	if layout.diff.contains(msg.X, msg.Y) {
		// Focus diff
		m.focus = focusMain
		m.sidebar.focused = false
		m.diffView.focused = true

		// Position cursor and start drag tracking
		_, relY := layout.diff.translate(msg.X, msg.Y)
		m.diffView.handleMouseClick(relY)
		return m, nil
	}

	return m, nil
}

// handleMouseWheel routes scroll wheel events to the pane under the cursor (hover-based).
func (m appModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	// If overlay is active, route wheel to scrollable overlay
	if m.overlay != overlayNone {
		return m.handleOverlayWheel(msg)
	}

	layout := computePaneLayout(&m)

	if layout.sidebar.contains(msg.X, msg.Y) {
		maxOffset := m.sidebar.totalItems() - m.sidebar.viewportHeight()
		if maxOffset < 0 {
			maxOffset = 0
		}
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				if m.sidebar.offset < maxOffset {
					m.sidebar.offset++
				}
			} else if msg.Button == tea.MouseWheelUp {
				if m.sidebar.offset > 0 {
					m.sidebar.offset--
				}
			}
		}
		return m, nil
	}

	if layout.diff.contains(msg.X, msg.Y) {
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				m.diffView.ScrollDown()
			} else if msg.Button == tea.MouseWheelUp {
				m.diffView.ScrollUp()
			}
		}
		return m, nil
	}

	return m, nil
}

// handleMouseMotion processes drag events for visual selection in the diff view.
func (m appModel) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.diffView.mouseDragActive {
		return m, nil
	}

	layout := computePaneLayout(&m)
	_, relY := layout.diff.translate(msg.X, msg.Y)
	m.diffView.handleMouseMotion(relY)
	return m, nil
}

// handleMouseRelease ends drag tracking and finalizes visual selection.
func (m appModel) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	_ = msg
	m.diffView.handleMouseRelease()
	return m, nil
}

// handleOverlayClick dispatches clicks to the active overlay, or dismisses it
// if the click is outside the overlay bounds.
func (m appModel) handleOverlayClick(x, y int) (tea.Model, tea.Cmd) {
	ow, oh := computeOverlayDimensions(&m)
	region := overlayRegion(m.width, m.height, ow, oh)

	if !region.contains(x, y) {
		// Click outside overlay — dismiss for dismissable overlays
		switch m.overlay {
		case overlayHelp:
			m.help.active = false
			m.overlay = overlayNone
		case overlayConnectionInfo:
			m.connectionInfo.active = false
			m.overlay = overlayNone
		case overlayComment:
			m.commentEditor.active = false
			m.overlay = overlayNone
			return m, func() tea.Msg { return cancelCommentMsg{} }
		case overlayReview:
			m.reviewSummary.active = false
			m.overlay = overlayNone
			return m, func() tea.Msg { return cancelSubmitMsg{} }
		case overlayConfirm:
			m.confirm.active = false
			m.overlay = overlayNone
			dontAsk := m.confirm.dontAsk
			return m, func() tea.Msg { return cancelConfirmMsg{dontAsk: dontAsk} }
		case overlayRefPicker:
			m.refPicker.active = false
			m.overlay = overlayNone
		case overlayHistory:
			m.history.active = false
			m.overlay = overlayNone
		}
		return m, nil
	}

	// Click inside overlay — translate to content coordinates
	content := overlayContentRegion(region)
	if !content.contains(x, y) {
		return m, nil // click on border/padding, ignore
	}
	cx, cy := content.translate(x, y)

	switch m.overlay {
	case overlayComment:
		m.commentEditor.handleClick(cx, cy)
	case overlayReview:
		m.reviewSummary.handleClick(cx, cy)
	case overlayConfirm:
		m.confirm.handleClick(cx, cy)
	case overlayRefPicker:
		cmd, handled := m.refPicker.handleClick(cy)
		if handled {
			m.overlay = overlayNone
			return m, cmd
		}
	}

	return m, nil
}

// handleOverlayWheel routes scroll wheel events to scrollable overlays.
func (m appModel) handleOverlayWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				m.help.scrollOffset++
			} else if msg.Button == tea.MouseWheelUp {
				if m.help.scrollOffset > 0 {
					m.help.scrollOffset--
				}
			}
		}
	case overlayRefPicker:
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				maxOffset := len(m.refPicker.entries) - m.refPicker.viewportHeight() + 1
				if maxOffset < 0 {
					maxOffset = 0
				}
				if m.refPicker.offset < maxOffset {
					m.refPicker.offset++
				}
			} else if msg.Button == tea.MouseWheelUp {
				if m.refPicker.offset > 0 {
					m.refPicker.offset--
				}
			}
		}
	case overlayHistory:
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				m.history.scrollOffset++
			} else if msg.Button == tea.MouseWheelUp {
				if m.history.scrollOffset > 0 {
					m.history.scrollOffset--
				}
			}
		}
	}
	return m, nil
}

// handleSidebarClick selects the item at the given relative Y coordinate.
func (m appModel) handleSidebarClick(relY int) (tea.Model, tea.Cmd) {
	itemIdx := m.sidebar.itemAtLine(relY)
	if itemIdx < 0 {
		return m, nil // clicked a header or separator
	}

	// Check if clicked item is a directory in tree mode
	contentCount := len(m.sidebar.contentItems)
	fileItemCount := m.sidebar.fileItemCount()
	additionalStart := contentCount + fileItemCount

	fileIdx := itemIdx - contentCount
	if m.sidebar.treeMode && fileIdx >= 0 && fileIdx < len(m.sidebar.visibleItems) {
		item := m.sidebar.visibleItems[fileIdx]
		if item.isDir {
			// Toggle collapse
			dirPath := item.node.Path
			if m.sidebar.collapsed[dirPath] {
				delete(m.sidebar.collapsed, dirPath)
			} else {
				m.sidebar.collapsed[dirPath] = true
			}
			m.sidebar.visibleItems = flattenTree(m.sidebar.treeRoots, m.sidebar.collapsed)
			if total := m.sidebar.totalItems(); total > 0 && m.sidebar.cursor >= total {
				m.sidebar.cursor = total - 1
			}
			m.sidebar.ensureVisible()
			return m, nil
		}
	}

	// Select the clicked item
	m.sidebar.cursor = itemIdx
	m.sidebar.ensureVisible()

	// Emit selection message
	if itemIdx < contentCount {
		ci := m.sidebar.contentItems[itemIdx]
		return m, func() tea.Msg {
			return sidebarSelectMsg{isContent: true, contentID: ci.ID}
		}
	} else if itemIdx < additionalStart {
		var filePath string
		if m.sidebar.treeMode {
			vi := m.sidebar.visibleItems[fileIdx]
			if vi.node.File != nil {
				filePath = vi.node.File.Path
			}
		} else {
			filePath = m.sidebar.files[fileIdx].Path
		}
		if filePath != "" {
			return m, func() tea.Msg {
				return sidebarSelectMsg{path: filePath}
			}
		}
	} else {
		additionalIdx := itemIdx - additionalStart
		if additionalIdx >= 0 && additionalIdx < len(m.sidebar.additionalFiles) {
			af := m.sidebar.additionalFiles[additionalIdx]
			return m, func() tea.Msg {
				return sidebarSelectMsg{path: af.Path, isAdditionalFile: true}
			}
		}
	}

	return m, nil
}
