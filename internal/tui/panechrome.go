package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// minHeaderLabel is the width the file path is allowed to keep before the
// optional chips (age, scroll, then mode) are dropped to make room for it.
// Enough to show a filename plus a little of its parent directory.
const minHeaderLabel = 16

// paneChrome is everything the diff pane embeds in its own borders: what you're
// looking at, how fresh it is, which mode it's in, and how much of it is off
// screen in each direction.
type paneChrome struct {
	label  string // file path or artifact title
	age    string // "5m", "3h" — "" when unknown
	mode   string // "ALL", "SPLIT", "v1→v2 DIFF" — "" for the default view
	extent scrollExtent
}

// withPaneChrome rewrites the TOP and BOTTOM borders of a rendered pane box to
// embed that information:
//
//	┌─ path/to/file · 5m [ALL] ───────── ↑ 128 lines · 3 chunks ─┐
//	│ …                                                          │
//	└──────────────────────────────────── ↓ 940 lines · 12 chunks ┘
//
// Identity lives in the top border rather than the bottom one so "what am I
// looking at", "is it current", and "which mode is it in" sit together at the
// point the eye starts, instead of being split between the pane's foot and the
// status bar. The mode badge is what makes a non-default view (whole-file,
// split, raw) self-evident, and the age is what distinguishes an artifact the
// agent just sent from one left over from an earlier round.
//
// The scroll chips sit on the edge they describe — above-content on the top
// border, below-content on the bottom — so the direction needs no decoding. A
// chip is absent exactly when that end of the document is already on screen,
// which makes "no chip" mean "nothing more that way".
//
// Returns the box unchanged when there's no label or it's too narrow.
func withPaneChrome(box string, c paneChrome, borderColor color.Color) string {
	lines := strings.Split(box, "\n")
	if len(lines) < 3 {
		return box // too short to have distinct top and bottom borders
	}
	w := lipgloss.Width(lines[0]) // outer width, including both corners

	lines[0] = topBorder(lines[0], w, c, borderColor)
	lines[len(lines)-1] = bottomBorder(lines[len(lines)-1], w, c.extent, borderColor)
	return strings.Join(lines, "\n")
}

// topBorder renders "┌─ label · age [MODE] ─── ↑ chip ─┐".
func topBorder(orig string, w int, c paneChrome, borderColor color.Color) string {
	if c.label == "" {
		return orig
	}

	age, mode := c.age, c.mode
	chipFull := scrollChip("↑", c.extent.linesAbove, c.extent.chunksAbove, false)
	chipShort := scrollChip("↑", c.extent.linesAbove, c.extent.chunksAbove, true)
	chip := chipFull

	// Budget: corners(2) + "─ "(2) + " "(1) before the trailing dashes, plus at
	// least one trailing dash. Every extra chip (and its separators) competes
	// with the label for the same room, so shed them in priority order until the
	// path has room to stay readable.
	//
	// The test is "does the path still fit legibly", not merely "does anything
	// fit" — otherwise the chips survive by squeezing the path down to an ellipsis
	// and a couple of characters, which defeats the point of showing it.
	//
	// Shed order: age (most static), then the scroll chip down to its short form
	// and away, then the mode badge. The path is the fact that identifies what
	// you're reading, so it is the last thing surrendered.
	shed := func() bool {
		switch {
		case age != "":
			age = ""
		case chip != "" && chip != chipShort:
			chip = chipShort
		case chip != "":
			chip = ""
		case mode != "":
			mode = ""
		default:
			return false
		}
		return true
	}

	var maxLabel int
	var ageChip, badge string
	for {
		ageChip, badge = "", ""
		if age != "" {
			ageChip = "· " + age
		}
		if mode != "" {
			badge = "[" + mode + "]"
		}
		maxLabel = w - 6
		if ageChip != "" {
			maxLabel -= 1 + lipgloss.Width(ageChip)
		}
		if badge != "" {
			maxLabel -= 1 + lipgloss.Width(badge)
		}
		if chip != "" {
			maxLabel -= 3 + lipgloss.Width(chip)
		}
		if maxLabel >= minHeaderLabel || maxLabel >= lipgloss.Width(c.label) {
			break
		}
		if !shed() {
			break // nothing left to shed; the path takes whatever remains
		}
	}
	if maxLabel < 1 {
		return orig
	}

	lbl := truncateLabelLeft(c.label, maxLabel)

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	used := lipgloss.Width(lbl)
	rendered := pathChipStyle().Render(lbl)
	if ageChip != "" {
		used += 1 + lipgloss.Width(ageChip)
		rendered += " " + ageChipStyle().Render(ageChip)
	}
	if badge != "" {
		used += 1 + lipgloss.Width(badge)
		rendered += " " + modeChipStyle().Render(badge)
	}

	if chip == "" {
		fill := w - 5 - used
		if fill < 1 {
			fill = 1
		}
		return borderStyle.Render("┌─ ") + rendered +
			borderStyle.Render(" "+strings.Repeat("─", fill)+"┐")
	}
	fill := w - 8 - used - lipgloss.Width(chip)
	if fill < 1 {
		fill = 1
	}
	return borderStyle.Render("┌─ ") + rendered +
		borderStyle.Render(" "+strings.Repeat("─", fill)+" ") +
		scrollChipStyle().Render(chip) +
		borderStyle.Render(" ─┐")
}

// bottomBorder renders "└──────── ↓ chip ─┘", right-anchored to mirror the top.
func bottomBorder(orig string, w int, e scrollExtent, borderColor color.Color) string {
	chip := scrollChip("↓", e.linesBelow, e.chunksBelow, false)
	if chip == "" {
		return orig
	}
	// "└"(1) + fill + " "(1) + chip + " ─┘"(3); keep at least one dash.
	if w-5-lipgloss.Width(chip) < 1 {
		chip = scrollChip("↓", e.linesBelow, e.chunksBelow, true)
		if chip == "" || w-5-lipgloss.Width(chip) < 1 {
			return orig
		}
	}
	fill := w - 5 - lipgloss.Width(chip)

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	return borderStyle.Render("└"+strings.Repeat("─", fill)+" ") +
		scrollChipStyle().Render(chip) +
		borderStyle.Render(" ─┘")
}

// scrollChip renders an off-screen indicator like "↑ 128 lines · 3 chunks",
// or "↑128 ·3" in its short form for a narrow pane. It returns "" when nothing
// is off screen in that direction: the absence of a chip is how "you are at
// that end" reads, which is quieter and less ambiguous than printing a zero.
// The chunk half is omitted when there are no change blocks that way, so an
// artifact or an unchanged stretch of file doesn't advertise "0 chunks".
func scrollChip(arrow string, lines, chunks int, short bool) string {
	if lines <= 0 {
		return ""
	}
	if short {
		if chunks > 0 {
			return fmt.Sprintf("%s%d ·%d", arrow, lines, chunks)
		}
		return fmt.Sprintf("%s%d", arrow, lines)
	}
	s := fmt.Sprintf("%s %d line%s", arrow, lines, plural(lines))
	if chunks > 0 {
		s += fmt.Sprintf(" · %d chunk%s", chunks, plural(chunks))
	}
	return s
}

func pathChipStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("7")) }

// Faint: the age and the scroll counts are context for the path, not peers of it.
func ageChipStyle() lipgloss.Style    { return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) }
func scrollChipStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) }

// Magenta + bold matches the badge treatment the status bar used, so the mode
// reads as the same kind of information after the move.
func modeChipStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
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
