package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// bandCells reads a rendered row cell by cell and counts the cells and those
// drawn on the accent background (SGR 46, or 48;5;6).
func bandCells(raw string) (cells, onAccent int) {
	accent := false
	for i := 0; i < len(raw); {
		if raw[i] == 0x1b && i+1 < len(raw) && raw[i+1] == '[' {
			j := i + 2
			for j < len(raw) && (raw[j] < 0x40 || raw[j] > 0x7e) {
				j++
			}
			if j < len(raw) && raw[j] == 'm' {
				params := strings.Split(raw[i+2:j], ";")
				for k := 0; k < len(params); k++ {
					switch p := params[k]; {
					case p == "" || p == "0" || p == "49":
						accent = false
					case p == "46":
						accent = true
					case p == "48" && k+2 < len(params) && params[k+1] == "5":
						accent = params[k+2] == "6"
						k += 2
					case p == "48" && k+4 < len(params) && params[k+1] == "2":
						accent = false
						k += 4
					case (p == "38" || p == "58") && k+1 < len(params) && params[k+1] == "5":
						k += 2
					case (p == "38" || p == "58") && k+1 < len(params) && params[k+1] == "2":
						k += 4
					}
				}
			}
			i = j + 1
			continue
		}
		r := []rune(raw[i:])[0]
		w := ansi.StringWidth(string(r))
		cells += w
		if accent {
			onAccent += w
		}
		i += len(string(r))
	}
	return cells, onAccent
}

// The note's header is the stop's id and title in the tour's accent, bold, on
// no background, with its scroll hint at the right end, and a rule under it
// (John, 2026-10-29, after a solid band: "Switch it back to the way we had it,
// with the coloring and the horizontal bar beneath").
func TestTheNoteHeaderIsAccentTextOverARule(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		th := DefaultTheme()
		d := docPaneModel{width: 80, height: 8, theme: &th}
		d.openNote("tour:1.2", "1.2 · Where it lands", strings.Repeat("A line of the note.\n", 20), nil)
		d.titleMark = stopVisitMark(fresh)
		d.titleLoc = noteLocation{path: "server/api/write.ts", lines: "5–9"}
		rows := strings.Split(d.View(), "\n")
		if cells, onAccent := bandCells(rows[0]); cells != 80 || onAccent != 0 {
			t.Errorf("fresh %v: the header is %d cells, %d of them on the accent: want 80, none (%q)", fresh, cells, onAccent, rows[0])
		}
		title := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor)).Bold(true).Render(" 1.2 · Where it lands")
		if !strings.HasPrefix(rows[0], title) {
			t.Errorf("fresh %v: the header does not start with the title in bold accent: %q", fresh, rows[0])
		}
		if head := ansi.Strip(rows[0]); !strings.HasSuffix(head, "more lines ") {
			t.Errorf("fresh %v: the header %q does not end with its scroll hint", fresh, head)
		}
		if rule := ansi.Strip(rows[1]); rule != strings.Repeat("─", 80) {
			t.Errorf("fresh %v: the row under the header is %q, want a rule across the pane", fresh, rule)
		}
	}
}

// A note longer than its pane says so where the reading ends: its last row is
// a marker — how many lines are below, and which lines are showing of how
// many — that reads "end of note" once scrolled to the end. The header says how
// much is above and below. A note that fits has neither.
func TestALongNoteSaysThereIsMore(t *testing.T) {
	m := longNoteApp(t)
	lay := computePaneLayout(&m)
	last := lay.doc.y + lay.doc.h - 1 // the pane's last row
	screen := screenText(m)
	lines := strings.Split(screen, "\n")
	if marker := lines[last]; !strings.Contains(marker, "↓ ") || !strings.Contains(marker, "more lines") || !strings.Contains(marker, "lines 1–") || !strings.Contains(marker, " of 60") {
		t.Errorf("the note's last row is %q, want the more-lines marker", marker)
	}
	if band := rowWith(screen, "1.2 · Where it lands"); !strings.Contains(band, "more lines") {
		t.Errorf("the header does not say there is more: %q", band)
	}
	nx, ny := onScreen(t, m, "Note line 2.")
	for i := 0; i < 70; i++ {
		m = updateApp(t, m, wheel(nx, ny, true))
	}
	lines = strings.Split(screenText(m), "\n")
	if marker := lines[last]; !strings.Contains(marker, "end of note") || !strings.Contains(marker, "–60 of 60") {
		t.Errorf("scrolled to the end the marker is %q", marker)
	}
	if !strings.Contains(lines[last-1], "Note line 60.") {
		t.Errorf("the note's last line should sit just over the marker: %q", lines[last-1])
	}

	short := viewsAppIn(t, &types.Config{}, 140, false)
	if s := screenText(short); strings.Contains(s, "more lines") || strings.Contains(s, "end of note") {
		t.Errorf("a note that fits shows a scroll marker:\n%s", s)
	}
}

// The marker's arithmetic: the rows of text a pane has, the lines below them,
// and where the view is.
func TestNoteMarkerText(t *testing.T) {
	for _, c := range []struct {
		total, rows, offset int
		left, right         string
	}{
		{20, 8, 0, "↓ 12 more lines", "lines 1–8 of 20"},
		{20, 8, 5, "↓ 7 more lines", "lines 6–13 of 20"},
		{20, 8, 12, "end of note", "lines 13–20 of 20"},
		{9, 8, 0, "↓ 1 more line", "lines 1–8 of 9"},
	} {
		left, right := noteMarkerText(c.total, c.rows, c.offset)
		if left != c.left || right != c.right {
			t.Errorf("noteMarkerText(%d, %d, %d) = %q, %q; want %q, %q", c.total, c.rows, c.offset, left, right, c.left, c.right)
		}
	}
}
