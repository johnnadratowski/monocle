package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A stop note's markdown links are OSC 8 hyperlinks, underlined, so a click in
// the terminal opens them (John 2026-10-22: stop-note links; stage:// focuses a
// view). Only http, https and stage: anything else stays as written.
func TestNoteLinksAreHyperlinks(t *testing.T) {
	ms := newMarkdownStyler(DefaultTheme())
	out := ms.styleInline("see [the flow](stage://view/view#retry) and [docs](https://example.com/a)")
	for _, url := range []string{"stage://view/view#retry", "https://example.com/a"} {
		if !strings.Contains(out, ansi.SetHyperlink(url)) {
			t.Errorf("no OSC 8 for %s in %q", url, out)
		}
	}
	if strings.Count(out, ansi.ResetHyperlink()) != 2 {
		t.Errorf("each link closes after its text: %q", out)
	}
	if plain := ansi.Strip(out); plain != "see the flow and docs" {
		t.Errorf("the text reads %q, want the link texts alone", plain)
	}
	if !strings.Contains(out, "\x1b[4") && !strings.Contains(out, ";4m") && !strings.Contains(out, ";4;") {
		t.Errorf("a link is underlined: %q", out)
	}
	for _, bad := range []string{"[x](javascript:alert(1))", "[x](file:///etc/passwd)", "[x](x-apple.systempreferences:)", "[x](relative/path)"} {
		got := ms.styleInline(bad)
		if strings.Contains(got, "\x1b]8;") {
			t.Errorf("%s became a hyperlink: %q", bad, got)
		}
	}
	if got := ms.styleInline("`[not](https://a.b)` code"); strings.Contains(got, "\x1b]8;") {
		t.Errorf("a link inside inline code stays code: %q", got)
	}
}

// A link that wraps is closed at its row's end and opened again on the next, so
// the box's border and padding never become part of it.
func TestAWrappedLinkIsWholeOnEachRow(t *testing.T) {
	src := "Read [the long description of the retry policy that wraps](https://example.com/retry) please."
	rows := wrapNote(src, 30, newMarkdownStyler(DefaultTheme()))
	if len(rows) < 2 {
		t.Fatalf("expected the link to wrap: %q", rows)
	}
	open := ansi.SetHyperlink("https://example.com/retry")
	for i, row := range rows {
		opens, closes := strings.Count(row, open), strings.Count(row, ansi.ResetHyperlink())
		if opens != closes {
			t.Errorf("row %d opens %d, closes %d: %q", i, opens, closes, row)
		}
	}
	if !strings.Contains(rows[1], open) {
		t.Errorf("the second row opens the link again: %q", rows[1])
	}
}

// :link opens the doc pane note's links by number, and the OpenLink key the
// first; only http, https and stage:// ever reach `open`.
func TestLinkCommandOpensANoteLink(t *testing.T) {
	var opened []string
	was := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	defer func() { openURL = was }()

	m := longNoteApp(t)
	m.docPane.noteSource = "a [one](https://example.com/1), [bad](file:///x) and [two](stage://view/regions#r2)"
	m = typeCommand(t, m, "link")
	m = typeCommand(t, m, "link 2")
	m = typeCommand(t, m, "link 3")
	if strings.Join(opened, " ") != "https://example.com/1 stage://view/regions#r2" {
		t.Errorf("opened %v, want link 1, then link 2 (the file: one is not a link)", opened)
	}
	if !strings.Contains(m.statusBar.searchInfo, "no link 3") {
		t.Errorf("a link that is not there says so: %q", m.statusBar.searchInfo)
	}
	opened = nil
	m.setFocus(focusDoc)
	m = pressKey(t, m, "V")
	if strings.Join(opened, " ") != "https://example.com/1" {
		t.Errorf("V in the doc pane opened %v, want its first link", opened)
	}
}
