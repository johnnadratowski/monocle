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

// SummaryTarget is one region an item accounts for: a file or an artifact,
// optionally narrowed to a range of lines. A target with no range claims the
// whole thing, which is the right default for a change that IS the file.
//
// Artifacts are targetable because a round's most consequential change is often
// in one — a plan, a ruling the reviewer has to make — and an item that could
// only point at files left that part of the review with no item at all.
type SummaryTarget struct {
	// Path names a file in the repo. Artifact names a content item by its id.
	// Exactly one is set; Artifact wins if both somehow are.
	Path      string
	Artifact  string
	LineStart int
	LineEnd   int
}

// IsArtifact reports whether the target names an artifact rather than a file.
func (t SummaryTarget) IsArtifact() bool { return t.Artifact != "" }

// Key is what the target names, for messages and comparison.
func (t SummaryTarget) Key() string {
	if t.IsArtifact() {
		return t.Artifact
	}
	return t.Path
}

// CoversLine reports whether the target's range includes a line, ignoring what
// the target names. Callers that have already matched the file or artifact ask
// this; Covers is the combined question.
func (t SummaryTarget) CoversLine(line int) bool {
	if t.WholeFile() {
		return true
	}
	end := t.LineEnd
	if end < t.LineStart {
		end = t.LineStart
	}
	return line >= t.LineStart && line <= end
}

// WholeFile reports whether the target claims its file entirely rather than a
// range within it.
func (t SummaryTarget) WholeFile() bool { return t.LineStart <= 0 }

// Covers reports whether the target claims a given new-file line of a FILE. A
// whole-file target covers every line, including the zero line used for
// file-level rows. An artifact target never covers a file.
func (t SummaryTarget) Covers(path string, line int) bool {
	if t.IsArtifact() || t.Path != path {
		return false
	}
	return t.CoversLine(line)
}

// ClaimsFile reports whether the item says anything about a file at all. The
// sidebar uses it to fade files the selected item has no stake in, which is a
// weaker question than whether a particular line is covered.
func (s SummaryItem) ClaimsFile(path string) bool {
	for _, t := range s.Targets {
		if !t.IsArtifact() && t.Path == path {
			return true
		}
	}
	return false
}

// ClaimsArtifact reports whether the item says anything about an artifact. The
// sidebar fades artifacts the selected item has no stake in, exactly as it does
// for files.
func (s SummaryItem) ClaimsArtifact(id string) bool {
	for _, t := range s.Targets {
		if t.IsArtifact() && t.Artifact == id {
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

// The modal is a front page, not a document. An item that runs long crowds out
// the list it belongs to, and an overview that runs long puts an essay between
// the reviewer and the diff. Both are capped rather than refused: a summary that
// says slightly too much is still worth showing, trimmed.
const (
	SummaryTextLimit     = 120
	SummaryOverviewLimit = 500
)

// TrimSummaryText collapses whitespace and caps length, marking a cut with an
// ellipsis so a trimmed line cannot be mistaken for the whole of what was said.
func TrimSummaryText(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return strings.TrimRight(string(r[:limit-1]), " ") + "…"
}

// NormalizeSummaryItems puts a set of items into display order and fills in what
// the agent left out, so the rest of the system can assume both. Items with no
// text are dropped: an unlabelled entry is a colour with nothing to say.
func NormalizeSummaryItems(items []SummaryItem) []SummaryItem {
	out := make([]SummaryItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, it := range items {
		it.Text = TrimSummaryText(it.Text, SummaryTextLimit)
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
			t.Artifact = strings.TrimSpace(t.Artifact)
			// An artifact target carries no path, so requiring one would silently
			// drop every artifact claim.
			if t.Artifact != "" {
				t.Path = ""
			} else if t.Path == "" {
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
