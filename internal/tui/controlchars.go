package tui

import (
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// Control characters are the bytes a terminal acts on rather than draws. Left
// raw they are invisible at best — a NUL in a source file looks like nothing at
// all, so an edit that only adds or removes one shows as an identical pair of
// lines — and at worst an escape sequence in reviewed content drives the
// reviewer's terminal.

// visibleControls swaps each control byte for the Unicode Control Pictures glyph
// naming it — ␀ for NUL, ␛ for ESC, ␡ for DEL. One glyph per byte, so every
// column offset downstream (intra-line change spans, comment anchors, the
// cursor) still lines up with the content it came from.
func visibleControls(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0x7f }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case !types.IsControlByte(c):
			b.WriteByte(c)
		case c == 0x7f:
			b.WriteRune('␡') // ␡
		default:
			b.WriteRune(rune(0x2400 + int(c))) // ␀ … ␟
		}
	}
	return b.String()
}

// visibleControlHunks returns hunks with every line's content made visible. The
// input is left untouched — the engine's copy stays byte-faithful, so feedback
// quoting a line quotes what is actually in the file.
func visibleControlHunks(hunks []types.DiffHunk) []types.DiffHunk {
	out := make([]types.DiffHunk, len(hunks))
	copy(out, hunks)
	for i := range out {
		var lines []types.DiffLine
		for j, l := range out[i].Lines {
			clean := visibleControls(l.Content)
			if clean == l.Content {
				continue
			}
			if lines == nil {
				lines = make([]types.DiffLine, len(out[i].Lines))
				copy(lines, out[i].Lines)
			}
			lines[j].Content = clean
		}
		if lines != nil {
			out[i].Lines = lines
		}
	}
	return out
}

// binarySampleBytes bounds the scan, matching git's own first-few-bytes window.
const binarySampleBytes = 8000

// isBinaryContent reports whether diff content should be shown as a binary
// placeholder rather than as text. The judgement itself lives in types so the
// no-git path (which classifies files on disk) cannot answer differently.
func isBinaryContent(hunks []types.DiffHunk) bool {
	total, control := 0, 0
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			control += types.CountControlBytes([]byte(line.Content))
			total += len(line.Content)
			if total >= binarySampleBytes {
				return types.LooksBinary(control, total)
			}
		}
	}
	return types.LooksBinary(control, total)
}
