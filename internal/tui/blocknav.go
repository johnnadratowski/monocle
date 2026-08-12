package tui

import (
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// Code-structure navigation in the diff pane: vim's `%`, `[{` and `[[` motions,
// approximated without a parser.
//
// Vim's own bindings are `%` (matching pair), `[{` / `]}` (start / end of the
// enclosing brace block) and `[[` (previous top-level section). Only `%` is
// reproduced under its real key here: `[` and `]` already navigate diff chunks,
// so the two-key `[{` sequences are remapped onto `-` (up one level) and `_`
// (up to the outermost level), keeping the shift-escalates convention `g`/`G`
// already sets.
//
// Two mechanisms back these motions. Bracket matching is exact and used first;
// indentation is the fallback, which is what makes the motions work at all in
// Python, YAML, and markdown artifacts. Both read only the lines the pane has
// rendered, so in a compact diff a target inside a skipped region simply isn't
// found and the motion is a no-op — press `a` for the whole file to navigate
// structure that the hunks don't include.

// codeAt returns the source text of a rendered line, and whether the line is
// code at all. Hunk headers, comment and annotation boxes, and blank lines are
// not: they carry no structure and must not terminate a search.
//
// Only text present in the NEW file counts. A removed line's brackets and
// indentation describe the file as it was, and mixing the two sides makes
// bracket depth unbalanceable across an edited block.
func (m diffViewModel) codeAt(i int) (string, bool) {
	if i < 0 || i >= len(m.lines) {
		return "", false
	}
	line := m.lines[i]
	if line.isHunk || line.isComment || line.isAnnotation || line.verbatim {
		return "", false
	}
	if m.isHiddenComment(line) {
		return "", false
	}
	text := line.content
	if line.isSplit {
		if line.rightEmpty {
			return "", false // a pure deletion: nothing on the new-file side
		}
		text = line.rightContent
	} else if line.kind == types.DiffLineRemoved && line.newLineNum == 0 && line.rightLineNum == 0 {
		return "", false
	}
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

// indentAt returns a line's indent in visual columns, and whether it is code.
func (m diffViewModel) indentAt(i int) (int, bool) {
	text, ok := m.codeAt(i)
	if !ok {
		return 0, false
	}
	tab := m.tabSize
	if tab <= 0 {
		tab = 4
	}
	col := 0
	for _, r := range text {
		switch r {
		case '\t':
			col += tab - col%tab
		case ' ':
			col++
		default:
			return col, true
		}
	}
	return col, true
}

// isCloserLine reports whether a line begins with a closing delimiter. Such a
// line sits at its own header's indent rather than one level in, so indent
// comparisons have to treat it as part of the block it closes, not a sibling.
func (m diffViewModel) isCloserLine(i int) bool {
	text, ok := m.codeAt(i)
	if !ok {
		return false
	}
	switch strings.TrimSpace(text)[0] {
	case '}', ')', ']':
		return true
	}
	return false
}

// enclosingBlockStart returns the line that opens the block containing from —
// the nearest preceding code line indented less far. Returns -1 at the
// outermost level.
func (m diffViewModel) enclosingBlockStart(from int) int {
	ind, ok := m.indentAt(from)
	if !ok {
		return -1
	}
	// From a closing delimiter, the header shares this indent; from anything
	// else it must be strictly further out.
	want := ind - 1
	if m.isCloserLine(from) {
		want = ind
	}
	for i := from - 1; i >= 0; i-- {
		ci, ok := m.indentAt(i)
		if !ok {
			continue
		}
		// A sibling block's closing brace matches on indent but opens nothing.
		if m.isCloserLine(i) {
			continue
		}
		if ci <= want {
			return i
		}
	}
	return -1
}

// blockEnd returns the last line of the block opened at header — its final
// indented line, or the closing delimiter that follows at the header's own
// indent. Returns -1 when the line opens no block.
func (m diffViewModel) blockEnd(header int) int {
	base, ok := m.indentAt(header)
	if !ok {
		return -1
	}
	last := -1
	for i := header + 1; i < len(m.lines); i++ {
		ind, ok := m.indentAt(i)
		if !ok {
			continue // blanks and comment boxes don't end a block
		}
		if ind > base {
			last = i
			continue
		}
		// A brace language closes at the header's indent; an indentation
		// language just stops, and last already holds the body's final line.
		if ind == base && last >= 0 && m.isCloserLine(i) {
			return i
		}
		break
	}
	return last
}

// netBracketDepth counts openers minus closers in a line, ignoring delimiters
// inside string and rune literals and anything after a line comment. It is a
// scanner, not a parser: a bracket inside a multi-line raw string will be
// miscounted, which costs a failed jump and nothing else.
func netBracketDepth(s string) int {
	depth := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '"', '\'', '`':
			// Skip to the closing quote; an unterminated literal ends the line.
			for i++; i < len(runes); i++ {
				if runes[i] == '\\' {
					i++
					continue
				}
				if runes[i] == c {
					break
				}
			}
		case '#':
			return depth // shell / Python / YAML comment
		case '/':
			if i+1 < len(runes) && runes[i+1] == '/' {
				return depth
			}
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		}
	}
	return depth
}

// netDepthAt is netBracketDepth for a rendered line, and 0 for anything that
// isn't new-file code, so non-code lines pass through a depth walk untouched.
func (m diffViewModel) netDepthAt(i int) int {
	text, ok := m.codeAt(i)
	if !ok {
		return 0
	}
	return netBracketDepth(text)
}

// bracketMatch returns the line holding the delimiter that pairs with the one
// this line leaves open (or closes), or -1 when the line is balanced or the
// partner isn't among the rendered lines.
func (m diffViewModel) bracketMatch(from int) int {
	depth := m.netDepthAt(from)
	switch {
	case depth > 0:
		for i := from + 1; i < len(m.lines); i++ {
			if depth += m.netDepthAt(i); depth <= 0 {
				return i
			}
		}
	case depth < 0:
		for i := from - 1; i >= 0; i-- {
			if depth += m.netDepthAt(i); depth >= 0 {
				return i
			}
		}
	}
	return -1
}

// JumpToBlockMatch is `%`: from a block's opening line go to its end, from its
// end go back to the opening, and from anywhere inside go out to the opening —
// so repeated presses toggle between a block's two edges.
func (m *diffViewModel) JumpToBlockMatch() bool {
	if t := m.bracketMatch(m.cursor); t >= 0 {
		return m.landOnLine(t)
	}
	// Indentation fallback, for languages that delimit blocks by layout.
	if t := m.blockEnd(m.cursor); t > m.cursor {
		return m.landOnLine(t)
	}
	return m.landOnLine(m.enclosingBlockStart(m.cursor))
}

// JumpToEnclosingBlock is `-` (vim's `[{`): out one level, to the line that
// opens the block the cursor sits in.
func (m *diffViewModel) JumpToEnclosingBlock() bool {
	return m.landOnLine(m.enclosingBlockStart(m.cursor))
}

// JumpToTopLevelBlock is `_` (vim's `[[`): out as far as the structure goes, to
// the outermost line enclosing the cursor — the func or type declaration, from
// wherever inside it you happen to be.
func (m *diffViewModel) JumpToTopLevelBlock() bool {
	target := -1
	for cur := m.cursor; ; {
		t := m.enclosingBlockStart(cur)
		if t < 0 {
			break
		}
		target, cur = t, t // strictly decreasing, so this terminates
	}
	return m.landOnLine(target)
}

// landOnLine moves the cursor to i and scrolls it into view, reporting whether
// it actually moved. A target already on screen is left where it is rather than
// centred: a jump within the viewport shouldn't scroll the code out from under
// you, while a jump out of it should arrive with context on both sides.
func (m *diffViewModel) landOnLine(i int) bool {
	if i < 0 || i >= len(m.lines) {
		return false
	}
	if !m.isSelectable(i) {
		if i = m.nearestSelectable(i, -1); !m.isSelectable(i) {
			return false
		}
	}
	if i == m.cursor {
		return false
	}
	visible := i >= m.offset && i <= m.lastVisibleLine()
	m.cursor = i
	if visible {
		m.ensureVisible()
	} else {
		m.centerCursor()
	}
	return true
}
