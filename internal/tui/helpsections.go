package tui

import (
	"sort"
	"strconv"
	"strings"
)

// Ordering methodology for every keybinding surface — the H modal, README and
// the docs reference — so a binding can be found two ways.
//
// BY TASK: sections answer "I want to do X, what's the key". Each section is a
// verb, and a binding belongs to exactly one. Sections run in the order you
// reach for them: move, jump, change the view, comment, act on the review, hand
// something to another program, commands, the app itself.
//
// The old split was Navigation / Review / General, which failed both ways.
// Navigation had accumulated the wrap toggle, the sidebar toggle, the tree
// mode, the base ref and — the case that prompted this — `o`, which opens an
// annotation's docs. Nobody looking for "open the docs" scans Navigation.
// General was whatever was left.
//
// BY KEY: within a section, rows sort by their primary key under keyRank
// below, so a reader who knows the key can scan to it. The rule is mechanical
// and TestHelpSectionsAreOrderedAndComplete enforces it, which is what keeps the next binding
// from being appended wherever it happened to be written.

// helpRow is one line of the help modal: the key label as rendered, and what
// it does.
type helpRow struct{ key, desc string }

// Section titles. Shared with the docs so all three surfaces group identically.
const (
	secMove    = "Move"
	secJump    = "Jump"
	secView    = "View & panes"
	secComment = "Comment"
	secReview  = "Review"
	secOpen    = "Open elsewhere"
	secCommand = "Commands"
	secApp     = "App"
	secEditor  = "Text editing (comment/submit modals)"
)

// helpSectionOrder is the render order, and the order the docs use.
var helpSectionOrder = []string{
	secMove, secJump, secView, secComment, secReview, secOpen, secCommand, secApp, secEditor,
}

// helpUnsorted names sections that keep their written order instead of the key
// order. The text editor is a cheat sheet for a modal you are already typing
// in — you look up "how do I delete a word", never "what does alt+d do" — and
// its labels lead with a mix of named and modified keys (home, ctrl+a,
// shift+enter), which key-sorting interleaves into nonsense. Movement, then
// deletion, then the rest reads far better.
var helpUnsorted = map[string]bool{secEditor: true}

// orderHelpRows applies the section's ordering policy.
func orderHelpRows(section string, rows []helpRow) []helpRow {
	if helpUnsorted[section] {
		return rows
	}
	return sortHelpRows(rows)
}

// keyClass buckets a key label so related keys sort together and modified keys
// don't interleave with plain ones. Lower sorts first.
func keyClass(k string) int {
	switch {
	case k == "":
		return 9
	case strings.HasPrefix(k, ":"):
		return 6 // command-mode names
	case strings.ContainsAny(k, "+") || len(k) > 1:
		return 5 // ctrl+/alt+/shift+ and named keys (tab, enter, space, esc)
	case k[0] >= 'a' && k[0] <= 'z':
		return 0
	case k[0] >= 'A' && k[0] <= 'Z':
		return 1
	case k[0] >= '0' && k[0] <= '9':
		return 2
	default:
		return 3 // punctuation
	}
}

// primaryKey extracts the key a row sorts on: the first of a "j/k" or
// "S / :submit" pair, with any trailing prose dropped.
func primaryKey(label string) string {
	label = strings.TrimSpace(label)
	for _, sep := range []string{"/", " "} {
		if i := strings.Index(label, sep); i > 0 {
			label = label[:i]
		}
	}
	return strings.TrimSpace(label)
}

// keyRank is the sort key for a help row: class first, then the key itself
// case-insensitively so `c` and `C` stay adjacent rather than landing in
// different halves of the section.
//
// Modified keys sort by their BASE key, so ctrl+t and ctrl+shift+t land next to
// each other instead of being split apart by the "s" in "shift" — the takeover
// variants only make sense read beside the thing they vary.
// Among keys sharing a base, the plainer one comes first — ctrl+t before
// ctrl+shift+t — so the variant reads as a modification of the row above it.
// Sorting on the raw string instead would order them by whether the modifier
// name happens to precede the base letter, which is arbitrary: it puts ctrl+g
// before ctrl+shift+g but ctrl+shift+t before ctrl+t.
func keyRank(label string) (int, string, string) {
	k := primaryKey(label)
	class := keyClass(k)
	base := k
	if class == 5 {
		if i := strings.LastIndex(k, "+"); i >= 0 {
			base = k[i+1:]
		}
	}
	return class, strings.ToLower(base), strconv.Itoa(strings.Count(k, "+")) + "|" + k
}

// sortHelpRows puts a section into the documented order. Stable, so two rows
// on the same key (`/` searches the diff and filters the sidebar) keep the
// order they were written in.
func sortHelpRows(rows []helpRow) []helpRow {
	out := append([]helpRow(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		ci, li, ri := keyRank(out[i].key)
		cj, lj, rj := keyRank(out[j].key)
		if ci != cj {
			return ci < cj
		}
		if li != lj {
			return li < lj
		}
		return ri < rj // lowercase before uppercase within a letter
	})
	return out
}

// helpRowsInOrder reports whether rows are already in the documented order.
func helpRowsInOrder(rows []helpRow) bool {
	sorted := sortHelpRows(rows)
	for i := range rows {
		if rows[i] != sorted[i] {
			return false
		}
	}
	return true
}
