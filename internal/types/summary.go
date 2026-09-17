package types

import "strings"

// A review lands as a pile of diffs with no account of what it was for. The
// agent knows — it just fixed three separate things — but that structure is lost
// by the time the reviewer opens it, and has to be reconstructed by reading.
//
// SummaryItem is one line of that account: a short statement of something the
// round fixed, plus the parts of the diff it accounts for. Given those, the
// reviewer can read the change one intention at a time instead of one file at a
// time.
type SummaryItem struct {
	ID   string // stable across rounds when the agent supplies one
	Text string // a short line: what was fixed, not how
	// Order is the agent's reading order, lowest first. Ties fall back to the
	// order the items arrived in.
	Order   int
	Targets []SummaryTarget
}

// SummaryTarget is one region an item accounts for: a file, optionally narrowed
// to a range of its new-file lines. A target with no range claims the whole
// file, which is the right default for a change that is the file.
type SummaryTarget struct {
	Path      string
	LineStart int
	LineEnd   int
}

// WholeFile reports whether the target claims its file entirely rather than a
// range within it.
func (t SummaryTarget) WholeFile() bool { return t.LineStart <= 0 }

// Covers reports whether the target claims a given new-file line. A whole-file
// target covers every line, including the zero line used for file-level rows.
func (t SummaryTarget) Covers(path string, line int) bool {
	if t.Path != path {
		return false
	}
	if t.WholeFile() {
		return true
	}
	end := t.LineEnd
	if end < t.LineStart {
		end = t.LineStart
	}
	return line >= t.LineStart && line <= end
}

// ClaimsFile reports whether the item says anything about a file at all. The
// sidebar uses it to fade files the selected item has no stake in, which is a
// weaker question than whether a particular line is covered.
func (s SummaryItem) ClaimsFile(path string) bool {
	for _, t := range s.Targets {
		if t.Path == path {
			return true
		}
	}
	return false
}

// Covers reports whether the item accounts for a given line of a file.
func (s SummaryItem) Covers(path string, line int) bool {
	for _, t := range s.Targets {
		if t.Covers(path, line) {
			return true
		}
	}
	return false
}

// NormalizeSummaryItems puts a set of items into display order and fills in what
// the agent left out, so the rest of the system can assume both. Items with no
// text are dropped: an unlabelled entry is a colour with nothing to say.
func NormalizeSummaryItems(items []SummaryItem) []SummaryItem {
	out := make([]SummaryItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, it := range items {
		it.Text = strings.TrimSpace(it.Text)
		if it.Text == "" {
			continue
		}
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" || seen[it.ID] {
			it.ID = summaryFallbackID(i)
		}
		seen[it.ID] = true
		targets := make([]SummaryTarget, 0, len(it.Targets))
		for _, t := range it.Targets {
			t.Path = strings.TrimSpace(t.Path)
			if t.Path == "" {
				continue
			}
			if t.LineEnd < t.LineStart {
				t.LineEnd = t.LineStart
			}
			targets = append(targets, t)
		}
		it.Targets = targets
		out = append(out, it)
	}
	// Stable by Order, then arrival — a sort that reorders equal items would make
	// the colour assigned to an item change between rounds for no reason.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Order < out[j-1].Order; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func summaryFallbackID(i int) string {
	return "item-" + itoa(i+1)
}

// itoa avoids pulling strconv into the domain package for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for n > 0 {
		p--
		b[p] = byte('0' + n%10)
		n /= 10
	}
	return string(b[p:])
}
