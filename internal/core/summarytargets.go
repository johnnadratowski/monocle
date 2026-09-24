package core

import (
	"fmt"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// A summary target that hits nothing used to fail in the quietest way there is:
// the call succeeded, the item appeared in the modal, and selecting it showed an
// empty review. The agent had no way to tell a mistyped path from a fix that
// genuinely touched nothing.
//
// checkSummaryTargets names every target that matches nothing in the review. It
// reports rather than rejects: the agent is the authority on what it fixed, and
// a target can be legitimately ahead of the changeset — an agent that calls
// set_review_summary before its last add_files would otherwise have a correct
// summary refused.
func (e *Engine) checkSummaryTargets(session *types.ReviewSession, items []types.SummaryItem) []string {
	if session == nil {
		return nil
	}
	files := make(map[string]bool, len(session.ChangedFiles))
	for _, f := range session.ChangedFiles {
		files[f.Path] = true
	}
	// An added context file is a legitimate target with no diff of its own, so it
	// matches the path but can never match a line range.
	extra := make(map[string]bool, len(session.AdditionalFiles))
	for _, f := range session.AdditionalFiles {
		extra[f.Path] = true
	}
	artifacts := make(map[string]int, len(session.ContentItems))
	for _, c := range session.ContentItems {
		artifacts[c.ID] = strings.Count(c.Content, "\n") + 1
	}

	// One git call per distinct file, not per target: an item commonly names
	// several ranges in the same file.
	hunkLines := map[string][]types.DiffHunk{}
	hunksFor := func(path string) []types.DiffHunk {
		if h, ok := hunkLines[path]; ok {
			return h
		}
		var h []types.DiffHunk
		if d, err := e.GetFileDiff(path); err == nil && d != nil {
			h = d.Hunks
		}
		hunkLines[path] = h
		return h
	}

	var out []string
	for _, it := range items {
		for _, t := range it.Targets {
			if msg := summaryTargetProblem(t, files, extra, artifacts, hunksFor); msg != "" {
				out = append(out, fmt.Sprintf("%s: %s", it.ID, msg))
			}
		}
	}
	return out
}

// summaryTargetProblem returns a description of why a target matches nothing,
// or "" when it matches.
func summaryTargetProblem(
	t types.SummaryTarget,
	files, extra map[string]bool,
	artifacts map[string]int,
	hunksFor func(string) []types.DiffHunk,
) string {
	if t.IsArtifact() {
		lines, ok := artifacts[t.Artifact]
		if !ok {
			return fmt.Sprintf("artifact %q is not in this review", t.Artifact)
		}
		if !t.WholeFile() && t.LineStart > lines {
			return fmt.Sprintf("artifact %q has %d line(s), so %s is past its end",
				t.Artifact, lines, targetRange(t))
		}
		return ""
	}
	switch {
	case files[t.Path]:
	case extra[t.Path]:
		// Context files are shown whole, so a range means nothing there — but the
		// file itself is a real target and the item still points somewhere.
		return ""
	default:
		return fmt.Sprintf("%q is not a file in this review", t.Path)
	}
	if t.WholeFile() {
		return ""
	}
	if !rangeHitsAHunk(t, hunksFor(t.Path)) {
		return fmt.Sprintf("%q has no hunk covering %s — new-file line numbers, as in the diff's @@ header",
			t.Path, targetRange(t))
	}
	return ""
}

// rangeHitsAHunk reports whether a target's range overlaps any hunk's new-file
// span. Overlap rather than containment: an item may name a range that starts in
// context above the hunk, which is still a correct claim.
func rangeHitsAHunk(t types.SummaryTarget, hunks []types.DiffHunk) bool {
	// No diff at all (an unreadable or binary file) is not evidence the range is
	// wrong, so it passes rather than producing a misleading complaint.
	if len(hunks) == 0 {
		return true
	}
	end := t.LineEnd
	if end < t.LineStart {
		end = t.LineStart
	}
	for _, h := range hunks {
		count := h.NewCount
		if count < 1 {
			count = 1
		}
		if t.LineStart <= h.NewStart+count-1 && end >= h.NewStart {
			return true
		}
	}
	return false
}

func targetRange(t types.SummaryTarget) string {
	if t.LineEnd > t.LineStart {
		return fmt.Sprintf("lines %d-%d", t.LineStart, t.LineEnd)
	}
	return fmt.Sprintf("line %d", t.LineStart)
}
