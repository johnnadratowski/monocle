package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// withPathFooter rewrites the bottom border of a rendered pane box to embed a
// label (the current file path), left-anchored like "└─ path/to/file ──────┘".
// The path is shown so you can tell which file is open even with the sidebar
// hidden. Returns the box unchanged when there's no label or it's too narrow.
func withPathFooter(box, label string, borderColor color.Color) string {
	if label == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	if len(lines) < 3 {
		return box // too short to have a distinct bottom border
	}
	w := lipgloss.Width(lines[0]) // outer width, including both corners
	// Budget: corners(2) + "─ "(2) + " "(1) before the trailing dashes, plus at
	// least one trailing dash.
	maxLabel := w - 6
	if maxLabel < 1 {
		return box
	}

	lbl := truncateLabelLeft(label, maxLabel)
	fill := w - 5 - lipgloss.Width(lbl)
	if fill < 1 {
		fill = 1
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	lines[len(lines)-1] = borderStyle.Render("└─ ") +
		pathStyle.Render(lbl) +
		borderStyle.Render(" "+strings.Repeat("─", fill)+"┘")
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
