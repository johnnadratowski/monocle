package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// minHeaderLabel is the width the file path is allowed to keep before the
// optional chips (age, then mode) are dropped to make room for it. Enough to
// show a filename plus a little of its parent directory.
const minHeaderLabel = 16

// withPathHeader rewrites the TOP border of a rendered pane box to embed the
// current file path, how long ago it changed, and the active view mode,
// left-anchored like "┌─ path/to/file · 5m [ALL] ──────┐".
//
// These live in the top border rather than the bottom one so the answers to
// "what am I looking at", "is it current", and "which mode is it in" sit
// together at the point the eye starts, instead of being split between the
// pane's foot and the status bar. The mode badge is what makes a non-default
// view (whole-file, split, raw) self-evident — previously full-file mode had no
// indicator at all — and the age is what distinguishes an artifact the agent
// just sent from one left over from an earlier round.
//
// When the pane is too narrow for everything, the age is dropped first and the
// mode second: the path is the fact that identifies what you're reading, so it
// is the last thing surrendered.
//
// Returns the box unchanged when there's no label or it's too narrow.
func withPathHeader(box, label, age, mode string, borderColor color.Color) string {
	if label == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	if len(lines) < 3 {
		return box // too short to have a distinct top border
	}
	w := lipgloss.Width(lines[0]) // outer width, including both corners

	badge := ""
	if mode != "" {
		badge = "[" + mode + "]"
	}
	ageChip := ""
	if age != "" {
		ageChip = "· " + age
	}

	// Budget: corners(2) + "─ "(2) + " "(1) before the trailing dashes, plus at
	// least one trailing dash. Every extra chip (and its leading space) competes
	// with the label for the same room, so shed them in priority order until the
	// path has room to stay readable.
	//
	// The test is "does the path still fit legibly", not merely "does anything
	// fit" — otherwise the chips survive by squeezing the path down to an ellipsis
	// and a couple of characters, which defeats the point of showing it.
	var maxLabel int
	for {
		maxLabel = w - 6
		if ageChip != "" {
			maxLabel -= 1 + lipgloss.Width(ageChip)
		}
		if badge != "" {
			maxLabel -= 1 + lipgloss.Width(badge)
		}
		if maxLabel >= minHeaderLabel || maxLabel >= lipgloss.Width(label) {
			break
		}
		// Shed the lowest-priority chip and re-measure.
		if ageChip != "" {
			ageChip = ""
			continue
		}
		if badge != "" {
			badge = ""
			continue
		}
		break // nothing left to shed; the path takes whatever remains
	}
	if maxLabel < 1 {
		return box
	}

	lbl := truncateLabelLeft(label, maxLabel)

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	// Faint: the age is context for the path, not a peer of it.
	ageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	// Magenta + bold matches the badge treatment the status bar used, so the
	// mode reads as the same kind of information after the move.
	badgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)

	used := lipgloss.Width(lbl)
	rendered := pathStyle.Render(lbl)
	if ageChip != "" {
		used += 1 + lipgloss.Width(ageChip)
		rendered += " " + ageStyle.Render(ageChip)
	}
	if badge != "" {
		used += 1 + lipgloss.Width(badge)
		rendered += " " + badgeStyle.Render(badge)
	}
	fill := w - 5 - used
	if fill < 1 {
		fill = 1
	}

	lines[0] = borderStyle.Render("┌─ ") +
		rendered +
		borderStyle.Render(" "+strings.Repeat("─", fill)+"┐")
	return strings.Join(lines, "\n")
}

// truncateLabelLeft keeps the tail of s (the filename is most identifying),
// prefixing "…" when it doesn't fit in max cells.
func truncateLabelLeft(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[len(r)-max:])
	}
	return "…" + string(r[len(r)-(max-1):])
}
