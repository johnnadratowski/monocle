package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// A generated file (types.GeneratedReason, or a tour's note) is not read line
// by line, but the reviewer needs to know what it is. Its diff opens folded to
// a card saying so, from the best source there is to the worst: the tour's
// note, the generator its header names, a lockfile's package changes, its
// line counts. One key (unfold_generated) shows the real diff, and folds it
// back. A jump to lines of the file — a stop, goto_line — shows those lines.
// It never counts as unreviewed.

// generatedBadge marks a generated file in the file list.
const generatedBadge = "⚙"

// generatedCard is what the folded diff of a generated file shows.
type generatedCard struct {
	path     string
	reason   string // a types.Generated* reason
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
		path: f.Path, reason: f.Generated, by: f.GeneratedBy, from: f.GeneratedFrom,
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

// generatedWhy is the card's reason, as the reviewer reads it.
func generatedWhy(reason string) string {
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
		return "its header says so"
	case types.GeneratedTour:
		return "the tour says so"
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

	add(" " + bold.Render(generatedBadge+" Generated file") + "  " + c.path)
	add(" " + faint.Render("Generated: "+generatedWhy(c.reason)+"."))
	add("")
	if c.note != "" {
		field("What it is", c.note)
	}
	if c.by != "" || c.from != "" {
		made := c.by
		switch {
		case made == "":
			made = "made from " + c.from
		case c.from != "":
			made += ", from " + c.from
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
			switch {
			case ch.From == "":
				row = "+ " + ch.Name + " " + ch.To
			case ch.To == "":
				row = "- " + ch.Name + " " + ch.From
			default:
				row = "↑ " + ch.Name + " " + ch.From + " → " + ch.To
			}
			add("  " + strings.Repeat(" ", 12) + ansi.Truncate(row, textW, ""))
		}
	}
	field("Lines", fmt.Sprintf("+%d -%d", c.adds, c.dels))
	if c.comments > 0 {
		field("Comments", fmt.Sprintf("%d on this file, in its diff", c.comments))
	}
	add("")
	status := "it never counts as unreviewed"
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
