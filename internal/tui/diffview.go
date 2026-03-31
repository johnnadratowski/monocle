package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

type diffStyle int

const (
	diffStyleUnified diffStyle = iota
	diffStyleSplit
	diffStyleFile // raw file content, no diff coloring
)

// diffViewLine represents a rendered line in the diff view.
type diffViewLine struct {
	kind       types.DiffLineKind
	oldLineNum int
	newLineNum int
	content    string
	isHunk     bool
	hunkHeader string
	isComment  bool
	comment    *types.ReviewComment

	// Paired line content for intra-line diff highlighting (unified mode)
	pairContent string

	// Markdown rendering state
	mdInCodeBlock bool
	mdIsFence     bool
	mdCodeLang    string

	// Split diff: right side
	isSplit      bool
	rightKind    types.DiffLineKind
	rightLineNum int
	rightContent string
	rightEmpty   bool // true if this side is a blank filler
	leftEmpty    bool
}

type diffViewModel struct {
	path      string
	hunks     []types.DiffHunk
	comments  []types.ReviewComment
	lines     []diffViewLine
	cursor    int
	offset    int // scroll offset
	width     int
	height    int
	focused   bool
	style     diffStyle
	theme     *Theme
	hl        *highlighter

	hOffset int  // horizontal scroll offset (runes)
	wrap    bool // soft-wrap long lines
	tabSize int  // spaces per tab character

	// Visual mode
	visualMode  bool
	visualStart int

	// Mouse drag state
	mouseDragActive bool

	// Comment expansion on hover
	expandedCommentID string // ID of the currently expanded comment (empty = none)
	expandSeq         int    // sequence counter; incremented on each cursor move to debounce

	// Content view mode (for plans/docs)
	contentMode        bool
	contentID          string
	contentTitle       string
	contentHasDiff     bool   // true when a previous version exists for diffing
	contentDiffContent string // current content text, for toggling back from diff view
	mdStyler           *markdownStyler

	// Content diff auto-switch (from config diff_style)
	preferredContentDiffStyle diffStyle // unified or split
	autoContentDiff           bool      // true when plans should auto-show diffs on version 2+

	// Additional file mode (external files, no diff)
	additionalFilePath string

	keys *KeyMap
}

// isViewingContentItem returns true when the diff view is showing a content item,
// whether in content mode (raw text) or diff mode (unified/split diff).
func (m diffViewModel) isViewingContentItem() bool {
	return m.contentID != ""
}

// clearFileState resets all file-display fields without touching content item
// metadata (contentID, contentMode). Call when no file should be shown.
func (m *diffViewModel) clearFileState() {
	m.path = ""
	m.hunks = nil
	m.lines = nil
	m.comments = nil
	m.cursor = 0
	m.offset = 0
	m.hOffset = 0
	m.visualMode = false
	m.expandedCommentID = ""
	m.expandSeq++
}

func newDiffViewModel(theme *Theme, keys *KeyMap) diffViewModel {
	return diffViewModel{
		theme:    theme,
		hl:       newHighlighter(),
		mdStyler: newMarkdownStyler(*theme),
		keys:     keys,
	}
}

type loadDiffMsg struct {
	path     string
	result   *types.DiffResult
	comments []types.ReviewComment
}

type requestFileContentMsg struct {
	path string
}

type loadFileContentMsg struct {
	path     string
	content  string
	err      error
	comments []types.ReviewComment
}

type loadAdditionalFileMsg struct {
	path     string
	content  string
	err      error
	comments []types.ReviewComment
}

// commentExpandTickMsg fires after a delay to expand a hovered comment.
type commentExpandTickMsg struct {
	seq int // must match m.expandSeq to be honoured
}

const commentExpandDelay = 500 * time.Millisecond

// cursorMoved handles comment expansion state when the cursor changes position.
// It collapses any expanded comment and schedules a new expand tick if the cursor
// is now on a comment line.
func (m *diffViewModel) cursorMoved() tea.Cmd {
	m.expandSeq++
	m.expandedCommentID = ""

	c := m.CursorComment()
	if c == nil {
		return nil
	}
	seq := m.expandSeq
	return tea.Tick(commentExpandDelay, func(time.Time) tea.Msg {
		return commentExpandTickMsg{seq: seq}
	})
}

func (m diffViewModel) Init() tea.Cmd {
	return nil
}

func (m diffViewModel) Update(msg tea.Msg) (diffViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case loadDiffMsg:
		m.contentMode = false
		m.contentID = ""
		m.contentTitle = ""
		m.additionalFilePath = ""
		sameFile := msg.path == m.path
		if msg.result != nil {
			m.hunks = msg.result.Hunks
		} else {
			m.hunks = nil
		}
		m.path = msg.path
		m.comments = msg.comments
		// If in file view mode, store hunks but fetch file content instead
		if m.style == diffStyleFile {
			path := m.path
			return m, func() tea.Msg { return requestFileContentMsg{path: path} }
		}
		prevCursor := m.cursor
		prevOffset := m.offset
		m.buildLines()
		if sameFile && prevCursor < len(m.lines) {
			m.cursor = prevCursor
			m.offset = prevOffset
		} else {
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.hOffset = 0
			m.visualMode = false
		}
		return m, nil

	case loadContentMsg:
		isReload := m.contentMode && m.contentID == msg.id
		m.contentMode = true
		m.contentID = msg.id
		m.contentTitle = msg.title
		m.contentHasDiff = msg.hasPreviousVersion
		m.contentDiffContent = msg.content
		m.additionalFilePath = ""
		if msg.contentType != "" {
			ext := msg.contentType
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			m.path = "content" + ext
		} else {
			m.path = msg.id
		}
		m.hunks = nil
		m.comments = msg.comments
		prevCursor := m.cursor
		prevOffset := m.offset
		m.buildContentLines(msg.content)
		if isReload && prevCursor < len(m.lines) {
			m.cursor = m.nearestSelectable(prevCursor, 1)
			m.offset = prevOffset
		} else {
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.visualMode = false
		}
		m.hOffset = 0

		// Auto-switch to preferred diff style when a previous version exists
		if msg.autoSwitchDiff && m.autoContentDiff {
			contentID := m.contentID
			style := m.preferredContentDiffStyle
			return m, func() tea.Msg {
				return requestContentDiffMsg{contentID: contentID, preferredStyle: style}
			}
		}

		return m, nil

	case loadContentDiffMsg:
		if msg.err != nil || msg.result == nil || len(msg.result.Hunks) == 0 {
			return m, nil // stay in content mode on error or no changes
		}
		m.contentMode = false
		m.hunks = msg.result.Hunks
		m.comments = msg.comments
		m.style = msg.preferredStyle
		m.buildLines()
		m.cursor = m.nearestSelectable(0, 1)
		m.offset = 0
		m.hOffset = 0
		m.visualMode = false
		return m, nil

	case loadFileContentMsg:
		if msg.err != nil {
			m.style = diffStyleFile
			m.path = msg.path
			m.hunks = nil
			m.comments = nil
			m.lines = []diffViewLine{{
				kind:       types.DiffLineContext,
				content:    msg.err.Error(),
				newLineNum: 0,
			}}
			m.cursor = 0
			m.offset = 0
			m.hOffset = 0
			return m, nil
		}
		m.style = diffStyleFile
		m.comments = msg.comments
		sameFile := msg.path == m.path
		prevCursor := m.cursor
		prevOffset := m.offset
		m.buildFileViewLines(msg.content)
		if sameFile && prevCursor < len(m.lines) {
			m.cursor = m.nearestSelectable(prevCursor, 1)
			m.offset = prevOffset
		} else {
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.visualMode = false
		}
		m.hOffset = 0
		return m, nil

	case loadAdditionalFileMsg:
		if msg.err != nil {
			return m, nil
		}
		m.contentMode = false
		m.contentID = ""
		m.contentTitle = ""
		m.additionalFilePath = msg.path
		m.path = msg.path
		m.hunks = nil
		m.comments = msg.comments
		m.style = diffStyleFile
		m.buildFileViewLines(msg.content)
		m.cursor = m.nearestSelectable(0, 1)
		m.offset = 0
		m.hOffset = 0
		m.visualMode = false
		return m, nil

	case commentExpandTickMsg:
		if msg.seq == m.expandSeq {
			if c := m.CursorComment(); c != nil {
				m.expandedCommentID = c.ID
				m.ensureVisible()
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		var expandCmd tea.Cmd
		key := msg.String()
		switch {
		case Matches(key, m.keys.Down):
			if m.isCursorOffScreen() {
				m.cursor = m.nearestSelectable(m.offset, 1)
			} else {
				m.cursor = m.nextSelectable(m.cursor, 1)
			}
			m.ensureVisible()
			expandCmd = m.cursorMoved()
		case Matches(key, m.keys.Up):
			if m.isCursorOffScreen() {
				m.cursor = m.nearestSelectable(m.lastVisibleLine(), -1)
			} else {
				m.cursor = m.nextSelectable(m.cursor, -1)
			}
			m.ensureVisible()
			expandCmd = m.cursorMoved()
		case Matches(key, m.keys.Top):
			m.cursor = m.nearestSelectable(0, 1)
			m.ensureVisible()
			expandCmd = m.cursorMoved()
		case Matches(key, m.keys.Bottom):
			if len(m.lines) > 0 {
				m.cursor = m.nearestSelectable(len(m.lines)-1, -1)
			}
			m.ensureVisible()
			expandCmd = m.cursorMoved()
		case Matches(key, m.keys.Visual):
			if !m.visualMode {
				m.visualMode = true
				m.visualStart = m.cursor
			} else {
				m.visualMode = false
			}
		case key == "esc":
			m.visualMode = false
		case key == "h" || key == "left":
			m.ScrollLeft()
		case key == "l" || key == "right":
			m.ScrollRight()
		case Matches(key, m.keys.Comment):
			// If cursor is on a comment, edit it
			if c := m.CursorComment(); c != nil {
				comment := *c
				return m, func() tea.Msg { return editCommentMsg{comment: &comment} }
			}
			// Otherwise open new comment editor
			if m.contentMode {
				if m.visualMode {
					start, end := m.visualRange()
					return m, openCommentCmd(m.contentID, start, end, types.TargetContent)
				}
				line := m.currentDiffLine()
				if line > 0 {
					return m, openCommentCmd(m.contentID, line, line, types.TargetContent)
				}
			} else {
				targetType := types.TargetFile
				targetRef := m.path
				if m.additionalFilePath != "" {
					targetType = types.TargetAdditionalFile
					targetRef = m.additionalFilePath
				}
				if m.visualMode {
					start, end := m.visualRange()
					return m, openCommentCmd(targetRef, start, end, targetType)
				}
				line := m.currentDiffLine()
				if line > 0 {
					return m, openCommentCmd(targetRef, line, line, targetType)
				}
			}
		case Matches(key, m.keys.Suggest):
			// Suggest edit — requires a line target (no file-level suggestions)
			if m.contentMode {
				if m.visualMode {
					start, end := m.visualRange()
					idxStart, idxEnd := m.orderedVisualIndices()
					code := m.selectedContent(idxStart, idxEnd)
					return m, openSuggestCmd(m.contentID, start, end, types.TargetContent, code)
				}
				line := m.currentDiffLine()
				if line > 0 {
					code := m.selectedContent(m.cursor, m.cursor)
					return m, openSuggestCmd(m.contentID, line, line, types.TargetContent, code)
				}
			} else {
				targetType := types.TargetFile
				targetRef := m.path
				if m.additionalFilePath != "" {
					targetType = types.TargetAdditionalFile
					targetRef = m.additionalFilePath
				}
				if m.visualMode {
					start, end := m.visualRange()
					idxStart, idxEnd := m.orderedVisualIndices()
					code := m.selectedContent(idxStart, idxEnd)
					return m, openSuggestCmd(targetRef, start, end, targetType, code)
				}
				line := m.currentDiffLine()
				if line > 0 {
					code := m.selectedContent(m.cursor, m.cursor)
					return m, openSuggestCmd(targetRef, line, line, targetType, code)
				}
			}
		case Matches(key, m.keys.FileComment):
			// File-level comment
			if m.contentMode {
				return m, openFileCommentCmd(m.contentID, types.TargetContent)
			}
			if m.additionalFilePath != "" {
				return m, openFileCommentCmd(m.additionalFilePath, types.TargetAdditionalFile)
			}
			if m.path != "" {
				return m, openFileCommentCmd(m.path, types.TargetFile)
			}
		case key == "d":
			// Delete comment under cursor
			if c := m.CursorComment(); c != nil {
				commentID := c.ID
				return m, func() tea.Msg { return deleteCommentMsg{commentID: commentID} }
			}
		case key == "x":
			// Toggle resolved on comment under cursor
			if c := m.CursorComment(); c != nil {
				commentID := c.ID
				return m, func() tea.Msg { return resolveCommentMsg{commentID: commentID} }
			}
		}
		if expandCmd != nil {
			return m, expandCmd
		}
	}
	return m, nil
}

func (m diffViewModel) View() string {
	if m.width == 0 || len(m.lines) == 0 {
		if m.path == "" {
			return renderSplash(m.width, m.height)
		}
		if m.contentMode {
			return centerBlock([]string{"Empty content"}, m.width, m.height)
		}
		if m.style == diffStyleFile {
			return centerBlock([]string{"File not available"}, m.width, m.height)
		}
		return centerBlock([]string{"No changes"}, m.width, m.height)
	}

	var b strings.Builder
	screenUsed := 0

	for i := m.offset; i < len(m.lines) && screenUsed < m.height; i++ {
		line := m.lines[i]
		selected := i == m.cursor
		inVisual := m.visualMode && m.inVisualRange(i)

		var rendered string
		if line.isHunk {
			rendered = m.renderHunkHeader(line, selected)
		} else if line.isComment {
			rendered = m.renderCommentLine(line, selected)
		} else if line.isSplit {
			rendered = m.renderSplitLine(line, selected, inVisual)
		} else if m.style == diffStyleFile || m.contentMode {
			gutterWidth := 4
			contentWidth := m.width - gutterWidth
			rendered = m.renderContentLine(line, gutterWidth, contentWidth, selected, inVisual)
		} else {
			gutterWidth := 10
			contentWidth := m.width - gutterWidth
			rendered = m.renderDiffLine(line, gutterWidth, contentWidth, selected, inVisual)
		}

		// rendered may contain multiple lines in wrap mode
		renderedLines := strings.Split(rendered, "\n")
		for _, rl := range renderedLines {
			if screenUsed >= m.height {
				break
			}
			if screenUsed > 0 {
				b.WriteString("\n")
			}
			// Truncate to pane width to prevent terminal-level wrapping
			// when background colors cause lines to bleed past the border.
			b.WriteString(ansi.Truncate(rl, m.width, ""))
			screenUsed++
		}
	}

	return b.String()
}

func (m *diffViewModel) buildLines() {
	m.lines = nil

	if m.style == diffStyleSplit {
		m.buildSplitLines()
		return
	}

	isMd := isMarkdownFile(m.path)

	// File-level comments (LineStart == 0) rendered before hunks
	for i := range m.comments {
		c := &m.comments[i]
		if c.TargetRef == m.path && c.LineStart == 0 {
			m.lines = append(m.lines, diffViewLine{
				isComment: true,
				comment:   c,
				content:   formatInlineComment(c),
			})
		}
	}

	inCodeBlock := false
	codeLang := ""

	for _, hunk := range m.hunks {
		// Hunk header
		m.lines = append(m.lines, diffViewLine{
			isHunk:     true,
			hunkHeader: hunk.Header,
			content:    fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Header),
		})

		// Diff lines with inline comments inserted after target line
		for _, dl := range hunk.Lines {
			// Track code fence state for markdown files
			isFence := false
			if isMd {
				if fence := codeFencePattern.FindStringSubmatch(dl.Content); fence != nil {
					isFence = true
					// Only advance state on context + added lines (new file version)
					if dl.Kind != types.DiffLineRemoved {
						if !inCodeBlock {
							inCodeBlock = true
							codeLang = fence[1]
						} else {
							inCodeBlock = false
							codeLang = ""
						}
					}
				}
			}

			m.lines = append(m.lines, diffViewLine{
				kind:          dl.Kind,
				oldLineNum:    dl.OldLineNum,
				newLineNum:    dl.NewLineNum,
				content:       m.expandTabs(dl.Content),
				mdInCodeBlock: inCodeBlock && isMd && !isFence,
				mdIsFence:     isFence,
				mdCodeLang:    codeLang,
			})

			// Insert comments after their last targeted line
			for i := range m.comments {
				c := &m.comments[i]
				anchor := c.LineEnd
				if anchor == 0 {
					anchor = c.LineStart
				}
				if c.TargetRef == m.path && anchor == dl.NewLineNum && dl.NewLineNum > 0 {
					m.lines = append(m.lines, diffViewLine{
						isComment: true,
						comment:   c,
						content:   formatInlineComment(c),
					})
				}
			}
		}
	}

	m.pairLines()
}

// buildContentLines builds lines for a content item (plan/doc) displayed as a document.
func (m *diffViewModel) buildContentLines(content string) {
	m.lines = nil

	// File-level comments (LineStart == 0) rendered before content
	for i := range m.comments {
		c := &m.comments[i]
		if c.TargetRef == m.contentID && c.LineStart == 0 {
			m.lines = append(m.lines, diffViewLine{
				isComment: true,
				comment:   c,
				content:   formatInlineComment(c),
			})
		}
	}

	isMd := isMarkdownContent(m.path)
	inCodeBlock := false
	codeLang := ""
	rawLines := strings.Split(content, "\n")
	for i, line := range rawLines {
		lineNum := i + 1

		// Track code fence state for markdown rendering
		isFence := false
		if isMd {
			if fence := codeFencePattern.FindStringSubmatch(line); fence != nil {
				isFence = true
				if !inCodeBlock {
					inCodeBlock = true
					codeLang = fence[1]
				} else {
					inCodeBlock = false
					codeLang = ""
				}
			}
		}

		m.lines = append(m.lines, diffViewLine{
			kind:          types.DiffLineContext,
			newLineNum:    lineNum,
			content:       m.expandTabs(line),
			mdInCodeBlock: inCodeBlock && isMd && !isFence,
			mdIsFence:     isFence,
			mdCodeLang:    codeLang,
		})

		// Insert comments after their last targeted line
		for j := range m.comments {
			c := &m.comments[j]
			anchor := c.LineEnd
			if anchor == 0 {
				anchor = c.LineStart
			}
			if c.TargetRef == m.contentID && anchor == lineNum {
				m.lines = append(m.lines, diffViewLine{
					isComment: true,
					comment:   c,
					content:   formatInlineComment(c),
				})
			}
		}
	}
}

// buildFileViewLines builds lines from raw file content for file view mode.
// Uses m.path for comment matching (unlike buildContentLines which uses m.contentID).
func (m *diffViewModel) buildFileViewLines(content string) {
	m.lines = nil

	// File-level comments (LineStart == 0)
	for i := range m.comments {
		c := &m.comments[i]
		if c.TargetRef == m.path && c.LineStart == 0 {
			m.lines = append(m.lines, diffViewLine{
				isComment: true,
				comment:   c,
				content:   formatInlineComment(c),
			})
		}
	}

	isMd := isMarkdownFile(m.path)
	inCodeBlock := false
	codeLang := ""
	rawLines := strings.Split(content, "\n")
	for i, line := range rawLines {
		lineNum := i + 1

		isFence := false
		if isMd {
			if fence := codeFencePattern.FindStringSubmatch(line); fence != nil {
				isFence = true
				if !inCodeBlock {
					inCodeBlock = true
					codeLang = fence[1]
				} else {
					inCodeBlock = false
					codeLang = ""
				}
			}
		}

		m.lines = append(m.lines, diffViewLine{
			kind:          types.DiffLineContext,
			newLineNum:    lineNum,
			content:       m.expandTabs(line),
			mdInCodeBlock: inCodeBlock && isMd && !isFence,
			mdIsFence:     isFence,
			mdCodeLang:    codeLang,
		})

		// Insert comments after their last targeted line
		for j := range m.comments {
			c := &m.comments[j]
			anchor := c.LineEnd
			if anchor == 0 {
				anchor = c.LineStart
			}
			if c.TargetRef == m.path && anchor == lineNum {
				m.lines = append(m.lines, diffViewLine{
					isComment: true,
					comment:   c,
					content:   formatInlineComment(c),
				})
			}
		}
	}
}

func (m *diffViewModel) buildSplitLines() {
	isMd := isMarkdownFile(m.path)

	// File-level comments (LineStart == 0) rendered before hunks
	for i := range m.comments {
		c := &m.comments[i]
		if c.TargetRef == m.path && c.LineStart == 0 {
			m.lines = append(m.lines, diffViewLine{
				isComment: true,
				comment:   c,
				content:   formatInlineComment(c),
			})
		}
	}

	inCodeBlock := false
	codeLang := ""

	for _, hunk := range m.hunks {
		m.lines = append(m.lines, diffViewLine{
			isHunk:     true,
			hunkHeader: hunk.Header,
			content:    fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Header),
		})

		// Collect removed and added runs, pair them up
		var removed, added []types.DiffLine
		flushPairs := func() {
			maxLen := len(removed)
			if len(added) > maxLen {
				maxLen = len(added)
			}
			for i := 0; i < maxLen; i++ {
				sl := diffViewLine{
					isSplit:       true,
					mdInCodeBlock: inCodeBlock && isMd,
					mdCodeLang:    codeLang,
				}
				if i < len(removed) {
					sl.kind = types.DiffLineRemoved
					sl.oldLineNum = removed[i].OldLineNum
					sl.content = m.expandTabs(removed[i].Content)
					if isMd {
						if fence := codeFencePattern.FindStringSubmatch(removed[i].Content); fence != nil {
							sl.mdIsFence = true
							sl.mdInCodeBlock = false
						}
					}
				} else {
					sl.leftEmpty = true
					sl.kind = types.DiffLineContext
				}
				if i < len(added) {
					sl.rightKind = types.DiffLineAdded
					sl.rightLineNum = added[i].NewLineNum
					sl.rightContent = m.expandTabs(added[i].Content)
				} else {
					sl.rightEmpty = true
					sl.rightKind = types.DiffLineContext
				}
				m.lines = append(m.lines, sl)
			}

			// Update fence state from added lines (new file version)
			if isMd {
				for _, a := range added {
					if fence := codeFencePattern.FindStringSubmatch(a.Content); fence != nil {
						if !inCodeBlock {
							inCodeBlock = true
							codeLang = fence[1]
						} else {
							inCodeBlock = false
							codeLang = ""
						}
					}
				}
			}

			removed = removed[:0]
			added = added[:0]
		}

		for _, dl := range hunk.Lines {
			switch dl.Kind {
			case types.DiffLineRemoved:
				removed = append(removed, dl)
			case types.DiffLineAdded:
				added = append(added, dl)
			case types.DiffLineContext:
				flushPairs()

				// Track fence state from context lines
				isFence := false
				if isMd {
					if fence := codeFencePattern.FindStringSubmatch(dl.Content); fence != nil {
						isFence = true
						if !inCodeBlock {
							inCodeBlock = true
							codeLang = fence[1]
						} else {
							inCodeBlock = false
							codeLang = ""
						}
					}
				}

				expanded := m.expandTabs(dl.Content)
				m.lines = append(m.lines, diffViewLine{
					isSplit:       true,
					kind:          types.DiffLineContext,
					oldLineNum:    dl.OldLineNum,
					content:       expanded,
					rightKind:     types.DiffLineContext,
					rightLineNum:  dl.NewLineNum,
					rightContent:  expanded,
					mdInCodeBlock: inCodeBlock && isMd && !isFence,
					mdIsFence:     isFence,
					mdCodeLang:    codeLang,
				})
			}
		}
		flushPairs()

		// Insert inline comments after their target lines
		m.insertInlineComments(hunk)
	}
}

// pairLines pairs consecutive removed/added line runs for intra-line diff highlighting.
func (m *diffViewModel) pairLines() {
	i := 0
	for i < len(m.lines) {
		if m.lines[i].isHunk || m.lines[i].isComment {
			i++
			continue
		}

		// Find run of removed lines
		removeStart := i
		for i < len(m.lines) && m.lines[i].kind == types.DiffLineRemoved &&
			!m.lines[i].isHunk && !m.lines[i].isComment {
			i++
		}
		removeEnd := i

		// Find run of added lines immediately after
		addStart := i
		for i < len(m.lines) && m.lines[i].kind == types.DiffLineAdded &&
			!m.lines[i].isHunk && !m.lines[i].isComment {
			i++
		}
		addEnd := i

		// Pair them up
		removeCount := removeEnd - removeStart
		addCount := addEnd - addStart
		pairCount := removeCount
		if addCount < pairCount {
			pairCount = addCount
		}
		for j := 0; j < pairCount; j++ {
			m.lines[removeStart+j].pairContent = m.lines[addStart+j].content
			m.lines[addStart+j].pairContent = m.lines[removeStart+j].content
		}

		// If we didn't advance past any removed/added, skip forward
		if removeStart == removeEnd && addStart == addEnd {
			i++
		}
	}
}

func (m diffViewModel) renderHunkHeader(line diffViewLine, selected bool) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Faint(true)
	content := style.Render(line.content)
	if selected && m.focused {
		content = lipgloss.NewStyle().Reverse(true).Render(line.content)
	}
	return fmt.Sprintf("%-*s", m.width, content)
}

func (m diffViewModel) renderCommentLine(line diffViewLine, selected bool) string {
	// Pick color based on comment type
	var clr color.Color
	if line.comment != nil {
		switch line.comment.Type {
		case types.CommentIssue:
			clr = lipgloss.Color("1")
		case types.CommentSuggestion:
			clr = lipgloss.Color("3")
		case types.CommentNote:
			clr = lipgloss.Color("4")
		case types.CommentPraise:
			clr = lipgloss.Color("2")
		default:
			clr = lipgloss.Color("3")
		}
	} else {
		clr = lipgloss.Color("3")
	}

	// Use expanded format if this comment is expanded
	content := line.content
	expanded := line.comment != nil && line.comment.ID == m.expandedCommentID
	if expanded {
		content = formatExpandedComment(line.comment, m.width)
	}

	style := lipgloss.NewStyle().Foreground(clr)
	if line.comment != nil && line.comment.Resolved {
		style = style.Faint(true)
	}
	// Expanded comments use a thick border to indicate selection instead of reverse
	if selected && !expanded {
		style = style.Reverse(true)
	}

	// Render each sub-line individually to preserve multi-line box structure
	subLines := strings.Split(content, "\n")

	// Track suggestion fence state for syntax highlighting
	inCodeFence := false

	var b strings.Builder
	for i, sl := range subLines {
		if i > 0 {
			b.WriteString("\n")
		}

		// Detect suggestion fence boundaries in expanded comments.
		// Check if the content after the │ prefix starts with ``` to avoid
		// false positives from backticks inside code (e.g. strings.Contains(s, "```")).
		if expanded {
			if contentAfterPrefix := extractCommentContent(sl); strings.HasPrefix(contentAfterPrefix, "```") {
				inCodeFence = !inCodeFence
				b.WriteString(style.Render(fmt.Sprintf("%-*s", m.width, sl)))
				continue
			}
			if inCodeFence {
				b.WriteString(m.renderExpandedCodeLine(sl, style))
				continue
			}
		}

		b.WriteString(style.Render(fmt.Sprintf("%-*s", m.width, sl)))
	}
	return b.String()
}

// extractCommentContent returns the text after the "║ " (or "║ ✓ ") prefix in
// a formatted expanded comment line, or "" if no prefix is found.
func extractCommentContent(sl string) string {
	idx := strings.Index(sl, "║")
	if idx < 0 {
		return ""
	}
	rest := sl[idx+len("║"):]
	rest = strings.TrimLeft(rest, " ✓")
	if len(rest) > 0 && rest[0] == ' ' {
		rest = rest[1:]
	}
	return rest
}

// renderExpandedCodeLine renders a code line inside a suggestion fence with syntax
// highlighting. The prefix (║) keeps the comment color; the code gets highlighted.
func (m diffViewModel) renderExpandedCodeLine(sl string, commentStyle lipgloss.Style) string {
	// Lines look like: "  ║ code here" — find the code after the prefix
	prefixEnd := strings.Index(sl, "║")
	if prefixEnd < 0 {
		return commentStyle.Render(fmt.Sprintf("%-*s", m.width, sl))
	}
	// Find the space after ║ (or ║ ✓)
	rest := sl[prefixEnd:]
	codeStart := strings.IndexByte(rest, ' ')
	if codeStart < 0 {
		return commentStyle.Render(fmt.Sprintf("%-*s", m.width, sl))
	}
	codeStart++ // skip the space

	prefix := sl[:prefixEnd+codeStart]
	code := rest[codeStart:]

	// Render prefix with comment color, code with syntax highlighting
	styledPrefix := commentStyle.Render(prefix)
	codeWidth := m.width - ansi.StringWidth(prefix)
	if codeWidth < 1 {
		codeWidth = 1
	}
	highlightedCode := m.hl.highlightLine(m.path, code, nil, nil, nil, codeWidth)

	return styledPrefix + highlightedCode
}

func (m diffViewModel) renderContentLine(line diffViewLine, _, contentWidth int, selected, inVisual bool) string {
	gutterWidth := 4
	gutter := fmt.Sprintf("%-3d ", line.newLineNum)
	isMd := (m.contentMode || m.style == diffStyleFile) && isMarkdownContent(m.path)

	// Wrap mode
	if m.wrap {
		return m.renderWrappedLine(gutter, line.content, gutterWidth, contentWidth,
			nil, nil, selected || inVisual, &line)
	}

	// Scroll mode: apply horizontal offset, then clip
	content := line.content
	if m.hOffset > 0 {
		content, _ = applyHOffset(content, m.hOffset)
	}
	content = ansi.Truncate(content, contentWidth, "")

	if (selected || inVisual) && m.focused {
		padded := gutter + padToWidth(content, contentWidth)
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	// Render gutter
	gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	if len(gutter) < gutterWidth {
		gutter = fmt.Sprintf("%-*s", gutterWidth, gutter)
	}
	renderedGutter := gutterStyle.Render(gutter)

	// Render content: markdown styling or syntax highlighting
	var renderedContent string
	if isMd && line.mdIsFence {
		// Code fence markers → render as horizontal rule
		renderedContent = m.mdStyler.theme.MarkdownRule.Render(strings.Repeat("─", min(40, contentWidth)))
		renderedContent = padToWidth(renderedContent, contentWidth)
	} else if isMd && line.mdInCodeBlock && line.mdCodeLang != "" {
		// Code block with language → use Chroma syntax highlighting
		fakePath := "code." + line.mdCodeLang
		renderedContent = m.hl.highlightLine(fakePath, content, nil, nil, nil, contentWidth)
	} else if isMd && line.mdInCodeBlock {
		// Code block without language → code block style
		renderedContent = m.mdStyler.theme.MarkdownCodeBlock.Render(content)
		renderedContent = padToWidth(renderedContent, contentWidth)
	} else if isMd {
		// Regular markdown line
		renderedContent = m.mdStyler.StyleLine(content)
		renderedContent = padToWidth(renderedContent, contentWidth)
	} else {
		renderedContent = m.hl.highlightLine(m.path, content, nil, nil, nil, contentWidth)
	}

	return renderedGutter + renderedContent
}

func (m diffViewModel) renderDiffLine(line diffViewLine, _, contentWidth int, selected, inVisual bool) string {
	gutterWidth := 10

	// Gutter
	var gutter string
	switch line.kind {
	case types.DiffLineContext:
		gutter = fmt.Sprintf("%4d %4d ", line.oldLineNum, line.newLineNum)
	case types.DiffLineAdded:
		gutter = fmt.Sprintf("     %4d ", line.newLineNum)
	case types.DiffLineRemoved:
		gutter = fmt.Sprintf("%4d      ", line.oldLineNum)
	}

	// Determine backgrounds
	var lineBg, changeBg color.Color
	switch line.kind {
	case types.DiffLineAdded:
		lineBg = m.theme.AddedBg
		changeBg = m.theme.AddedChangeBg
	case types.DiffLineRemoved:
		lineBg = m.theme.RemovedBg
		changeBg = m.theme.RemovedChangeBg
	}

	// Wrap mode: render line as multiple screen lines
	if m.wrap {
		return m.renderWrappedLine(gutter, line.content, gutterWidth, contentWidth,
			lineBg, changeBg, selected || inVisual, &line)
	}

	// Scroll mode: apply horizontal offset, then clip
	content := line.content
	if m.hOffset > 0 {
		content, _ = applyHOffset(content, m.hOffset)
	}
	content = ansi.Truncate(content, contentWidth, "")

	// Selected: reverse the full plain line
	if (selected || inVisual) && m.focused {
		padded := gutter + padToWidth(content, contentWidth)
		return lipgloss.NewStyle().Reverse(true).Render(padded)
	}

	// Render gutter
	gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	if lineBg != nil {
		gutterStyle = gutterStyle.Background(lineBg)
	}
	if len(gutter) < gutterWidth {
		gutter = fmt.Sprintf("%-*s", gutterWidth, gutter)
	}
	renderedGutter := gutterStyle.Render(gutter)

	// Render content: markdown styling or syntax highlighting
	isMd := isMarkdownFile(m.path)
	var renderedContent string

	if isMd && line.mdIsFence {
		// Code fence markers → horizontal rule with diff background
		rule := m.mdStyler.theme.MarkdownRule.Render(strings.Repeat("─", min(40, contentWidth)))
		renderedContent = applyBgAndPad(rule, lineBg, contentWidth)
	} else if isMd && line.mdInCodeBlock && line.mdCodeLang != "" {
		// Code block with language → Chroma syntax highlighting
		fakePath := "code." + line.mdCodeLang
		renderedContent = m.hl.highlightLine(fakePath, content, lineBg, changeBg, nil, contentWidth)
	} else if isMd && line.mdInCodeBlock {
		// Code block without language → code block style with diff background
		styled := m.mdStyler.theme.MarkdownCodeBlock.Render(content)
		renderedContent = applyBgAndPad(styled, lineBg, contentWidth)
	} else if isMd {
		// Regular markdown line with diff background
		styled := m.mdStyler.StyleLine(content)
		renderedContent = applyBgAndPad(styled, lineBg, contentWidth)
	} else {
		// Non-markdown → syntax highlighting with intra-line changes
		var changes []changeRange
		if line.pairContent != "" {
			if line.kind == types.DiffLineRemoved {
				changes, _ = computeChangeRanges(line.content, line.pairContent)
			} else if line.kind == types.DiffLineAdded {
				_, changes = computeChangeRanges(line.pairContent, line.content)
			}
			if m.hOffset > 0 {
				changes = shiftChangeRanges(changes, m.hOffset)
			}
			changes = clipChangeRanges(changes, contentWidth)
		}
		renderedContent = m.hl.highlightLine(m.path, content, lineBg, changeBg, changes, contentWidth)
	}

	return renderedGutter + renderedContent
}

func (m diffViewModel) renderSplitLine(line diffViewLine, selected, inVisual bool) string {
	halfW := (m.width - 1) / 2 // subtract divider, then halve
	gutterW := 5               // "NNNN "
	contentW := halfW - gutterW
	if contentW < 1 {
		contentW = 1
	}

	// Prepare left side raw content
	var leftGutter, leftRawContent string
	leftTruncatedAt := -1
	if line.leftEmpty {
		leftGutter = strings.Repeat(" ", gutterW)
		leftRawContent = ""
	} else {
		if line.oldLineNum > 0 {
			leftGutter = fmt.Sprintf("%4d ", line.oldLineNum)
		} else {
			leftGutter = strings.Repeat(" ", gutterW)
		}
		leftRawContent = line.content
		if m.hOffset > 0 {
			leftRawContent, _ = applyHOffset(leftRawContent, m.hOffset)
		}
		if ansi.StringWidth(leftRawContent) > contentW {
			leftTruncatedAt = contentW
			leftRawContent = ansi.Truncate(leftRawContent, contentW, "")
		}
	}

	// Prepare right side raw content
	var rightGutter, rightRawContent string
	rightTruncatedAt := -1
	if line.rightEmpty {
		rightGutter = strings.Repeat(" ", gutterW)
		rightRawContent = ""
	} else {
		if line.rightLineNum > 0 {
			rightGutter = fmt.Sprintf("%4d ", line.rightLineNum)
		} else {
			rightGutter = strings.Repeat(" ", gutterW)
		}
		rightRawContent = line.rightContent
		if m.hOffset > 0 {
			rightRawContent, _ = applyHOffset(rightRawContent, m.hOffset)
		}
		if ansi.StringWidth(rightRawContent) > contentW {
			rightTruncatedAt = contentW
			rightRawContent = ansi.Truncate(rightRawContent, contentW, "")
		}
	}

	divider := "│"

	// Selected: reverse the full plain line
	if (selected || inVisual) && m.focused {
		leftFull := leftGutter + padToWidth(leftRawContent, contentW)
		rightFull := rightGutter + padToWidth(rightRawContent, contentW)
		return lipgloss.NewStyle().Reverse(true).Render(leftFull + divider + rightFull)
	}

	// Compute intra-line change ranges for paired sides
	var leftChanges, rightChanges []changeRange
	if !line.leftEmpty && !line.rightEmpty &&
		line.kind == types.DiffLineRemoved && line.rightKind == types.DiffLineAdded {
		leftChanges, rightChanges = computeChangeRanges(line.content, line.rightContent)
		if m.hOffset > 0 {
			leftChanges = shiftChangeRanges(leftChanges, m.hOffset)
			rightChanges = shiftChangeRanges(rightChanges, m.hOffset)
		}
		if leftTruncatedAt >= 0 {
			leftChanges = clipChangeRanges(leftChanges, leftTruncatedAt)
		}
		if rightTruncatedAt >= 0 {
			rightChanges = clipChangeRanges(rightChanges, rightTruncatedAt)
		}
	}

	// Render each side
	leftStyled := m.renderSplitSide(leftGutter, leftRawContent, line.kind, line.leftEmpty, leftChanges, gutterW, contentW, line)
	rightStyled := m.renderSplitSide(rightGutter, rightRawContent, line.rightKind, line.rightEmpty, rightChanges, gutterW, contentW, line)
	divStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(divider)

	return leftStyled + divStyled + rightStyled
}

func (m diffViewModel) renderSplitSide(gutter, content string, kind types.DiffLineKind, empty bool, changes []changeRange, gutterW, contentW int, line diffViewLine) string {
	if empty {
		full := strings.Repeat(" ", gutterW) + strings.Repeat(" ", contentW)
		return lipgloss.NewStyle().Faint(true).Render(full)
	}

	// Determine backgrounds
	var lineBg, changeBg color.Color
	switch kind {
	case types.DiffLineAdded:
		lineBg = m.theme.AddedBg
		changeBg = m.theme.AddedChangeBg
	case types.DiffLineRemoved:
		lineBg = m.theme.RemovedBg
		changeBg = m.theme.RemovedChangeBg
	}

	// Render gutter
	gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	if lineBg != nil {
		gutterStyle = gutterStyle.Background(lineBg)
	}
	if len(gutter) < gutterW {
		gutter = fmt.Sprintf("%-*s", gutterW, gutter)
	}
	renderedGutter := gutterStyle.Render(gutter)

	// Render content: markdown styling or syntax highlighting
	isMd := isMarkdownFile(m.path)
	var renderedContent string

	if isMd && line.mdIsFence {
		rule := m.mdStyler.theme.MarkdownRule.Render(strings.Repeat("─", min(40, contentW)))
		renderedContent = applyBgAndPad(rule, lineBg, contentW)
	} else if isMd && line.mdInCodeBlock && line.mdCodeLang != "" {
		fakePath := "code." + line.mdCodeLang
		renderedContent = m.hl.highlightLine(fakePath, content, lineBg, changeBg, nil, contentW)
	} else if isMd && line.mdInCodeBlock {
		styled := m.mdStyler.theme.MarkdownCodeBlock.Render(content)
		renderedContent = applyBgAndPad(styled, lineBg, contentW)
	} else if isMd {
		styled := m.mdStyler.StyleLine(content)
		renderedContent = applyBgAndPad(styled, lineBg, contentW)
	} else {
		renderedContent = m.hl.highlightLine(m.path, content, lineBg, changeBg, changes, contentW)
	}

	return renderedGutter + renderedContent
}

// padToWidth pads a string with spaces to reach the target visual width,
// using lipgloss.Width for correct measurement of multi-byte characters.
func padToWidth(s string, width int) string {
	visWidth := lipgloss.Width(s)
	if visWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visWidth)
}

// renderWrappedLine renders a single logical line wrapped across multiple screen lines.
// Used by both renderDiffLine and renderContentLine in wrap mode.
// When mdLine is non-nil and the path indicates markdown (via isMarkdownFile for diffs,
// or isMarkdownContent for content mode), markdown styling is used instead of syntax highlighting.
func (m diffViewModel) renderWrappedLine(gutter, content string, gutterWidth, contentWidth int,
	lineBg, changeBg color.Color, highlight bool, mdLine *diffViewLine) string {

	chunks := wrapContent(content, contentWidth)
	blankGutter := strings.Repeat(" ", gutterWidth)
	isMd := mdLine != nil && (isMarkdownFile(m.path) || (m.contentMode && isMarkdownContent(m.path)))

	var parts []string
	for ci, chunk := range chunks {
		chunkGutter := gutter
		if ci > 0 {
			chunkGutter = blankGutter
		}

		if highlight && m.focused {
			full := chunkGutter + fmt.Sprintf("%-*s", contentWidth, chunk)
			parts = append(parts, lipgloss.NewStyle().Reverse(true).Render(full))
			continue
		}

		// Render gutter
		gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		if lineBg != nil {
			gutterStyle = gutterStyle.Background(lineBg)
		}
		if len(chunkGutter) < gutterWidth {
			chunkGutter = fmt.Sprintf("%-*s", gutterWidth, chunkGutter)
		}
		renderedGutter := gutterStyle.Render(chunkGutter)

		// Render content: markdown styling or syntax highlighting
		var renderedContent string
		if isMd && mdLine.mdIsFence {
			rule := m.mdStyler.theme.MarkdownRule.Render(strings.Repeat("─", min(40, contentWidth)))
			renderedContent = applyBgAndPad(rule, lineBg, contentWidth)
		} else if isMd && mdLine.mdInCodeBlock && mdLine.mdCodeLang != "" {
			fakePath := "code." + mdLine.mdCodeLang
			renderedContent = m.hl.highlightLine(fakePath, chunk, lineBg, changeBg, nil, contentWidth)
		} else if isMd && mdLine.mdInCodeBlock {
			styled := m.mdStyler.theme.MarkdownCodeBlock.Render(chunk)
			renderedContent = applyBgAndPad(styled, lineBg, contentWidth)
		} else if isMd {
			styled := m.mdStyler.StyleLine(chunk)
			renderedContent = applyBgAndPad(styled, lineBg, contentWidth)
		} else {
			renderedContent = m.hl.highlightLine(m.path, chunk, lineBg, changeBg, nil, contentWidth)
		}

		parts = append(parts, renderedGutter+renderedContent)
	}
	return strings.Join(parts, "\n")
}

// ScrollRight scrolls the diff content right by a tab stop.
func (m *diffViewModel) ScrollRight() {
	if m.wrap {
		return
	}
	m.hOffset += 4
}

// ScrollLeft scrolls the diff content left by a tab stop.
func (m *diffViewModel) ScrollLeft() {
	if m.wrap {
		return
	}
	m.hOffset -= 4
	if m.hOffset < 0 {
		m.hOffset = 0
	}
}

// ResetHScroll resets the horizontal scroll offset to 0 (vim `0`).
func (m *diffViewModel) ResetHScroll() {
	m.hOffset = 0
}

// ScrollToFirstChar scrolls to the first non-whitespace column (vim `^`).
// Finds the minimum leading whitespace across all visible content lines.
func (m *diffViewModel) ScrollToFirstChar() {
	if m.wrap || len(m.lines) == 0 {
		return
	}
	minIndent := -1
	for _, line := range m.lines {
		if line.isHunk || line.isComment || line.content == "" {
			continue
		}
		indent := 0
		for _, r := range line.content {
			if r == ' ' || r == '\t' {
				if r == '\t' {
					indent += m.tabSize
				} else {
					indent++
				}
			} else {
				break
			}
		}
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
		if minIndent == 0 {
			break
		}
	}
	if minIndent > 0 {
		m.hOffset = minIndent
	} else {
		m.hOffset = 0
	}
}

// ScrollToEnd scrolls horizontally to the longest visible line.
func (m *diffViewModel) ScrollToEnd() {
	if m.wrap || len(m.lines) == 0 {
		return
	}
	maxLen := 0
	for _, line := range m.lines {
		if n := len([]rune(line.content)); n > maxLen {
			maxLen = n
		}
	}
	cw := m.contentWidthFor(m.lines[0])
	if maxLen > cw {
		m.hOffset = maxLen - cw
	}
}

// ToggleWrap toggles line wrapping and resets horizontal scroll when enabling.
func (m *diffViewModel) ToggleWrap() {
	m.wrap = !m.wrap
	if m.wrap {
		m.hOffset = 0
	}
	m.ensureVisible()
}

// CycleDiffStyle cycles through display styles.
// For file diffs: unified → split → file → unified.
// For content items with a previous version: content → unified → split → content.
func (m *diffViewModel) CycleDiffStyle() tea.Cmd {
	if m.additionalFilePath != "" {
		return nil
	}

	// Content mode: toggle into diff view if a previous version exists
	if m.contentMode && m.contentID != "" {
		if !m.contentHasDiff {
			return nil // no previous version, nothing to diff
		}
		contentID := m.contentID
		style := m.preferredContentDiffStyle
		return func() tea.Msg {
			return requestContentDiffMsg{contentID: contentID, preferredStyle: style}
		}
	}

	// Viewing a content diff: cycle unified → split → back to content
	if !m.contentMode && m.contentID != "" {
		switch m.style {
		case diffStyleUnified:
			m.style = diffStyleSplit
			m.buildLines()
			return nil
		default:
			// Back to content view
			m.contentMode = true
			m.style = diffStyleUnified
			m.hunks = nil
			m.buildContentLines(m.contentDiffContent)
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.hOffset = 0
			return nil
		}
	}

	// Regular file diffs
	switch m.style {
	case diffStyleUnified:
		m.style = diffStyleSplit
		m.buildLines()
	case diffStyleSplit:
		path := m.path
		return func() tea.Msg { return requestFileContentMsg{path: path} }
	case diffStyleFile:
		m.style = diffStyleUnified
		m.buildLines()
	}
	return nil
}

// contentWidthFor returns the available content width (excluding gutter) for a line.
func (m diffViewModel) contentWidthFor(line diffViewLine) int {
	if line.isSplit {
		return (m.width-1)/2 - 5 // subtract divider, then halve, minus gutter
	}
	if m.contentMode {
		return m.width - 4 // gutterWidth=4
	}
	return m.width - 10 // gutterWidth=10
}

// screenLinesFor returns how many screen lines a logical line occupies.
// In non-wrap mode or for split/hunk/comment lines, this is always 1.
func (m diffViewModel) screenLinesFor(idx int) int {
	if idx < 0 || idx >= len(m.lines) {
		return 1
	}
	line := m.lines[idx]
	// Expanded comments take multiple screen lines
	if line.isComment && line.comment != nil && line.comment.ID == m.expandedCommentID {
		return strings.Count(formatExpandedComment(line.comment, m.width), "\n") + 1
	}
	if !m.wrap {
		return 1
	}
	if line.isHunk || line.isComment || line.isSplit {
		return 1
	}
	cw := m.contentWidthFor(line)
	if cw <= 0 {
		return 1
	}
	return len(wrapContent(line.content, cw))
}

// applyHOffset slices content at the horizontal offset (visual-width-aware).
// Returns the sliced content and whether there is hidden content to the left.
func applyHOffset(content string, hOffset int) (string, bool) {
	if hOffset <= 0 {
		return content, false
	}
	if ansi.StringWidth(content) <= hOffset {
		return "", true
	}
	return ansi.TruncateLeft(content, hOffset, ""), true
}

// shiftChangeRanges adjusts byte-offset change ranges by a rune offset.
// This is approximate since rune offset != byte offset for multi-byte chars,
// but works correctly for ASCII content (the common case for code).
func shiftChangeRanges(changes []changeRange, runeOffset int) []changeRange {
	if runeOffset <= 0 || len(changes) == 0 {
		return changes
	}
	var result []changeRange
	for _, cr := range changes {
		shifted := changeRange{start: cr.start - runeOffset, end: cr.end - runeOffset}
		if shifted.end <= 0 {
			continue
		}
		if shifted.start < 0 {
			shifted.start = 0
		}
		result = append(result, shifted)
	}
	return result
}

// expandTabs replaces tab characters with spaces for consistent width calculation.
// Tabs are 1 rune but render as multiple visual columns in the terminal, which
// breaks rune-based width truncation in the diff view.
func (m *diffViewModel) expandTabs(s string) string {
	tabSize := m.tabSize
	if tabSize <= 0 {
		tabSize = 4
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabSize))
}

// wrapContent splits content into lines that fit within width, preferring
// to break at word boundaries (spaces). Falls back to character-based
// wrapping when a single word exceeds the available width.
func wrapContent(content string, width int) []string {
	if width <= 0 {
		return []string{content}
	}
	runes := []rune(content)
	if len(runes) <= width {
		return []string{content}
	}

	var chunks []string
	lineStart := 0
	lastSpace := -1 // index of last space seen on the current line

	for i := 0; i < len(runes); i++ {
		if runes[i] == ' ' {
			lastSpace = i
		}

		lineLen := i - lineStart + 1
		if lineLen > width {
			if lastSpace > lineStart {
				// Break after the last space (space stays at end of current line)
				chunks = append(chunks, string(runes[lineStart:lastSpace+1]))
				lineStart = lastSpace + 1
				lastSpace = -1
			} else {
				// No space on this line — force break at width (character fallback)
				chunks = append(chunks, string(runes[lineStart:lineStart+width]))
				lineStart = lineStart + width
				lastSpace = -1
				i = lineStart - 1 // will be incremented by loop
			}
		}
	}

	// Emit remaining content
	if lineStart < len(runes) {
		chunks = append(chunks, string(runes[lineStart:]))
	}

	return chunks
}

// ScrollDown scrolls the diff viewport down by one line.
func (m *diffViewModel) ScrollDown() {
	// In wrap mode, compute max offset accounting for wrapped lines
	if m.wrap {
		// Check if there's content below to scroll to
		screenLines := 0
		for i := m.offset; i < len(m.lines); i++ {
			screenLines += m.screenLinesFor(i)
			if screenLines > m.height {
				m.offset++
				return
			}
		}
		return
	}
	maxOffset := len(m.lines) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset < maxOffset {
		m.offset++
	}
}

// ScrollUp scrolls the diff viewport up by one line.
func (m *diffViewModel) ScrollUp() {
	if m.offset > 0 {
		m.offset--
	}
}

// ScrollDownHalfPage scrolls the diff viewport down by half a page.
func (m *diffViewModel) ScrollDownHalfPage() {
	jump := m.height / 2
	if jump < 1 {
		jump = 1
	}
	if m.wrap {
		for i := 0; i < jump; i++ {
			screenLines := 0
			canScroll := false
			for j := m.offset; j < len(m.lines); j++ {
				screenLines += m.screenLinesFor(j)
				if screenLines > m.height {
					m.offset++
					canScroll = true
					break
				}
			}
			if !canScroll {
				break
			}
		}
		return
	}
	maxOffset := len(m.lines) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.offset += jump
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

// ScrollUpHalfPage scrolls the diff viewport up by half a page.
func (m *diffViewModel) ScrollUpHalfPage() {
	jump := m.height / 2
	if jump < 1 {
		jump = 1
	}
	m.offset -= jump
	if m.offset < 0 {
		m.offset = 0
	}
}

// isCursorOffScreen returns true if the cursor is outside the visible viewport.
func (m diffViewModel) isCursorOffScreen() bool {
	if m.cursor < m.offset {
		return true
	}
	if !m.wrap {
		return m.cursor >= m.offset+m.height
	}
	// Wrap mode: count screen lines from offset to cursor
	screenLines := 0
	for i := m.offset; i <= m.cursor && i < len(m.lines); i++ {
		screenLines += m.screenLinesFor(i)
		if screenLines > m.height {
			return true
		}
	}
	return false
}

// lastVisibleLine returns the index of the last line visible in the viewport.
func (m diffViewModel) lastVisibleLine() int {
	if !m.wrap {
		last := m.offset + m.height - 1
		if last >= len(m.lines) {
			last = len(m.lines) - 1
		}
		return last
	}
	// Wrap mode: walk from offset, summing screen lines
	screenLines := 0
	last := m.offset
	for i := m.offset; i < len(m.lines); i++ {
		sl := m.screenLinesFor(i)
		if screenLines+sl > m.height {
			break
		}
		screenLines += sl
		last = i
	}
	return last
}

func (m *diffViewModel) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if !m.wrap {
		if m.cursor >= m.offset+m.height {
			m.offset = m.cursor - m.height + 1
		}
		return
	}
	// Wrap mode: count screen lines from offset to cursor
	screenLines := 0
	for i := m.offset; i <= m.cursor && i < len(m.lines); i++ {
		screenLines += m.screenLinesFor(i)
	}
	for screenLines > m.height && m.offset < m.cursor {
		screenLines -= m.screenLinesFor(m.offset)
		m.offset++
	}
}

// CursorComment returns the comment under the cursor, or nil if the cursor is not on a comment line.
func (m diffViewModel) CursorComment() *types.ReviewComment {
	if m.cursor >= 0 && m.cursor < len(m.lines) && m.lines[m.cursor].isComment {
		return m.lines[m.cursor].comment
	}
	return nil
}

// isSelectable returns true if the line at idx can receive cursor focus.
// Hunk headers are skipped; comments and diff content lines are selectable.
func (m diffViewModel) isSelectable(idx int) bool {
	if idx < 0 || idx >= len(m.lines) {
		return false
	}
	line := m.lines[idx]
	if line.isHunk {
		return false
	}
	if line.isComment {
		return true
	}
	// Skip removed lines — they have no new-file line number and can't be commented on
	if line.kind == types.DiffLineRemoved && line.newLineNum == 0 {
		return false
	}
	return true
}

// nextSelectable moves from current position by dir (+1 or -1), skipping non-selectable lines.
func (m diffViewModel) nextSelectable(from, dir int) int {
	next := from + dir
	for next >= 0 && next < len(m.lines) && !m.isSelectable(next) {
		next += dir
	}
	if next < 0 || next >= len(m.lines) {
		return from // stay put if nothing selectable in that direction
	}
	return next
}

// jumpToNextComment moves the cursor to the next comment line, wrapping to the first comment at the end.
func (m *diffViewModel) jumpToNextComment() {
	for i := m.cursor + 1; i < len(m.lines); i++ {
		if m.lines[i].isComment {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
	// Wrap: search from the beginning up to cursor.
	for i := 0; i <= m.cursor; i++ {
		if m.lines[i].isComment {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

// jumpToPrevComment moves the cursor to the previous comment line, wrapping to the last comment at the top.
func (m *diffViewModel) jumpToPrevComment() {
	for i := m.cursor - 1; i >= 0; i-- {
		if m.lines[i].isComment {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
	// Wrap: search from the end down to cursor.
	for i := len(m.lines) - 1; i >= m.cursor; i-- {
		if m.lines[i].isComment {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

// nearestSelectable finds the closest selectable line from pos, preferring the given direction.
func (m diffViewModel) nearestSelectable(pos, dir int) int {
	if pos < 0 {
		pos = 0
	}
	if pos >= len(m.lines) {
		pos = len(m.lines) - 1
	}
	if m.isSelectable(pos) {
		return pos
	}
	return m.nextSelectable(pos, dir)
}

func (m diffViewModel) visualRange() (int, int) {
	start := m.visualStart
	end := m.cursor
	if start > end {
		start, end = end, start
	}
	// Map to line numbers
	startLine := m.lineNumAt(start)
	endLine := m.lineNumAt(end)
	if startLine == 0 {
		startLine = endLine
	}
	if endLine == 0 {
		endLine = startLine
	}
	return startLine, endLine
}

func (m diffViewModel) inVisualRange(idx int) bool {
	if !m.visualMode {
		return false
	}
	start := m.visualStart
	end := m.cursor
	if start > end {
		start, end = end, start
	}
	return idx >= start && idx <= end
}

func (m diffViewModel) lineNumAt(idx int) int {
	if idx < 0 || idx >= len(m.lines) {
		return 0
	}
	line := m.lines[idx]
	// Only return new-file line numbers — comments reference lines that
	// exist in the current working tree so the agent can act on them.
	return line.newLineNum
}

func (m diffViewModel) currentDiffLine() int {
	return m.lineNumAt(m.cursor)
}

// screenLineToIndex maps a screen-relative Y coordinate to a logical lines[] index.
// Walks from offset counting the actual display lines each logical line occupies,
// including multi-line comment rendering (3 lines per comment).
// Returns -1 if the coordinate is out of bounds.
func (m diffViewModel) screenLineToIndex(screenY int) int {
	if screenY < 0 || len(m.lines) == 0 {
		return -1
	}

	screenLine := 0
	for i := m.offset; i < len(m.lines); i++ {
		sl := m.displayLinesFor(i)
		if screenY < screenLine+sl {
			return i
		}
		screenLine += sl
		if screenLine > m.height {
			break
		}
	}
	return -1
}

// displayLinesFor returns the actual number of terminal lines a logical line
// occupies in the rendered output. It renders the line using the same logic as
// View() and counts newlines, guaranteeing accuracy for comments, wrapped lines,
// and any other multi-line rendering. Only called during mouse event processing,
// not every frame.
func (m diffViewModel) displayLinesFor(idx int) int {
	if idx < 0 || idx >= len(m.lines) {
		return 1
	}
	line := m.lines[idx]

	var rendered string
	if line.isHunk {
		rendered = m.renderHunkHeader(line, false)
	} else if line.isComment {
		rendered = m.renderCommentLine(line, false)
	} else if line.isSplit {
		rendered = m.renderSplitLine(line, false, false)
	} else if m.style == diffStyleFile || m.contentMode {
		gutterWidth := 4
		contentWidth := m.width - gutterWidth
		rendered = m.renderContentLine(line, gutterWidth, contentWidth, false, false)
	} else {
		gutterWidth := 10
		contentWidth := m.width - gutterWidth
		rendered = m.renderDiffLine(line, gutterWidth, contentWidth, false, false)
	}

	return strings.Count(rendered, "\n") + 1
}

// handleMouseClick positions the cursor at the clicked screen line and starts
// drag tracking for visual selection.
func (m *diffViewModel) handleMouseClick(relY int) {
	idx := m.screenLineToIndex(relY)
	if idx < 0 {
		return
	}
	idx = m.nearestSelectable(idx, 1)
	m.cursor = idx
	m.visualMode = true
	m.visualStart = m.cursor
	m.mouseDragActive = true
	m.ensureVisible()
}

// handleMouseMotion extends the visual selection to the line under the cursor
// during a mouse drag.
func (m *diffViewModel) handleMouseMotion(relY int) {
	if !m.mouseDragActive {
		return
	}
	idx := m.screenLineToIndex(relY)
	if idx < 0 {
		return
	}
	idx = m.nearestSelectable(idx, 1)
	m.cursor = idx
	m.ensureVisible()
}

// handleMouseRelease ends drag tracking. If the click didn't produce a range
// (start == end), visual mode is cancelled — it was just a click, not a drag.
func (m *diffViewModel) handleMouseRelease() {
	m.mouseDragActive = false
	if m.visualStart == m.cursor {
		m.visualMode = false
	}
}

type openCommentMsg struct {
	path        string
	lineStart   int
	lineEnd     int
	targetType  types.TargetType
	prefillBody string           // pre-filled body text (for suggestions)
	prefillType types.CommentType // pre-set comment type (zero value = default)
}

func openCommentCmd(path string, start, end int, targetType types.TargetType) tea.Cmd {
	return func() tea.Msg {
		return openCommentMsg{path: path, lineStart: start, lineEnd: end, targetType: targetType}
	}
}

func openFileCommentCmd(path string, targetType types.TargetType) tea.Cmd {
	return func() tea.Msg {
		return openCommentMsg{path: path, lineStart: 0, lineEnd: 0, targetType: targetType}
	}
}

func openSuggestCmd(path string, start, end int, targetType types.TargetType, codeContent string) tea.Cmd {
	body := "```suggestion\n" + codeContent + "\n```"
	return func() tea.Msg {
		return openCommentMsg{
			path:        path,
			lineStart:   start,
			lineEnd:     end,
			targetType:  targetType,
			prefillBody: body,
			prefillType: types.CommentSuggestion,
		}
	}
}

// selectedContent returns the raw text content of the new-file lines
// in the range [idxStart, idxEnd] (indices into m.lines).
// Skips hunk headers, comment lines, and removed lines.
func (m diffViewModel) selectedContent(idxStart, idxEnd int) string {
	if idxStart > idxEnd {
		idxStart, idxEnd = idxEnd, idxStart
	}
	var lines []string
	for i := idxStart; i <= idxEnd && i < len(m.lines); i++ {
		line := m.lines[i]
		if line.isHunk || line.isComment {
			continue
		}
		if line.kind == types.DiffLineRemoved {
			continue
		}
		content := line.content
		if line.isSplit {
			content = line.rightContent
		}
		lines = append(lines, content)
	}
	return strings.Join(lines, "\n")
}

// orderedVisualIndices returns visual selection indices in order (low, high).
func (m diffViewModel) orderedVisualIndices() (int, int) {
	start := m.visualStart
	end := m.cursor
	if start > end {
		start, end = end, start
	}
	return start, end
}

// insertInlineComments inserts comment lines after the diff line they target.
// It walks the existing lines (from the current hunk) in reverse-insertion order.
func (m *diffViewModel) insertInlineComments(hunk types.DiffHunk) {
	// Collect comments for this hunk
	var hunkComments []*types.ReviewComment
	for i := range m.comments {
		c := &m.comments[i]
		if c.TargetRef == m.path && c.LineStart >= hunk.NewStart && c.LineStart <= hunk.NewStart+hunk.NewCount {
			hunkComments = append(hunkComments, c)
		}
	}
	if len(hunkComments) == 0 {
		return
	}

	// Walk lines and insert comments after matching lines
	var result []diffViewLine
	for _, line := range m.lines {
		result = append(result, line)

		// Match on new-file line number (rightLineNum in split mode)
		lineNum := line.rightLineNum
		if lineNum == 0 {
			lineNum = line.newLineNum
		}
		if lineNum == 0 {
			continue
		}

		for _, c := range hunkComments {
			anchor := c.LineEnd
			if anchor == 0 {
				anchor = c.LineStart
			}
			if anchor == lineNum {
				result = append(result, diffViewLine{
					isComment: true,
					comment:   c,
					content:   formatInlineComment(c),
				})
			}
		}
	}
	m.lines = result
}

func formatInlineComment(c *types.ReviewComment) string {
	typeLabel := strings.ToUpper(string(c.Type))
	hasSuggestionBlock := strings.Contains(c.Body, "```suggestion")
	if hasSuggestionBlock {
		typeLabel = "✏ " + typeLabel
	}
	prefix := "│"
	if c.Resolved {
		prefix = "│ ✓"
		typeLabel = "✓ " + typeLabel
	}
	body := c.Body
	if hasSuggestionBlock {
		body = "(suggested edit)"
	} else if len(body) > 60 {
		body = body[:57] + "..."
	}
	return fmt.Sprintf("  ┌─── %s %s", typeLabel, strings.Repeat("─", 20)) + "\n" +
		fmt.Sprintf("  %s %s", prefix, body) + "\n" +
		fmt.Sprintf("  └───%s", strings.Repeat("─", 25))
}

// formatExpandedComment renders a comment's full body word-wrapped to the given width.
// Uses double-line box-drawing characters (╔═║╚) to visually distinguish from collapsed comments.
func formatExpandedComment(c *types.ReviewComment, width int) string {
	typeLabel := strings.ToUpper(string(c.Type))
	hasSuggestionBlock := strings.Contains(c.Body, "```suggestion")
	if hasSuggestionBlock {
		typeLabel = "✏ " + typeLabel
	}
	prefix := "║"
	if c.Resolved {
		prefix = "║ ✓"
		typeLabel = "✓ " + typeLabel
	}

	// Box occupies full width minus the 2-char indent
	boxWidth := width - 2
	if boxWidth < 20 {
		boxWidth = 20
	}
	// Body width: box minus "  " indent, prefix, and a space
	bodyWidth := boxWidth - ansi.StringWidth(prefix) - 2
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	headerDashes := boxWidth - ansi.StringWidth(typeLabel) - 6
	if headerDashes < 0 {
		headerDashes = 0
	}
	footerDashes := boxWidth - 4
	if footerDashes < 0 {
		footerDashes = 0
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("  ╔═══ %s %s", typeLabel, strings.Repeat("═", headerDashes)))

	for _, paragraph := range strings.Split(c.Body, "\n") {
		if paragraph == "" {
			lines = append(lines, fmt.Sprintf("  %s", prefix))
			continue
		}
		wrapped := wrapContent(paragraph, bodyWidth)
		for _, w := range wrapped {
			lines = append(lines, fmt.Sprintf("  %s %s", prefix, w))
		}
	}

	lines = append(lines, fmt.Sprintf("  ╚═══%s", strings.Repeat("═", footerDashes)))
	return strings.Join(lines, "\n")
}

