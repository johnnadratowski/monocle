package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// A generated file (types.JudgeGenerated) is not read line by line, but the
// reviewer needs to know what it is. Its diff opens folded to a card saying
// so, from the best source there is to the worst: the tour's note, the
// generator its header names, a lockfile's package changes, a new file's first
// lines, its line counts. One key (unfold_generated) shows the real diff, and
// folds it back. A jump to lines of the file — a stop, goto_line — shows those
// lines. It counts as unreviewed until the reviewer marks it (r): the card is
// there to make that quick, not to take the file out of review.
//
// Everything on the card but its labels is the change's own text (a header, a
// lockfile, a path, a tour note), so the card strips escape sequences and
// control bytes, and bidi controls, from it before drawing (termText).

// generatedBadge marks a generated file in the file list.
const generatedBadge = "⚙"

// generatedCard is what the folded diff of a generated file shows.
type generatedCard struct {
	path     string
	reason   string // a types.Generated* reason
	own      bool   // classified only by a signal the change introduced
	new      bool   // the change adds the file
	head     string // a new file's first lines
	note     string // the tour's line for the file
	by, from string // what made it and from what, as its header says
	changes  []types.PackageChange
	adds     int
	dels     int
	comments int  // comments on the file, which the card hides
	reviewed bool // set at render time from the file list
	unfold   string
	review   string
}

// maxCardPackages is how many package changes the card lists; the rest are
// counted.
const maxCardPackages = 12

// changedFileFor is the review's changed file at path, as the engine judged it.
func (m appModel) changedFileFor(path string) (types.ChangedFile, bool) {
	if m.engine == nil || path == "" {
		return types.ChangedFile{}, false
	}
	for _, f := range m.engine.GetChangedFiles() {
		if f.Path == path {
			return f, true
		}
	}
	return types.ChangedFile{}, false
}

// syncGeneratedCard folds the diff on screen to its card when it is a
// generated file the reviewer has not unfolded, and unfolds it otherwise.
func (m *appModel) syncGeneratedCard() {
	m.diffView.genCard = nil
	dv := m.diffView
	if dv.path == "" || dv.isViewingContentItem() || dv.additionalFilePath != "" {
		return
	}
	f, ok := m.changedFileFor(dv.path)
	if !ok || !f.IsGenerated() || m.genUnfolded[f.Path] {
		return
	}
	card := &generatedCard{
		path: f.Path, reason: f.Generated, own: f.GeneratedOwn, new: f.GeneratedNew, head: f.GeneratedHead,
		by: f.GeneratedBy, from: f.GeneratedFrom,
		adds: f.Additions, dels: f.Deletions, comments: len(dv.comments),
		changes: types.LockfileChanges(&types.DiffResult{Path: f.Path, Hunks: dv.hunks}),
		unfold:  Label(m.keys.UnfoldGenerated), review: Label(m.keys.Reviewed),
	}
	if m.tour.tour != nil {
		card.note = m.tour.tour.Generated[f.Path]
	}
	m.diffView.genCard = card
}

// unfoldGenerated shows the real diff of a generated file from now on,
// without touching what is on screen otherwise.
func (m *appModel) unfoldGenerated(path string) {
	if m.genUnfolded == nil {
		m.genUnfolded = map[string]bool{}
	}
	m.genUnfolded[path] = true
	if m.diffView.genCard != nil && m.diffView.genCard.path == path {
		m.diffView.genCard = nil
	}
}

// toggleGeneratedFold is unfold_generated: the card on screen unfolds to the
// diff, and a generated file's diff folds back to its card.
func (m *appModel) toggleGeneratedFold() {
	path := m.diffView.path
	f, ok := m.changedFileFor(path)
	if !ok || !f.IsGenerated() || m.diffView.isViewingContentItem() || m.diffView.additionalFilePath != "" {
		m.statusBar.searchInfo = "This file is not generated: there is nothing to fold"
		return
	}
	if m.diffView.genCard != nil {
		m.unfoldGenerated(path)
		m.statusBar.searchInfo = fmt.Sprintf("Showing the generated diff (%s folds it again)", Label(m.keys.UnfoldGenerated))
		return
	}
	delete(m.genUnfolded, path)
	m.syncGeneratedCard()
	m.statusBar.searchInfo = ""
}

// generatedWhy is the card's reason, as the reviewer reads it. own: the only
// signal is one the change itself introduced, and the card says so.
func generatedWhy(reason string, own bool) string {
	if own {
		what := "name"
		if reason == types.GeneratedHeader {
			what = "header"
		}
		return "classified by this PR's own " + what + "; nothing at the base says so"
	}
	switch reason {
	case types.GeneratedByAttr:
		return ".gitattributes marks it linguist-generated"
	case types.GeneratedLockfile:
		return "a lockfile"
	case types.GeneratedName:
		return "its name says so (.gen. or .generated.)"
	case types.GeneratedCodegen:
		return "a codegen output"
	case types.GeneratedHeader:
		return "its header says so, as at the base"
	}
	return reason
}

// renderGeneratedCard is the folded diff: what the file is, then the key that
// shows the diff.
func (c generatedCard) render(width, height int) string {
	if width <= 0 {
		return ""
	}
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")).Width(12)
	textW := max(width-14, 20)
	wrap := lipgloss.NewStyle().Width(textW)

	var lines []string
	add := func(s string) { lines = append(lines, s) }
	field := func(name, value string) {
		for i, ln := range strings.Split(wrap.Render(value), "\n") {
			if i == 0 {
				add("  " + label.Render(name) + ln)
			} else {
				add("  " + strings.Repeat(" ", 12) + ln)
			}
		}
	}

	note, by, from := termText(c.note), termText(c.by), termText(c.from)
	add(" " + bold.Render(generatedBadge+" Generated file") + "  " + termText(c.path))
	add(" " + faint.Render("Generated: "+generatedWhy(c.reason, c.own)+"."))
	add("")
	if note != "" {
		field("What it is", note)
	}
	if by != "" || from != "" {
		made := by
		switch {
		case made == "":
			made = "made from " + from
		case from != "":
			made += ", from " + from
		}
		field("Made by", made)
	}
	if len(c.changes) > 0 {
		var added, removed, bumped int
		for _, ch := range c.changes {
			switch {
			case ch.From == "":
				added++
			case ch.To == "":
				removed++
			default:
				bumped++
			}
		}
		var counts []string
		for _, n := range []struct {
			n    int
			what string
		}{{added, "added"}, {removed, "removed"}, {bumped, "bumped"}} {
			if n.n > 0 {
				counts = append(counts, fmt.Sprintf("%d %s", n.n, n.what))
			}
		}
		field("Packages", strings.Join(counts, " · "))
		for i, ch := range c.changes {
			if i == maxCardPackages {
				add("  " + strings.Repeat(" ", 12) + faint.Render(fmt.Sprintf("… and %d more", len(c.changes)-maxCardPackages)))
				break
			}
			var row string
			name, was, is := termText(ch.Name), termText(ch.From), termText(ch.To)
			switch {
			case ch.From == "":
				row = "+ " + name + " " + is
			case ch.To == "":
				row = "- " + name + " " + was
			default:
				row = "↑ " + name + " " + was + " → " + is
			}
			add("  " + strings.Repeat(" ", 12) + ansi.Truncate(row, textW, ""))
		}
	}
	if c.new {
		field("New", "this change adds the file")
		for i, ln := range strings.Split(c.head, "\n") {
			if i == types.GeneratedHeaderLines || (i == 0 && ln == "") {
				break
			}
			ln = ansi.Truncate(termText(ln), textW, "…")
			if i == 0 {
				add("  " + label.Render("First lines") + faint.Render(ln))
			} else {
				add("  " + strings.Repeat(" ", 12) + faint.Render(ln))
			}
		}
	}
	field("Lines", fmt.Sprintf("+%d -%d", c.adds, c.dels))
	if c.comments > 0 {
		field("Comments", fmt.Sprintf("%d on this file, in its diff", c.comments))
	}
	add("")
	status := "not reviewed yet"
	if c.reviewed {
		status = "marked reviewed"
	}
	add(" " + faint.Render(fmt.Sprintf("%s shows the diff · %s marks it reviewed · %s", c.unfold, c.review, status)))
	if len(lines) > height && height > 0 {
		lines = lines[:height]
	}
	for i, ln := range lines {
		lines[i] = ansi.Truncate(ln, width, "")
	}
	return strings.Join(lines, "\n")
}
