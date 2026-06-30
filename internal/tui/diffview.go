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

// commentFilterMode controls how source-code comment-only lines are shown. The
// `#` key cycles through the three states.
type commentFilterMode int

const (
	commentsShown  commentFilterMode = iota // normal syntax highlighting
	commentsDimmed                          // comment-only lines rendered faint
	commentsHidden                          // comment-only lines removed from the view
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

	// Agent annotation rendering.
	isAnnotation bool
	annotation   *types.Annotation
	annotated    bool // this code line falls within an annotation's range (draws a gutter bar)

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
	path          string
	hunks         []types.DiffHunk
	comments      []types.ReviewComment
	annotations   []types.Annotation // agent-authored, for the current file
	hideOverlays  bool               // when true, comments + annotations are not inserted
	commentFilter commentFilterMode  // show / dim / hide source-code comment-only lines
	commentLines  map[int]bool       // new-file line numbers that are comment-only (when filter != shown)
	lines         []diffViewLine
	cursor        int
	offset        int // scroll offset
	width         int
	height        int
	focused       bool
	style         diffStyle
	theme         *Theme
	hl            *highlighter
	isBinary      bool // true when hunk content contains binary control characters

	hOffset  int  // horizontal scroll offset (runes)
	wrap     bool // soft-wrap long lines
	fullFile bool // show whole file with diff coloring instead of compact hunks
	tabSize  int  // spaces per tab character

	// Visual mode
	visualMode  bool
	visualStart int

	// Search
	searchQuery    string // active query; matched substrings are highlighted while set
	searchMatches  []int  // indices into m.lines that contain the query
	searchIndex    int    // position within searchMatches of the current match
	searchBackward bool   // direction of the committed search (affects n/N)

	// Mouse drag state
	mouseDragActive bool

	// Comment expansion on hover
	expandedCommentID  string        // ID of the currently expanded comment (empty = none)
	expandSeq          int           // sequence counter; incremented on each cursor move to debounce
	commentExpandDelay time.Duration // <0 = disabled, 0 = instant, >0 = delay before auto-expand

	// Content view mode (for plans/docs)
	contentMode         bool
	contentID           string
	contentTitle        string
	contentHasDiff      bool   // true when multiple versions exist for diffing
	contentVersionCount int    // number of versions for this content item
	diffBaseVersion     int    // base version being diffed from (0 = default latest-vs-previous)
	diffToVersion       int    // target version being diffed to
	contentDiffContent  string // current content text, for toggling back from diff view
	mdStyler            *markdownStyler

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
	m.annotations = nil
	m.isBinary = false
	m.cursor = 0
	m.offset = 0
	m.hOffset = 0
	m.visualMode = false
	m.expandedCommentID = ""
	m.expandSeq++
}

func newDiffViewModel(theme *Theme, keys *KeyMap) diffViewModel {
	return diffViewModel{
		theme:              theme,
		hl:                 newHighlighterWithStyle(theme.SyntaxStyle),
		mdStyler:           newMarkdownStyler(*theme),
		keys:               keys,
		commentExpandDelay: 2 * time.Second,
	}
}

type loadDiffMsg struct {
	path            string
	result          *types.DiffResult
	comments        []types.ReviewComment
	annotations     []types.Annotation
	selectCommentID string // if set, auto-select and expand this comment after loading
	anchorLine      int    // if set, re-anchor cursor to this new-file line after loading
}

// requestFileDiffMsg asks the app to re-fetch a file diff honoring the full-file
// modifier, then re-anchor the viewport. Used by the full-file toggle.
type requestFileDiffMsg struct {
	path       string
	full       bool
	anchorLine int
}

type requestFileContentMsg struct {
	path            string
	selectCommentID string
	anchorLine      int // re-anchor cursor to this new-file line after load (0 = none)
}

type loadFileContentMsg struct {
	path            string
	content         string
	err             error
	comments        []types.ReviewComment
	annotations     []types.Annotation
	selectCommentID string
	anchorLine      int // re-anchor cursor to this new-file line after load (0 = none)
}

type loadAdditionalFileMsg struct {
	path            string
	content         string
	err             error
	comments        []types.ReviewComment
	selectCommentID string
}

// commentExpandTickMsg fires after a delay to expand a hovered comment.
type commentExpandTickMsg struct {
	seq int // must match m.expandSeq to be honoured
}

// cursorMoved handles comment expansion state when the cursor changes position.
// It collapses any expanded comment and schedules a new expand tick if the cursor
// is now on a comment line.
func (m *diffViewModel) cursorMoved() tea.Cmd {
	m.expandSeq++
	m.expandedCommentID = ""

	c := m.CursorComment()
	if c == nil || m.commentExpandDelay < 0 {
		return nil
	}
	if m.commentExpandDelay == 0 {
		m.expandedCommentID = c.ID
		m.ensureVisible()
		return nil
	}
	seq := m.expandSeq
	return tea.Tick(m.commentExpandDelay, func(time.Time) tea.Msg {
		return commentExpandTickMsg{seq: seq}
	})
}

// selectComment moves the cursor to the comment with the given ID and
// schedules it to auto-expand after the configured delay.
func (m *diffViewModel) selectComment(commentID string) tea.Cmd {
	if commentID == "" {
		return nil
	}
	for i, line := range m.lines {
		if line.isComment && line.comment != nil && line.comment.ID == commentID {
			m.cursor = i
			m.expandedCommentID = ""
			m.expandSeq++
			m.ensureVisible()
			if m.commentExpandDelay < 0 {
				return nil
			}
			if m.commentExpandDelay == 0 {
				m.expandedCommentID = commentID
				m.ensureVisible()
				return nil
			}
			seq := m.expandSeq
			return tea.Tick(m.commentExpandDelay, func(time.Time) tea.Msg {
				return commentExpandTickMsg{seq: seq}
			})
		}
	}
	return nil
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
		m.annotations = msg.annotations
		m.isBinary = isBinaryContent(m.hunks)
		// If in file view mode, store hunks but fetch file content instead
		if m.style == diffStyleFile {
			path := m.path
			selectID := msg.selectCommentID
			return m, func() tea.Msg { return requestFileContentMsg{path: path, selectCommentID: selectID} }
		}
		prevCursor := m.cursor
		prevOffset := m.offset
		if m.isBinary {
			m.lines = nil
		} else {
			m.buildLines()
		}
		switch {
		case msg.anchorLine > 0:
			// Re-anchor after a full-file toggle: the line list changed shape,
			// so re-find the same source line and center on it.
			m.reanchorTo(msg.anchorLine)
			m.visualMode = false
		case sameFile && prevCursor < len(m.lines):
			m.cursor = prevCursor
			m.offset = prevOffset
		default:
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.hOffset = 0
			m.visualMode = false
		}
		return m, m.selectComment(msg.selectCommentID)

	case loadContentMsg:
		isReload := m.contentMode && m.contentID == msg.id
		m.contentMode = true
		m.contentID = msg.id
		m.contentTitle = msg.title
		m.contentVersionCount = msg.versionCount
		m.contentHasDiff = msg.versionCount > 1
		m.diffBaseVersion = 0
		m.diffToVersion = 0
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
		m.annotations = nil
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
		selectCmd := m.selectComment(msg.selectCommentID)

		// Auto-switch to preferred diff style when a previous version exists
		if msg.autoSwitchDiff && m.autoContentDiff {
			contentID := m.contentID
			style := m.preferredContentDiffStyle
			return m, func() tea.Msg {
				return requestContentDiffMsg{contentID: contentID, preferredStyle: style}
			}
		}

		return m, selectCmd

	case loadContentDiffMsg:
		if msg.err != nil || msg.result == nil || len(msg.result.Hunks) == 0 {
			return m, nil // stay in content mode on error or no changes
		}
		m.contentMode = false
		m.hunks = msg.result.Hunks
		m.comments = msg.comments
		m.annotations = nil
		m.style = msg.preferredStyle
		m.diffBaseVersion = msg.fromVersion
		m.diffToVersion = msg.toVersion
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
			m.annotations = nil
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
		m.annotations = msg.annotations
		sameFile := msg.path == m.path
		prevCursor := m.cursor
		prevOffset := m.offset
		m.buildFileViewLines(msg.content)
		switch {
		case msg.anchorLine > 0:
			// Re-anchor after a diff-style toggle so the file view lands on the
			// same source line the reviewer was looking at.
			m.reanchorTo(msg.anchorLine)
			m.visualMode = false
		case sameFile && prevCursor < len(m.lines):
			m.cursor = m.nearestSelectable(prevCursor, 1)
			m.offset = prevOffset
		default:
			m.cursor = m.nearestSelectable(0, 1)
			m.offset = 0
			m.visualMode = false
		}
		m.hOffset = 0
		return m, m.selectComment(msg.selectCommentID)

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
		m.annotations = nil
		m.style = diffStyleFile
		m.buildFileViewLines(msg.content)
		m.cursor = m.nearestSelectable(0, 1)
		m.offset = 0
		m.hOffset = 0
		m.visualMode = false
		return m, m.selectComment(msg.selectCommentID)

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
		case key == "space":
			// Toggle expand/collapse on comment under cursor
			if c := m.CursorComment(); c != nil {
				if m.expandedCommentID == c.ID {
					m.expandedCommentID = ""
					m.expandSeq++
				} else {
					m.expandedCommentID = c.ID
					m.expandSeq++
					m.ensureVisible()
				}
				return m, nil
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
		if m.isBinary {
			heading := lipgloss.NewStyle().Bold(true).Render("Binary file — preview not available")
			dim := lipgloss.NewStyle().Faint(true)
			icon := fileIcon(m.path)
			return centerBlock([]string{
				heading,
				"",
				icon + " " + dim.Render(m.path),
			}, m.width, m.height)
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
		// Hide-comments filter: comment-only lines are removed from the view.
		if m.isHiddenComment(line) {
			continue
		}
		selected := i == m.cursor
		inVisual := m.visualMode && m.inVisualRange(i)

		var rendered string
		if line.isHunk {
			rendered = m.renderHunkHeader(line, selected)
		} else if line.isComment {
			rendered = m.renderCommentLine(line, selected)
		} else if line.isAnnotation {
			rendered = m.renderAnnotationLine(line, selected)
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

// isBinaryContent samples the first few hunks/lines for control characters
// that indicate binary content (matching the desktop app's detection logic).
func isBinaryContent(hunks []types.DiffHunk) bool {
	if len(hunks) == 0 {
		return false
	}
	limit := 2
	if len(hunks) < limit {
		limit = len(hunks)
	}
	for _, hunk := range hunks[:limit] {
		lineLimit := 10
		if len(hunk.Lines) < lineLimit {
			lineLimit = len(hunk.Lines)
		}
		for _, line := range hunk.Lines[:lineLimit] {
			for _, b := range line.Content {
				if b <= 0x08 || (b >= 0x0e && b <= 0x1f) {
					return true
				}
			}
		}
	}
	return false
}

func (m *diffViewModel) buildLines() {
	m.lines = nil
	m.ClearSearch()

	if m.style == diffStyleSplit {
		m.buildSplitLines()
		return
	}

	isMd := isMarkdownFile(m.path)

	// File-level comments (LineStart == 0) rendered before hunks
	if !m.hideOverlays {
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

			if !m.hideOverlays {
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
	}

	m.pairLines()
	m.insertInlineAnnotations()
	m.computeCommentLines()
}

// buildContentLines builds lines for a content item (plan/doc) displayed as a document.
func (m *diffViewModel) buildContentLines(content string) {
	m.lines = nil
	m.ClearSearch()

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

	m.computeCommentLines()
}

// buildFileViewLines builds lines from raw file content for file view mode.
// Uses m.path for comment matching (unlike buildContentLines which uses m.contentID).
func (m *diffViewModel) buildFileViewLines(content string) {
	m.lines = nil
	m.ClearSearch()

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

	m.insertInlineAnnotations()
	m.computeCommentLines()
}

func (m *diffViewModel) buildSplitLines() {
	isMd := isMarkdownFile(m.path)

	// File-level comments (LineStart == 0) rendered before hunks
	if !m.hideOverlays {
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

	m.insertInlineAnnotations()
	m.computeCommentLines()
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

// annotationColor is the accent used for agent annotations (boxes + the gutter
// range bar). Distinct from comment colors so the two channels read apart.
const annotationColor = "6" // cyan

// annotationRangeBar is the solid glyph drawn as the far-left rail on every code
// line inside an annotation's range. A full block on a cyan background reads as
// an unbroken vertical line down the left edge of the annotated block.
const annotationRangeBar = "▌"

// gutterWithRangeBar renders a (plain, ASCII line-number) gutter. When the line
// is inside an annotation's range it draws a solid cyan rail in the leftmost
// column — at the far-left edge of the pane — so the range reads as a continuous
// vertical line down the side. base is the gutter's normal style.
func gutterWithRangeBar(gutter string, base lipgloss.Style, annotated bool, bg color.Color) string {
	if !annotated || len(gutter) == 0 {
		return base.Render(gutter)
	}
	// Solid cyan block in column 0; the rest of the gutter keeps its normal style.
	rail := lipgloss.NewStyle().
		Foreground(lipgloss.Color(annotationColor)).
		Background(lipgloss.Color(annotationColor)).
		Render(annotationRangeBar)
	return rail + base.Render(gutter[1:])
}

// renderDimmedComment renders a source-code comment line faint/greyed (used by
// the hide-comments filter), padded to width on the line's background.
func renderDimmedComment(content string, bg color.Color, width int) string {
	style := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8"))
	if bg != nil {
		style = style.Background(bg)
	}
	return applyBgAndPad(style.Render(content), bg, width)
}

// renderAnnotationLine renders an agent annotation box. Each sub-line is tinted
// with the annotation accent and prefixed with a bar so it reads as a single
// attached block; when selected it reverses like other inline overlays.
// annotationBoxRows wraps an annotation box's logical lines (summary, refs) to
// the inner content width — between the left bar and the right border — so long
// text wraps instead of running off the pane.
func (m diffViewModel) annotationBoxRows(content string) []string {
	cw := m.width - 2 // left bar + right border
	if cw < 1 {
		cw = 1
	}
	var rows []string
	for _, logical := range strings.Split(content, "\n") {
		w := wrapContent(logical, cw)
		if len(w) == 0 {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, w...)
	}
	return rows
}

func (m diffViewModel) renderAnnotationLine(line diffViewLine, selected bool) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor))
	if selected && m.focused {
		style = style.Reverse(true)
	}
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor))
	leftBar := barStyle.Render("▌")
	rightBar := barStyle.Render("│")
	cw := m.width - 2
	if cw < 1 {
		cw = 1
	}
	rows := m.annotationBoxRows(line.content)
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(leftBar + style.Render(padToWidth(r, cw)) + rightBar)
	}
	return b.String()
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
		origCode := m.originalCodeForComment(line.comment)
		content = formatExpandedComment(line.comment, m.width, origCode, m.wrap)
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
	inCodeFence := false
	isSuggestion := expanded && line.comment != nil && strings.Contains(line.comment.Body, "```suggestion")
	var suggestionDiffBg color.Color // tracks current diff section bg for wrapped continuation lines
	var b strings.Builder
	for i, sl := range subLines {
		if i > 0 {
			b.WriteString("\n")
		}

		if expanded {
			if after := extractCommentContent(sl); strings.HasPrefix(after, "```") {
				inCodeFence = !inCodeFence
				b.WriteString(style.Render(fmt.Sprintf("%-*s", m.width, sl)))
				continue
			}
			if inCodeFence {
				b.WriteString(m.renderExpandedCodeLine(sl, style))
				continue
			}
			// Render suggestion diff lines with diff colors
			if isSuggestion {
				raw := rawCommentContent(sl)
				if strings.HasPrefix(raw, "- ") {
					suggestionDiffBg = m.theme.RemovedBg
					b.WriteString(m.renderSuggestionDiffLine(sl, style, suggestionDiffBg))
					continue
				} else if strings.HasPrefix(raw, "+ ") {
					suggestionDiffBg = m.theme.AddedBg
					b.WriteString(m.renderSuggestionDiffLine(sl, style, suggestionDiffBg))
					continue
				} else if suggestionDiffBg != nil && strings.HasPrefix(raw, "  ") {
					// Wrapped continuation line of a diff entry
					b.WriteString(m.renderSuggestionDiffLine(sl, style, suggestionDiffBg))
					continue
				} else {
					suggestionDiffBg = nil
				}
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

// rawCommentContent returns the text after the "║ " prefix without stripping
// extra whitespace. Unlike extractCommentContent, this preserves leading spaces
// so callers can distinguish "- ", "+ ", and "  " (continuation) prefixes in
// suggestion diff lines.
func rawCommentContent(sl string) string {
	idx := strings.Index(sl, "║")
	if idx < 0 {
		return ""
	}
	rest := sl[idx+len("║"):]
	if len(rest) > 0 && rest[0] == ' ' {
		rest = rest[1:]
	}
	return rest
}

// originalCodeForComment extracts the original (new-file) code lines that a
// comment targets, based on its LineStart..LineEnd range.
func (m diffViewModel) originalCodeForComment(c *types.ReviewComment) string {
	if c == nil || c.LineStart == 0 {
		return ""
	}
	end := c.LineEnd
	if end == 0 {
		end = c.LineStart
	}
	var codeLines []string
	for _, line := range m.lines {
		if line.isComment || line.isHunk {
			continue
		}
		ln := line.rightLineNum
		if ln == 0 {
			ln = line.newLineNum
		}
		if ln == 0 {
			continue
		}
		if ln >= c.LineStart && ln <= end {
			content := line.content
			if line.isSplit {
				content = line.rightContent
			}
			codeLines = append(codeLines, content)
		}
	}
	return strings.Join(codeLines, "\n")
}

// renderSuggestionDiffLine renders a diff line (prefixed with "- ", "+ ", or
// continuation "  ") inside an expanded suggestion comment, using the given
// background color and syntax highlighting.
func (m diffViewModel) renderSuggestionDiffLine(sl string, commentStyle lipgloss.Style, bg color.Color) string {
	outerIdx := strings.Index(sl, "║")
	if outerIdx < 0 {
		return commentStyle.Render(fmt.Sprintf("%-*s", m.width, sl))
	}

	outerEnd := outerIdx + len("║")
	if outerEnd < len(sl) && sl[outerEnd] == ' ' {
		outerEnd++
	}
	outerPrefix := sl[:outerEnd]
	after := sl[outerEnd:]

	styledOuter := commentStyle.Render(outerPrefix)
	codeWidth := m.width - ansi.StringWidth(outerPrefix)
	if codeWidth < 1 {
		codeWidth = 1
	}

	// Syntax-highlight the code portion (after the +/- or continuation prefix)
	prefix := after[:2]
	code := after[2:]
	highlightedCode := m.hl.highlightLine(m.path, code, bg, nil, nil, codeWidth-2)
	prefixStyle := lipgloss.NewStyle()
	if bg != nil {
		prefixStyle = prefixStyle.Background(bg)
	}
	return styledOuter + prefixStyle.Render(prefix) + highlightedCode
}

// renderExpandedCodeLine renders a code line from a suggestion block with syntax
// highlighting. The ║ prefix keeps the comment color; the code gets highlighted.
func (m diffViewModel) renderExpandedCodeLine(sl string, commentStyle lipgloss.Style) string {
	outerIdx := strings.Index(sl, "║")
	if outerIdx < 0 {
		return commentStyle.Render(fmt.Sprintf("%-*s", m.width, sl))
	}

	outerEnd := outerIdx + len("║")
	if outerEnd < len(sl) && sl[outerEnd] == ' ' {
		outerEnd++
	}
	outerPrefix := sl[:outerEnd]
	code := sl[outerEnd:]

	styledOuter := commentStyle.Render(outerPrefix)
	codeWidth := m.width - ansi.StringWidth(outerPrefix)
	if codeWidth < 1 {
		codeWidth = 1
	}
	highlightedCode := m.hl.highlightLine(m.path, code, nil, nil, nil, codeWidth)

	return styledOuter + highlightedCode
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
	renderedGutter := gutterWithRangeBar(gutter, gutterStyle, line.annotated, nil)

	// Hide-comments filter: dim comment-only lines.
	if m.isDimmedComment(line) {
		return renderedGutter + renderDimmedComment(content, nil, contentWidth)
	}

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
		sc, sbg := m.applySearchHighlight(content, nil, nil)
		renderedContent = m.hl.highlightLine(m.path, content, nil, sbg, sc, contentWidth)
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
	// Annotated code lines get a cyan bar in the gutter's trailing column to mark
	// the annotation's range.
	renderedGutter := gutterWithRangeBar(gutter, gutterStyle, line.annotated, lineBg)

	// Hide-comments filter: dim comment-only lines instead of syntax-highlighting.
	if m.isDimmedComment(line) {
		return renderedGutter + renderDimmedComment(content, lineBg, contentWidth)
	}

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
		sc, sbg := m.applySearchHighlight(content, changes, changeBg)
		renderedContent = m.hl.highlightLine(m.path, content, lineBg, sbg, sc, contentWidth)
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

	// Render each side, then clamp each to exactly gutterW+contentW columns.
	// renderSplitSide only pads up to width; markdown styling or width-measure
	// drift can leave a side over- or under-wide, which shifts the divider and
	// (when over-wide) lets the terminal wrap the row. Hard-fitting both sides
	// keeps the divider in a stable column regardless of content or wrap mode.
	sideW := gutterW + contentW
	// Annotation ranges and the comment dim are keyed on new-file lines, so they
	// apply to the right (new) side; the left side only dims on context lines,
	// where both sides show the same line.
	dimmed := m.isDimmedComment(line)
	leftStyled := fitToWidth(m.renderSplitSide(leftGutter, leftRawContent, line.kind, line.leftEmpty, leftChanges, gutterW, contentW, line, false, dimmed && line.kind == types.DiffLineContext), sideW)
	rightStyled := fitToWidth(m.renderSplitSide(rightGutter, rightRawContent, line.rightKind, line.rightEmpty, rightChanges, gutterW, contentW, line, line.annotated, dimmed), sideW)
	divStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(divider)

	return leftStyled + divStyled + rightStyled
}

func (m diffViewModel) renderSplitSide(gutter, content string, kind types.DiffLineKind, empty bool, changes []changeRange, gutterW, contentW int, line diffViewLine, annotated, dimmed bool) string {
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
	renderedGutter := gutterWithRangeBar(gutter, gutterStyle, annotated, lineBg)

	// Hide-comments filter: dim comment-only lines.
	if dimmed {
		return renderedGutter + renderDimmedComment(content, lineBg, contentW)
	}

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
		sc, sbg := m.applySearchHighlight(content, changes, changeBg)
		renderedContent = m.hl.highlightLine(m.path, content, lineBg, sbg, sc, contentW)
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

// fitToWidth forces a (possibly ANSI-styled) string to exactly the given visual
// width: it truncates content wider than width (preserving escape sequences) and
// pads content narrower than width. Used to keep split-diff columns aligned.
func fitToWidth(s string, width int) string {
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "")
	}
	return padToWidth(s, width)
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
		// Annotated lines keep the cyan range rail on every wrapped row.
		annotated := mdLine != nil && mdLine.annotated
		renderedGutter := gutterWithRangeBar(chunkGutter, gutterStyle, annotated, lineBg)

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
			sc, sbg := m.applySearchHighlight(chunk, nil, changeBg)
			renderedContent = m.hl.highlightLine(m.path, chunk, lineBg, sbg, sc, contentWidth)
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

// ToggleFullFile flips the full-file diff modifier and re-fetches the diff with
// the new amount of context. It applies to regular file diffs shown in unified
// or split style; it is a no-op for content items, raw file view (already the
// whole file), and additional files.
func (m *diffViewModel) ToggleFullFile() tea.Cmd {
	if m.additionalFilePath != "" || m.contentID != "" || m.style == diffStyleFile {
		return nil
	}
	m.fullFile = !m.fullFile
	path := m.path
	full := m.fullFile
	anchor := m.anchorLineForCursor()
	return func() tea.Msg {
		return requestFileDiffMsg{path: path, full: full, anchorLine: anchor}
	}
}

// ToggleOverlays hides or shows inline comments and annotations, re-anchoring the
// cursor to the same source line so the viewport doesn't jump.
func (m *diffViewModel) ToggleOverlays() {
	anchor := m.anchorLineForCursor()
	m.hideOverlays = !m.hideOverlays
	m.buildLines()
	if anchor > 0 {
		if idx := m.indexForNewLine(anchor); idx >= 0 {
			m.cursor = idx
		}
	}
	if m.cursor >= len(m.lines) {
		m.cursor = len(m.lines) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.cursor = m.nearestSelectable(m.cursor, 1)
	m.ensureVisible()
}

// CycleCommentFilter advances the comment filter: shown → dimmed → hidden →
// shown. Dim renders comment-only lines faint; hide removes them from the view
// (skipped at render and in navigation). It recomputes the comment-line set and
// nudges the cursor off any now-hidden line; no rebuild, so scroll stays stable.
func (m *diffViewModel) CycleCommentFilter() {
	m.commentFilter = (m.commentFilter + 1) % 3
	m.computeCommentLines()
	m.cursor = m.nearestSelectable(m.cursor, 1)
	m.ensureVisible()
}

// CommentFilterLabel returns a short label for the current filter state, or ""
// when comments are shown normally. Used for a transient status hint.
func (m diffViewModel) CommentFilterLabel() string {
	switch m.commentFilter {
	case commentsDimmed:
		return "comments dimmed"
	case commentsHidden:
		return "comments hidden"
	default:
		return ""
	}
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

	// Source line under the cursor, preserved across the rebuild so the
	// viewport re-anchors instead of keeping a now-meaningless line index.
	srcLine := m.anchorLineForCursor()

	// Viewing a content diff: cycle unified → split → back to content
	if !m.contentMode && m.contentID != "" {
		switch m.style {
		case diffStyleUnified:
			m.style = diffStyleSplit
			m.buildLines()
			m.reanchorTo(srcLine)
			return nil
		default:
			// Back to content view
			m.contentMode = true
			m.style = diffStyleUnified
			m.hunks = nil
			m.buildContentLines(m.contentDiffContent)
			m.reanchorTo(srcLine)
			return nil
		}
	}

	// Regular file diffs
	switch m.style {
	case diffStyleUnified:
		m.style = diffStyleSplit
		m.buildLines()
		m.reanchorTo(srcLine)
	case diffStyleSplit:
		path := m.path
		return func() tea.Msg { return requestFileContentMsg{path: path, anchorLine: srcLine} }
	case diffStyleFile:
		m.style = diffStyleUnified
		m.buildLines()
		m.reanchorTo(srcLine)
	}
	return nil
}

// indexForNewLine returns the index of the lines[] entry whose new-file line
// number matches n (split lines carry it in rightLineNum), or -1 if none.
func (m diffViewModel) indexForNewLine(n int) int {
	if n <= 0 {
		return -1
	}
	for i := range m.lines {
		if m.lines[i].rightLineNum == n || m.lines[i].newLineNum == n {
			return i
		}
	}
	return -1
}

// reanchorTo re-centers the viewport on the rebuilt line matching source line
// lineNum after a style change. Falls back to the top when the line isn't found
// (e.g. it was a pure-removed line absent from the new layout).
func (m *diffViewModel) reanchorTo(lineNum int) {
	m.hOffset = 0
	idx := m.indexForNewLine(lineNum)
	if idx < 0 {
		m.cursor = m.nearestSelectable(0, 1)
		m.offset = 0
		return
	}
	m.cursor = m.nearestSelectable(idx, 1)
	m.offset = m.cursor - m.height/2
	if m.offset < 0 {
		m.offset = 0
	}
	m.ensureVisible()
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
	// Hidden comment lines occupy no screen rows (filter in the hide state).
	if m.isHiddenComment(line) {
		return 0
	}
	// Comments render as a multi-line box regardless of wrap mode: a collapsed
	// comment is a 3-line box (line.content), an expanded one spans its body.
	// This must match what renderCommentLine/View actually draw, or the scroll
	// and cursor math desync and the cursor/bottom run off the viewport.
	if line.isComment {
		if line.comment != nil && line.comment.ID == m.expandedCommentID {
			origCode := m.originalCodeForComment(line.comment)
			return strings.Count(formatExpandedComment(line.comment, m.width, origCode, m.wrap), "\n") + 1
		}
		return strings.Count(line.content, "\n") + 1
	}
	// Annotation boxes wrap their text to the pane width; count the wrapped rows
	// so scroll math matches what renderAnnotationLine draws.
	if line.isAnnotation {
		return len(m.annotationBoxRows(line.content))
	}
	if !m.wrap {
		return 1
	}
	if line.isHunk || line.isSplit {
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
	// Account for rows that occupy more than one screen line (expanded comments
	// always, wrapped lines in wrap mode). Only scroll while content remains
	// below the viewport.
	screenLines := 0
	for i := m.offset; i < len(m.lines); i++ {
		screenLines += m.screenLinesFor(i)
		if screenLines > m.height {
			m.offset++
			return
		}
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
	// Scroll the viewport down. Account for rows that occupy more than one
	// screen line (expanded comments always, wrapped lines in wrap mode) so the
	// cursor can't be left stranded at the bottom of a file.
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
	// Move the cursor down with the viewport so it keeps roughly the same
	// relative screen position (vim Ctrl-D). Near the bottom, where the viewport
	// can no longer scroll, the cursor still advances toward the last line.
	target := m.cursor + jump
	if target >= len(m.lines) {
		target = len(m.lines) - 1
	}
	m.cursor = m.nearestSelectable(target, 1)
	m.ensureVisible()
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
	// Move the cursor up with the viewport (vim Ctrl-U), keeping its relative
	// screen position; near the top it advances toward the first line.
	target := m.cursor - jump
	if target < 0 {
		target = 0
	}
	m.cursor = m.nearestSelectable(target, -1)
	m.ensureVisible()
}

// isCursorOffScreen returns true if the cursor is outside the visible viewport.
// Screen lines are counted (not logical lines) so multi-row rows — expanded and
// collapsed comments always, wrapped lines in wrap mode — are measured correctly;
// otherwise the cursor can be reported on-screen while it has actually scrolled
// off the bottom past a tall comment.
func (m diffViewModel) isCursorOffScreen() bool {
	if m.cursor < m.offset {
		return true
	}
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
// It walks from the offset summing screen lines so comments (which occupy
// multiple rows even in non-wrap mode) and wrapped lines don't make it overshoot
// past the bottom of the viewport.
func (m diffViewModel) lastVisibleLine() int {
	screenLines := 0
	last := m.offset
	for i := m.offset; i < len(m.lines); i++ {
		sl := m.screenLinesFor(i)
		if screenLines+sl > m.height && i > m.offset {
			break
		}
		screenLines += sl
		last = i
	}
	return last
}

// lineHasChange reports whether line i is an added or removed line on either
// side of the diff. Comment, annotation, and hunk-header lines never count.
func (m diffViewModel) lineHasChange(i int) bool {
	if i < 0 || i >= len(m.lines) {
		return false
	}
	line := m.lines[i]
	if line.isComment || line.isAnnotation || line.isHunk {
		return false
	}
	if line.kind == types.DiffLineAdded || line.kind == types.DiffLineRemoved {
		return true
	}
	return line.rightKind == types.DiffLineAdded || line.rightKind == types.DiffLineRemoved
}

// isChangeBlockStart reports whether line i begins a run of changed lines — a
// changed line whose predecessor is not itself a changed line.
func (m diffViewModel) isChangeBlockStart(i int) bool {
	return m.lineHasChange(i) && !m.lineHasChange(i-1)
}

// selectableForChange returns the line the cursor should land on for a change
// block that starts at i. Block-start lines are often removed lines (not
// selectable), so it picks the nearest selectable line, preferring forward into
// the block. Returns -1 when no selectable line exists.
func (m diffViewModel) selectableForChange(i int) int {
	t := m.nearestSelectable(i, 1)
	if m.isSelectable(t) {
		return t
	}
	t = m.nearestSelectable(i, -1)
	if m.isSelectable(t) {
		return t
	}
	return -1
}

// JumpToChange moves the cursor to the start of the next (dir=+1) or previous
// (dir=-1) block of changed lines and scrolls it into view. A "block" is a run of
// consecutive added/removed lines, so this works the same in compact and
// full-file modes — full-file diffs are a single git hunk, but each contiguous
// edit is still its own block. Returns false (a no-op) when there is no change in
// that direction.
func (m *diffViewModel) JumpToChange(dir int) bool {
	n := len(m.lines)
	if n == 0 {
		return false
	}

	if dir > 0 {
		for i := m.cursor + 1; i < n; i++ {
			if m.isChangeBlockStart(i) && m.tryLandOnChange(i) {
				return true
			}
		}
		return false
	}

	// Backward: skip past the block the cursor currently sits in (its start is
	// before the cursor and would otherwise bounce us back), then jump to the
	// previous block's start.
	bound := m.cursor
	if m.lineHasChange(m.cursor) {
		start := m.cursor
		for start-1 >= 0 && m.lineHasChange(start-1) {
			start--
		}
		bound = start
	}
	for i := bound - 1; i >= 0; i-- {
		if m.isChangeBlockStart(i) && m.tryLandOnChange(i) {
			return true
		}
	}
	return false
}

// tryLandOnChange moves the cursor to the selectable line for the change block at
// i and scrolls it into view. It returns false (without moving) when there is no
// selectable line or when the landing spot is the cursor's current line — so a
// block that can't actually be landed on (e.g. a pure deletion) is skipped rather
// than trapping the cursor.
func (m *diffViewModel) tryLandOnChange(i int) bool {
	target := m.selectableForChange(i)
	if target < 0 || target == m.cursor {
		return false
	}
	m.cursor = target
	m.centerCursor()
	return true
}

// centerCursor scrolls so the cursor sits near the middle of the viewport,
// clamping at the file ends. Used when jumping between change blocks so the
// landing chunk is centered rather than pinned to an edge.
func (m *diffViewModel) centerCursor() {
	if m.height <= 0 {
		m.ensureVisible()
		return
	}
	m.offset = m.cursor - m.height/2
	if m.offset < 0 {
		m.offset = 0
	}
	m.ensureVisible() // clamp + respect screen-line accounting
}

// LandOnChunkEdge positions the cursor on the first (dir>=0) or last (dir<0)
// change block in the file and centers it. Used when [ / ] cross into an
// adjacent file so the cursor lands on a chunk instead of the file's top.
func (m *diffViewModel) LandOnChunkEdge(dir int) {
	target := -1
	if dir >= 0 {
		for i := 0; i < len(m.lines); i++ {
			if m.isChangeBlockStart(i) {
				target = i
				break
			}
		}
	} else {
		for i := len(m.lines) - 1; i >= 0; i-- {
			if m.isChangeBlockStart(i) {
				target = i
				break
			}
		}
	}
	if target < 0 {
		return
	}
	if sel := m.selectableForChange(target); sel >= 0 {
		m.cursor = sel
		m.centerCursor()
	}
}

// JumpToMark moves the cursor to the next (dir=+1) or previous (dir=-1) review
// mark — an inline comment box or an agent annotation box — and scrolls it into
// view. No-op when there are no marks in that direction (no wrap-around).
func (m *diffViewModel) JumpToMark(dir int) bool {
	for i := m.cursor + dir; i >= 0 && i < len(m.lines); i += dir {
		if m.lines[i].isComment || m.lines[i].isAnnotation {
			m.cursor = i
			m.ensureVisible()
			return true
		}
	}
	return false
}

func (m *diffViewModel) ensureVisible() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	// Count screen lines from offset to cursor. screenLinesFor returns >1 for
	// expanded comments (regardless of wrap) and for wrapped lines in wrap mode,
	// so advancing offset by screen lines keeps the cursor on screen even when
	// rows occupy multiple terminal rows.
	screenLines := 0
	for i := m.offset; i <= m.cursor && i < len(m.lines); i++ {
		screenLines += m.screenLinesFor(i)
	}
	for screenLines > m.height && m.offset < m.cursor {
		screenLines -= m.screenLinesFor(m.offset)
		m.offset++
	}
}

// --- Diff search ---

// searchTextFor returns the searchable text for a line: the content, plus the
// right side for split lines.
func searchTextFor(line diffViewLine) string {
	if line.isSplit && line.rightContent != "" {
		return line.content + "\n" + line.rightContent
	}
	return line.content
}

// queryCaseInsensitive applies smartcase: a query with no uppercase letters is
// matched case-insensitively.
func queryCaseInsensitive(query string) bool {
	return query == strings.ToLower(query)
}

// lineMatchesQuery reports whether a line contains the query (smartcase).
func lineMatchesQuery(line diffViewLine, query string) bool {
	if query == "" {
		return false
	}
	text := searchTextFor(line)
	if queryCaseInsensitive(query) {
		return strings.Contains(strings.ToLower(text), strings.ToLower(query))
	}
	return strings.Contains(text, query)
}

// RunSearch recomputes matches for query and jumps to the nearest match from
// originCursor in the given direction. An empty query clears the matches but
// leaves originCursor in place. Returns the number of matches found.
func (m *diffViewModel) RunSearch(query string, backward bool, originCursor int) int {
	m.searchQuery = query
	m.searchMatches = nil
	m.searchIndex = 0
	m.searchBackward = backward
	if query == "" {
		m.cursor = originCursor
		m.ensureVisible()
		return 0
	}
	for i := range m.lines {
		if lineMatchesQuery(m.lines[i], query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	if len(m.searchMatches) == 0 {
		// Keep the query (so the prompt shows it) but don't move.
		m.cursor = originCursor
		m.ensureVisible()
		return 0
	}
	m.searchIndex = m.nearestMatchFrom(originCursor, backward)
	m.jumpToMatch()
	return len(m.searchMatches)
}

// nearestMatchFrom returns the position within searchMatches of the nearest
// match from origin in the given direction, wrapping around the ends. Assumes
// searchMatches is non-empty and sorted ascending.
func (m diffViewModel) nearestMatchFrom(origin int, backward bool) int {
	if backward {
		// Last match strictly before origin, else wrap to the last match.
		for i := len(m.searchMatches) - 1; i >= 0; i-- {
			if m.searchMatches[i] < origin {
				return i
			}
		}
		return len(m.searchMatches) - 1
	}
	// First match at or after origin, else wrap to the first match.
	for i := 0; i < len(m.searchMatches); i++ {
		if m.searchMatches[i] >= origin {
			return i
		}
	}
	return 0
}

// jumpToMatch moves the cursor to the current match and centers the viewport.
func (m *diffViewModel) jumpToMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	if m.searchIndex < 0 {
		m.searchIndex = 0
	}
	if m.searchIndex >= len(m.searchMatches) {
		m.searchIndex = len(m.searchMatches) - 1
	}
	m.cursor = m.nearestSelectable(m.searchMatches[m.searchIndex], 1)
	m.hOffset = 0
	m.offset = m.cursor - m.height/2
	if m.offset < 0 {
		m.offset = 0
	}
	m.ensureVisible()
}

// StepMatch advances to the next match (backward=false) or previous match
// (backward=true), wrapping around. No-op when there are no matches.
func (m *diffViewModel) StepMatch(backward bool) {
	if len(m.searchMatches) == 0 {
		return
	}
	if backward {
		m.searchIndex--
		if m.searchIndex < 0 {
			m.searchIndex = len(m.searchMatches) - 1
		}
	} else {
		m.searchIndex++
		if m.searchIndex >= len(m.searchMatches) {
			m.searchIndex = 0
		}
	}
	m.jumpToMatch()
}

// ClearSearch removes the active search query and matches.
func (m *diffViewModel) ClearSearch() {
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchIndex = 0
}

// SearchActive reports whether a search with matches is in effect.
func (m diffViewModel) SearchActive() bool {
	return m.searchQuery != "" && len(m.searchMatches) > 0
}

// SearchStatus returns the 1-based current match position and total count.
func (m diffViewModel) SearchStatus() (int, int) {
	if len(m.searchMatches) == 0 {
		return 0, 0
	}
	return m.searchIndex + 1, len(m.searchMatches)
}

// searchMatchBg is the background applied to highlighted search matches.
var searchMatchBg = lipgloss.Color("3")

// matchRanges returns the byte ranges in content where query occurs (smartcase:
// case-insensitive unless the query has an uppercase letter).
func matchRanges(content, query string) []changeRange {
	if query == "" {
		return nil
	}
	hay, needle := content, query
	if queryCaseInsensitive(query) {
		hay = strings.ToLower(content)
		needle = strings.ToLower(query)
	}
	var ranges []changeRange
	from := 0
	for from <= len(hay) {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(needle)
		ranges = append(ranges, changeRange{start: start, end: end})
		from = end
	}
	return ranges
}

// applySearchHighlight overrides the change ranges/background to highlight
// search matches in content. When no search is active (or the line has no
// match) it returns the passed-through diff-change values unchanged.
func (m diffViewModel) applySearchHighlight(content string, changes []changeRange, changeBg color.Color) ([]changeRange, color.Color) {
	if m.searchQuery == "" {
		return changes, changeBg
	}
	if sr := matchRanges(content, m.searchQuery); len(sr) > 0 {
		return sr, searchMatchBg
	}
	return changes, changeBg
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
	// Hidden comment lines can't hold the cursor (filter in the hide state).
	if m.isHiddenComment(line) {
		return false
	}
	if line.isComment {
		return true
	}
	// Skip pure removed lines — they have no new-file line number and can't be
	// commented on. In split mode an in-place change is built as a removed line
	// paired with an added right side: kind == DiffLineRemoved and newLineNum == 0,
	// but rightLineNum carries the new-file number. Those remain selectable so the
	// cursor can land on the right-side content (matching lineNumAt).
	if line.kind == types.DiffLineRemoved && line.newLineNum == 0 && line.rightLineNum == 0 {
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

// nearestSelectable finds the closest selectable line from pos, preferring the given direction.
func (m diffViewModel) nearestSelectable(pos, dir int) int {
	if len(m.lines) == 0 {
		return 0
	}
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
	// Comments reference new-file line numbers. In split mode the new-file
	// number lives in rightLineNum; in unified/file mode it lives in newLineNum.
	if line.rightLineNum > 0 {
		return line.rightLineNum
	}
	return line.newLineNum
}

func (m diffViewModel) currentDiffLine() int {
	return m.lineNumAt(m.cursor)
}

// anchorLineForCursor returns the new-file line to re-anchor the viewport on
// after a rebuild (diff-style toggle, full-file toggle, refresh). When the cursor
// sits on a row with no file number — a hunk header, a comment box, or an
// annotation box — it falls back to the nearest code line so the viewport stays
// put instead of jumping to the top.
func (m diffViewModel) anchorLineForCursor() int {
	if ln := m.lineNumAt(m.cursor); ln > 0 {
		return ln
	}
	for d := 1; d < len(m.lines); d++ {
		if i := m.cursor - d; i >= 0 {
			if ln := m.lineNumAt(i); ln > 0 {
				return ln
			}
		}
		if i := m.cursor + d; i < len(m.lines) {
			if ln := m.lineNumAt(i); ln > 0 {
				return ln
			}
		}
	}
	return 0
}

// EditorTargetLine returns the new-file line number an external editor should
// open at: the cursor's line when the cursor is visible in the viewport,
// otherwise the line at the top of the viewport — so the editor lands where the
// reviewer is actually looking after scrolling away from the cursor. Lines with
// no file number (hunk headers, comments) are skipped by scanning downward.
func (m diffViewModel) EditorTargetLine() int {
	if !m.isCursorOffScreen() {
		if ln := m.lineNumAt(m.cursor); ln > 0 {
			return ln
		}
	}
	for i := m.offset; i < len(m.lines); i++ {
		if ln := m.lineNumAt(i); ln > 0 {
			return ln
		}
	}
	return 1
}

// lineText returns the source text of a line for yanking: the code content,
// the right side for split-mode added lines, or a comment's body.
func lineText(line diffViewLine) string {
	if line.isComment {
		if line.comment != nil {
			return line.comment.Body
		}
		return ""
	}
	if line.isSplit && line.leftEmpty {
		return line.rightContent
	}
	return line.content
}

// YankText returns the text to copy to the clipboard: the selected lines
// (joined by newlines) when visual mode is active, otherwise the cursor line.
// CurrentLineText returns the text content of the line under the cursor.
func (m diffViewModel) CurrentLineText() string {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return ""
	}
	return lineText(m.lines[m.cursor])
}

func (m diffViewModel) YankText() string {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return ""
	}
	if m.visualMode {
		start, end := m.visualStart, m.cursor
		if start > end {
			start, end = end, start
		}
		var parts []string
		for i := start; i <= end && i < len(m.lines); i++ {
			parts = append(parts, lineText(m.lines[i]))
		}
		return strings.Join(parts, "\n")
	}
	return lineText(m.lines[m.cursor])
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
	// Hidden comment lines render nothing (filter in the hide state).
	if m.isHiddenComment(line) {
		return 0
	}

	var rendered string
	if line.isHunk {
		rendered = m.renderHunkHeader(line, false)
	} else if line.isComment {
		rendered = m.renderCommentLine(line, false)
	} else if line.isAnnotation {
		rendered = m.renderAnnotationLine(line, false)
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
	prefillBody string            // pre-filled body text (for suggestions)
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

// insertInlineAnnotations flags every code line inside an annotation's range so
// the gutter draws the cyan range bar, and inserts each annotation's box after
// the last line of its range. Runs as a single post-build pass so unified,
// split, and full-file modes all render annotations identically. No-op when
// overlays are hidden.
func (m *diffViewModel) insertInlineAnnotations() {
	if m.hideOverlays || len(m.annotations) == 0 {
		return
	}
	var result []diffViewLine
	for _, line := range m.lines {
		// New-file line number: split lines carry it on the right side.
		ln := line.rightLineNum
		if ln == 0 {
			ln = line.newLineNum
		}
		if ln > 0 && m.lineInAnnotation(ln) {
			line.annotated = true
		}
		result = append(result, line)
		if ln == 0 {
			continue
		}
		for i := range m.annotations {
			a := &m.annotations[i]
			if a.TargetRef == m.path && annotationAnchor(a) == ln {
				result = append(result, diffViewLine{
					isAnnotation: true,
					annotation:   a,
					content:      formatAnnotation(a),
				})
			}
		}
	}
	m.lines = result
}

// computeCommentLines classifies which displayed code lines are source-code
// comment-only and records their new-file line numbers, so the renderer can dim
// them. It reconstructs the new-file text from the built lines (in order) and
// tokenises it once, so block comments spanning consecutive lines classify
// correctly. No-op (and clears the set) when the toggle is off.
func (m *diffViewModel) computeCommentLines() {
	m.commentLines = nil
	if m.commentFilter == commentsShown || m.hl == nil {
		return
	}
	type codeLine struct {
		num  int
		text string
	}
	var code []codeLine
	for _, ln := range m.lines {
		if ln.isHunk || ln.isComment || ln.isAnnotation {
			continue
		}
		// New-file side: split lines carry it on the right.
		num, text := ln.rightLineNum, ln.rightContent
		if num == 0 {
			num, text = ln.newLineNum, ln.content
		}
		if num <= 0 {
			continue
		}
		code = append(code, codeLine{num, text})
	}
	if len(code) == 0 {
		return
	}
	var sb strings.Builder
	for i, c := range code {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(c.text)
	}
	set := m.hl.commentOnlyLines(m.path, sb.String())
	if len(set) == 0 {
		return
	}
	m.commentLines = make(map[int]bool, len(set))
	for i, c := range code {
		if set[i+1] {
			m.commentLines[c.num] = true
		}
	}
}

// commentLineNum returns the new-file line number of a code line that the
// comment filter has classified as comment-only, or 0 when the line is not a
// classified comment line.
func (m diffViewModel) commentLineNum(line diffViewLine) int {
	if len(m.commentLines) == 0 || line.isHunk || line.isComment || line.isAnnotation {
		return 0
	}
	num := line.rightLineNum
	if num == 0 {
		num = line.newLineNum
	}
	if num > 0 && m.commentLines[num] {
		return num
	}
	return 0
}

// isDimmedComment reports whether a code line should be rendered faint because
// the filter is in the dim state and the line is comment-only.
func (m diffViewModel) isDimmedComment(line diffViewLine) bool {
	return m.commentFilter == commentsDimmed && m.commentLineNum(line) > 0
}

// isHiddenComment reports whether a code line should be removed from the view
// because the filter is in the hide state and the line is comment-only.
func (m diffViewModel) isHiddenComment(line diffViewLine) bool {
	return m.commentFilter == commentsHidden && m.commentLineNum(line) > 0
}

// CursorAnnotation returns the annotation under the cursor: the annotation whose
// box the cursor is on, or the one whose code range covers the cursor's line.
func (m diffViewModel) CursorAnnotation() *types.Annotation {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return nil
	}
	line := m.lines[m.cursor]
	if line.isAnnotation {
		return line.annotation
	}
	ln := line.newLineNum
	if ln <= 0 {
		return nil
	}
	for i := range m.annotations {
		a := &m.annotations[i]
		if a.TargetRef != m.path {
			continue
		}
		end := a.LineEnd
		if end == 0 {
			end = a.LineStart
		}
		if ln >= a.LineStart && ln <= end {
			return a
		}
	}
	return nil
}

// annotationAnchor returns the new-file line after which an annotation's box is
// drawn (the last line of its range).
func annotationAnchor(a *types.Annotation) int {
	if a.LineEnd > 0 {
		return a.LineEnd
	}
	return a.LineStart
}

// lineInAnnotation reports whether a new-file line number falls within any
// annotation's range for the current file (used to draw the gutter range bar).
func (m diffViewModel) lineInAnnotation(lineNum int) bool {
	for i := range m.annotations {
		a := &m.annotations[i]
		if a.TargetRef != m.path {
			continue
		}
		end := a.LineEnd
		if end == 0 {
			end = a.LineStart
		}
		if lineNum >= a.LineStart && lineNum <= end {
			return true
		}
	}
	return false
}

// formatAnnotation renders the inline annotation box: a one-line summary and,
// when present, a line of numbered doc links.
func formatAnnotation(a *types.Annotation) string {
	lines := []string{fmt.Sprintf("  ⟐ %s", a.Summary)}
	if len(a.Refs) > 0 {
		parts := make([]string, 0, len(a.Refs))
		for i, r := range a.Refs {
			label := r.Label
			if label == "" {
				label = r.Doc
			}
			parts = append(parts, fmt.Sprintf("[%d] %s", i+1, label))
		}
		lines = append(lines, "    ↪ "+strings.Join(parts, "  "))
	}
	return strings.Join(lines, "\n")
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

// extractSuggestionCode extracts the code content from a ```suggestion block.
// Returns the code lines and true if a suggestion block was found.
func extractSuggestionCode(body string) (string, bool) {
	start := strings.Index(body, "```suggestion")
	if start < 0 {
		return "", false
	}
	// Skip past the opening fence line
	codeStart := strings.Index(body[start:], "\n")
	if codeStart < 0 {
		return "", false
	}
	codeStart += start + 1

	// Find closing fence
	codeEnd := strings.Index(body[codeStart:], "\n```")
	if codeEnd < 0 {
		// Try without leading newline (fence at end of body)
		if strings.HasSuffix(body, "```") {
			codeEnd = len(body) - codeStart - 3
		} else {
			return "", false
		}
	}
	return body[codeStart : codeStart+codeEnd], true
}

// formatExpandedComment renders a comment's full body word-wrapped to the given width.
// Uses double-line box-drawing characters (╔═║╚) to visually distinguish from collapsed comments.
// For suggestion comments with original code available, renders a diff preview instead
// of the raw suggestion block.
func formatExpandedComment(c *types.ReviewComment, width int, originalCode string, wrap bool) string {
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

	// For suggestions with original code, render a diff preview
	suggestionCode, hasSuggestion := extractSuggestionCode(c.Body)
	if hasSuggestion && originalCode != "" {
		// Render any text before the suggestion block
		body := c.Body
		fenceStart := strings.Index(body, "```suggestion")
		if fenceStart > 0 {
			before := strings.TrimSpace(body[:fenceStart])
			if before != "" {
				for _, paragraph := range strings.Split(before, "\n") {
					if paragraph == "" {
						lines = append(lines, fmt.Sprintf("  %s", prefix))
						continue
					}
					wrapped := wrapContent(paragraph, bodyWidth)
					for _, w := range wrapped {
						lines = append(lines, fmt.Sprintf("  %s %s", prefix, w))
					}
				}
				lines = append(lines, fmt.Sprintf("  %s", prefix))
			}
		}

		// Render removed lines (original code)
		for _, origLine := range strings.Split(originalCode, "\n") {
			if !wrap {
				lines = append(lines, fmt.Sprintf("  %s - %s", prefix, origLine))
			} else {
				diffCodeWidth := bodyWidth - 2
				if diffCodeWidth < 10 {
					diffCodeWidth = 10
				}
				wrapped := wrapContent(origLine, diffCodeWidth)
				for j, w := range wrapped {
					if j == 0 {
						lines = append(lines, fmt.Sprintf("  %s - %s", prefix, w))
					} else {
						lines = append(lines, fmt.Sprintf("  %s   %s", prefix, w))
					}
				}
			}
		}
		// Render added lines (suggestion code)
		for _, sugLine := range strings.Split(suggestionCode, "\n") {
			if !wrap {
				lines = append(lines, fmt.Sprintf("  %s + %s", prefix, sugLine))
			} else {
				diffCodeWidth := bodyWidth - 2
				if diffCodeWidth < 10 {
					diffCodeWidth = 10
				}
				wrapped := wrapContent(sugLine, diffCodeWidth)
				for j, w := range wrapped {
					if j == 0 {
						lines = append(lines, fmt.Sprintf("  %s + %s", prefix, w))
					} else {
						lines = append(lines, fmt.Sprintf("  %s   %s", prefix, w))
					}
				}
			}
		}

		// Render any text after the suggestion block
		fenceEnd := strings.Index(body[fenceStart:], "\n```")
		if fenceEnd >= 0 {
			afterStart := fenceStart + fenceEnd + 4 // skip "\n```"
			if afterStart < len(body) {
				after := strings.TrimSpace(body[afterStart:])
				if after != "" {
					lines = append(lines, fmt.Sprintf("  %s", prefix))
					for _, paragraph := range strings.Split(after, "\n") {
						if paragraph == "" {
							lines = append(lines, fmt.Sprintf("  %s", prefix))
							continue
						}
						wrapped := wrapContent(paragraph, bodyWidth)
						for _, w := range wrapped {
							lines = append(lines, fmt.Sprintf("  %s %s", prefix, w))
						}
					}
				}
			}
		}
	} else {
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
	}

	// Drop trailing empty body lines (a bare prefix with no content) so a comment
	// body that ends in a newline or blank paragraph doesn't render a stray blank
	// line between the last line of text and the box footer.
	emptyBody := fmt.Sprintf("  %s", prefix)
	for len(lines) > 1 && strings.TrimRight(lines[len(lines)-1], " ") == emptyBody {
		lines = lines[:len(lines)-1]
	}

	lines = append(lines, fmt.Sprintf("  ╚═══%s", strings.Repeat("═", footerDashes)))
	return strings.Join(lines, "\n")
}
