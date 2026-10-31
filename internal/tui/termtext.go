package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// termText is s as the TUI may draw it where s is the change's own text: a
// path, a header, a lockfile, a tour note. Such text can carry what a terminal
// obeys rather than shows: escape sequences and control bytes (clear the
// screen, set the clipboard with OSC 52, redraw what the reviewer reads), and
// bidi controls, which reorder what it draws so a line reads other than it is
// (Trojan Source, CVE-2021-42574). ANSI sequences go whole; every other
// control character (C0, DEL, C1) and every bidi control (U+061C, U+200E/F,
// U+202A-202E, U+2066-2069) goes, a tab as a space; invalid UTF-8 becomes
// U+FFFD. Printable text, a zero-width joiner among it, stays.
//
// A diff's lines are not drawn through it: they show a control byte as a
// glyph instead (visibleControls), keeping its column.
func termText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.Is(unicode.Cc, r), unicode.Is(unicode.Bidi_Control, r):
			return -1
		}
		return r
	}, ansi.Strip(strings.ToValidUTF8(s, "\uFFFD")))
}
