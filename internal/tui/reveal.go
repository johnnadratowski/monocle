package tui

import (
	"fmt"
	"sort"

	"github.com/josephschmitt/monocle/internal/types"
)

// A compact diff shows each change with a few lines of context and hides the
// unchanged stretches between. When an agent sends the reviewer to lines in
// such a stretch — a goto_line, a highlight_range, a tour stop — those lines
// are revealed: that stretch, with a margin, joins the diff as context, the
// way expanding between hunks does on a code host, and the rest of the file
// stays compact. Revealed stretches stay revealed for the session.

// lineRange is a run of new-file lines, both ends included.
type lineRange struct{ start, end int }

// revealMargin is how many lines of context a revealed stretch gets on either
// side, as git's own default context.
const revealMargin = 3

// addRange adds r to ranges, merging any it overlaps or touches, sorted.
func addRange(ranges []lineRange, r lineRange) []lineRange {
	out := append(append([]lineRange(nil), ranges...), r)
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	merged := out[:0]
	for _, x := range out {
		if n := len(merged); n > 0 && x.start <= merged[n-1].end+1 {
			merged[n-1].end = max(merged[n-1].end, x.end)
			continue
		}
		merged = append(merged, x)
	}
	return merged
}

// revealHunks is the compact diff with the revealed stretches shown: the rows
// of the whole-file diff that are a change, context the compact diff already
// shows, or context in a revealed range, regrouped into hunks wherever rows
// run on. A hunk the compact diff had keeps its header.
func revealHunks(compact, full []types.DiffHunk, revealed []lineRange) []types.DiffHunk {
	if len(revealed) == 0 || len(full) == 0 {
		return compact
	}
	shown := map[int]bool{}
	headers := map[[2]int]string{}
	for _, h := range compact {
		headers[[2]int{h.OldStart, h.NewStart}] = h.Header
		for _, l := range h.Lines {
			if l.Kind != types.DiffLineRemoved && l.NewLineNum > 0 {
				shown[l.NewLineNum] = true
			}
		}
	}
	wanted := func(l types.DiffLine) bool {
		if l.Kind != types.DiffLineContext {
			return true // a change is always in the compact diff
		}
		if shown[l.NewLineNum] {
			return true
		}
		for _, r := range revealed {
			if l.NewLineNum >= r.start && l.NewLineNum <= r.end {
				return true
			}
		}
		return false
	}

	var out []types.DiffHunk
	for _, fh := range full {
		oldPos, newPos := fh.OldStart, fh.NewStart
		var cur *types.DiffHunk
		closeHunk := func() {
			if cur == nil {
				return
			}
			if h, ok := headers[[2]int{cur.OldStart, cur.NewStart}]; ok {
				cur.Header = h
			} else {
				cur.Header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", cur.OldStart, cur.OldCount, cur.NewStart, cur.NewCount)
			}
			out = append(out, *cur)
			cur = nil
		}
		for _, l := range fh.Lines {
			if !wanted(l) {
				closeHunk()
			} else {
				if cur == nil {
					cur = &types.DiffHunk{OldStart: oldPos, NewStart: newPos}
				}
				cur.Lines = append(cur.Lines, l)
				if l.Kind != types.DiffLineAdded {
					cur.OldCount++
				}
				if l.Kind != types.DiffLineRemoved {
					cur.NewCount++
				}
			}
			if l.Kind != types.DiffLineAdded {
				oldPos++
			}
			if l.Kind != types.DiffLineRemoved {
				newPos++
			}
		}
		closeHunk()
	}
	return out
}

// revealedFor is a copy of the stretches revealed in a file, safe to hand to a
// load running in the background.
func (m appModel) revealedFor(path string) []lineRange {
	return append([]lineRange(nil), m.revealed[path]...)
}

// reveal marks new-file lines start to end of a file of the review — with
// revealMargin either side — to be shown even where a compact diff would hide
// them, and reports whether the diff on screen needs reloading to show them:
// the file is the one on screen, compact, and does not show them all yet.
func (m *appModel) reveal(path string, start, end int) bool {
	if start < 1 || path == "" {
		return false
	}
	if end < start {
		end = start
	}
	if m.revealed == nil {
		m.revealed = map[string][]lineRange{}
	}
	m.revealed[path] = addRange(m.revealed[path], lineRange{max(start-revealMargin, 1), end + revealMargin})
	dv := m.diffView
	if dv.path != path || dv.fullFile || dv.style == diffStyleFile || dv.additionalFilePath != "" || dv.isViewingContentItem() {
		return false
	}
	for n := start; n <= end; n++ {
		if dv.indexForNewLine(n) < 0 {
			return true
		}
	}
	return false
}
