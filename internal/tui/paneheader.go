package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// withPathHeader rewrites the TOP border of a rendered pane box to embed the
// current file path plus the active view mode, left-anchored like
// "┌─ path/to/file [ALL] ──────┐".
//
// Both live in the top border rather than the bottom one so the answers to
// "what am I looking at" and "which mode is it in" sit together at the point the
// eye starts, instead of being split between the pane's foot and the status bar.
// The mode badge is what makes a non-default view (whole-file, split, raw)
// self-evident — previously full-file mode had no indicator at all.
//
// Returns the box unchanged when there's no label or it's too narrow.
func withPathHeader(box, label, mode string, borderColor color.Color) string {
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
	// Budget: corners(2) + "─ "(2) + " "(1) before the trailing dashes, plus at
	// least one trailing dash. The badge (and the space before it) competes with
	// the label for the same room.
	maxLabel := w - 6 - lipgloss.Width(badge)
	if badge != "" {
		maxLabel-- // space between label and badge
	}
	if maxLabel < 1 {
		// Too narrow for both: the mode is the smaller, more perishable fact, so
		// drop it and keep the path identifiable.
		badge = ""
		maxLabel = w - 6
		if maxLabel < 1 {
			return box
		}
	}

	lbl := truncateLabelLeft(label, maxLabel)

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	// Magenta + bold matches the badge treatment the status bar used, so the
	// mode reads as the same kind of information after the move.
	badgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)

	used := lipgloss.Width(lbl)
	rendered := pathStyle.Render(lbl)
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
