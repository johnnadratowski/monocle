package tui

import "strings"

// truncateMiddle shortens s to max cells by removing the middle and keeping
// both ends: "internal/…/engine_impl.go", "pseudocode: …in the swap path".
//
// Dropping the head instead — the older behaviour — works for a file path,
// where the filename is the identifying end, and fails badly for an artifact
// title, where the subject is at the front. "…rounding in the swap path" tells
// you nothing about which artifact it is. Keeping both ends serves both, since
// what a long name has in the middle is usually the least distinguishing part:
// intermediate directories, or a preposition.
//
// The tail gets the larger share. An even split reads well for prose but costs
// a path its filename, and the filename is the half you cannot do without.
func truncateMiddle(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	avail := max - 1 // one cell for the ellipsis
	tail := avail * 6 / 10
	if tail < 1 {
		tail = 1
	}
	head := avail - tail
	if head < 1 {
		head = 1
		tail = avail - 1
	}
	// Prefer to cut at a path separator so the final segment stays whole. A
	// proportional split lands a character or two inside the filename, and
	// "internal/c…ngine_impl.go" is a strictly worse answer than
	// "internal/…engine_impl.go" at the very same width.
	if i := strings.LastIndex(s, "/"); i >= 0 {
		if seg := len([]rune(s[i+1:])); seg >= tail && seg <= avail-1 {
			tail, head = seg, avail-seg
		}
	}
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
