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

// paneLayout holds the computed regions for all major content panes. doc is
// the doc pane under the diff, empty while it is closed; its first row is the
// pane's title.
type paneLayout struct {
	sidebar paneRegion
	diff    paneRegion
	doc     paneRegion
}

const (
	borderW     = 2 // left + right border
	borderH     = 2 // top + bottom border
	titleHeight = 1
)

// computePaneLayout calculates pane content regions from the app's layout state.
// Coordinates are the content area inside each pane's border (where items render).
//
// They are the View's own rows and columns: Bubble Tea reports a mouse event
// at the 0-based cell clicked, and the View is drawn from the screen's top-left
// cell, so a click on a row arrives as that row. (These regions once added a
// row, which under tmux put every click one row below where it landed; the
// overlays never did.)
func computePaneLayout(m *appModel) paneLayout {
	l := computeMainPanes(m)
	if m.docPane.active {
		// The doc pane is its own box directly under the diff's, so its content
		// starts past the diff's bottom border and its own top one.
		l.doc = paneRegion{l.diff.x, l.diff.y + l.diff.h + borderH, l.diff.w, m.docPane.height}
	}
	return l
}

// computeMainPanes is computePaneLayout's sidebar and diff.
func computeMainPanes(m *appModel) paneLayout {
	// Title bar occupies 1 row. Border top occupies 1 row.
	// Content starts after: titleHeight + borderTop(1).
	bodyY := titleHeight

	if m.sidebarHidden {
		// The diff has the body to itself: no sidebar box to its left or above.
		return paneLayout{diff: paneRegion{1, bodyY + 1, m.diffView.width, m.diffView.height}}
	}

	if m.layout == layoutStacked {
		// Sidebar: full width, above diff
		sidebarContentX := 1         // 1 char border left
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
	case overlayInfo:
		content = m.infoBanner.View()
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
		return m, m.diffView.cursorMoved()
	}

	if layout.doc.contains(msg.X, msg.Y) {
		// A tour note's labels are the pane's only clickable things. A click on
		// one sends what its command sends (`:view 2`, `:related 2`), so both
		// run the same code, and does nothing else: focus and the diff stay
		// where they are.
		relX, relY := layout.doc.translate(msg.X, msg.Y)
		if act, ok := m.docPane.linkAt(relX, relY); ok {
			return m, func() tea.Msg { return act }
		}
	}

	return m, nil
}

// handleMouseWheel routes scroll wheel events. Over the doc pane the wheel
// scrolls it. The sidebar scrolls only when the cursor is clearly hovering over
// a visible sidebar; every other wheel event —
// over the diff, over the title/borders, or outside the computed regions entirely
// — scrolls the diff. This makes wheel scrolling work anywhere in the window
// regardless of which pane is focused, and is robust to layout modes where the
// computed regions don't perfectly match the rendered content.
func (m appModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	// If overlay is active, route wheel to scrollable overlay
	if m.overlay != overlayNone {
		return m.handleOverlayWheel(msg)
	}

	// A sideways wheel (a trackpad swipe, or shift+wheel) scrolls the diff sideways,
	// like h / L, when lines are not wrapped. One step per event:
	// a trackpad sends many.
	if !m.diffView.wrap {
		sideways := msg.Button == tea.MouseWheelLeft || msg.Button == tea.MouseWheelRight
		shifted := msg.Mod.Contains(tea.ModShift) && (msg.Button == tea.MouseWheelUp || msg.Button == tea.MouseWheelDown)
		if sideways || shifted {
			if msg.Button == tea.MouseWheelRight || msg.Button == tea.MouseWheelDown {
				m.diffView.ScrollRight()
			} else {
				m.diffView.ScrollLeft()
			}
			return m, nil
		}
	}

	layout := computePaneLayout(&m)

	if layout.doc.contains(msg.X, msg.Y) {
		// Over the doc pane the wheel scrolls it — a tour note too long for
		// the pane, or a document — and not the diff above it. Over the label
		// rows it scrolls them, when there are more than fit.
		if _, relY := layout.doc.translate(msg.X, msg.Y); m.docPane.overPins(relY) {
			if msg.Button == tea.MouseWheelDown {
				m.docPane.scrollPinsDown()
			} else if msg.Button == tea.MouseWheelUp {
				m.docPane.scrollPinsUp()
			}
			return m, nil
		}
		for i := 0; i < mouseScrollLines; i++ {
			if msg.Button == tea.MouseWheelDown {
				m.docPane.scrollDown()
			} else if msg.Button == tea.MouseWheelUp {
				m.docPane.scrollUp()
			}
		}
		return m, nil
	}

	if !m.sidebarHidden && layout.sidebar.contains(msg.X, msg.Y) {
		// Allow scrolling until the last item can reach the top. Header/group
		// lines make an exact "last visible offset" hard to compute, so cap at
		// the last item (mild over-scroll is fine; under-scroll hid the bottom).
		maxOffset := m.sidebar.totalItems() - 1
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

	// Default: any wheel event not clearly over the sidebar scrolls the diff.
	for i := 0; i < mouseScrollLines; i++ {
		if msg.Button == tea.MouseWheelDown {
			m.diffView.ScrollDown()
		} else if msg.Button == tea.MouseWheelUp {
			m.diffView.ScrollUp()
		}
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
	return m, m.diffView.cursorMoved()
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
			return m, func() tea.Msg { return cancelConfirmMsg{} }
		case overlayRefPicker:
			m.refPicker.active = false
			m.overlay = overlayNone
		case overlayHistory:
			m.history.active = false
			m.overlay = overlayNone
		case overlayInfo:
			m.infoBanner.active = false
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
	case overlayVersionPicker:
		cmd, handled := m.versionPicker.handleClick(cy)
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
			filePath = m.sidebar.displayFiles()[fileIdx].Path
		}
		if filePath != "" {
			return m, func() tea.Msg {
				return sidebarSelectMsg{path: filePath}
			}
		}
	} else {
		additionalIdx := itemIdx - additionalStart
		if additionalIdx >= 0 && additionalIdx < len(m.sidebar.additionalFiles) {
			af := m.sidebar.displayAdditionalFiles()[additionalIdx]
			return m, func() tea.Msg {
				return sidebarSelectMsg{path: af.Path, isAdditionalFile: true}
			}
		}
	}

	return m, nil
}
