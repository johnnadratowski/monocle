package tui

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A note's markdown links (John 2026-10-22: stop-note links; stage://view/<name>#<anchor> focuses a stage view and scrolls
// to the item) are OSC 8 hyperlinks, underlined, so a click in the terminal opens them; :link N and the OpenLink key open one
// from the keyboard. Only http, https and stage:// links: anything else (file:, javascript:, an app's own scheme) stays as
// written, never a link.

// openURL hands a link to macOS (`open`): a test replaces it.
var openURL = func(u string) error { return exec.Command("open", u).Start() }

var linkPattern = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)

// linkAllowed is whether a link may be opened: http, https or stage.
func linkAllowed(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "stage://")
}

// outsideCode runs fn over the parts of line that are not inline code spans.
func outsideCode(line string, fn func(string) string) string {
	var b strings.Builder
	at := 0
	for _, span := range inlineCodePattern.FindAllStringIndex(line, -1) {
		b.WriteString(fn(line[at:span[0]]))
		b.WriteString(line[span[0]:span[1]])
		at = span[1]
	}
	b.WriteString(fn(line[at:]))
	return b.String()
}

// styleLinks makes each allowed link its text, underlined, inside an OSC 8 hyperlink.
func (ms *markdownStyler) styleLinks(line string) string {
	style := lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("12"))
	return outsideCode(line, func(s string) string {
		return linkPattern.ReplaceAllStringFunc(s, func(match string) string {
			m := linkPattern.FindStringSubmatch(match)
			if m == nil || !linkAllowed(m[2]) {
				return match
			}
			return ansi.SetHyperlink(m[2]) + style.Render(m[1]) + ansi.ResetHyperlink()
		})
	})
}

// noteLinks is a note's openable links, in order (those in inline code are code, not links).
func noteLinks(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		outsideCode(line, func(s string) string {
			for _, m := range linkPattern.FindAllStringSubmatch(s, -1) {
				if linkAllowed(m[2]) {
					out = append(out, m[2])
				}
			}
			return s
		})
	}
	return out
}

// linkMsg is `:link [n]`.
type linkMsg struct{ arg string }

// handleLink opens the doc pane note's nth link (1 when not given).
func (m appModel) handleLink(arg string) appModel {
	links := noteLinks(m.docPane.noteSource)
	n := 1
	if a := strings.TrimSpace(arg); a != "" {
		v, err := strconv.Atoi(a)
		if err != nil {
			m.statusBar.searchInfo = "usage: :link [n]"
			return m
		}
		n = v
	}
	if n < 1 || n > len(links) {
		m.statusBar.searchInfo = fmt.Sprintf("no link %d (the note has %d)", n, len(links))
		return m
	}
	if err := openURL(links[n-1]); err != nil {
		m.statusBar.searchInfo = "could not open " + links[n-1] + ": " + err.Error()
		return m
	}
	m.statusBar.searchInfo = fmt.Sprintf("opened link %d: %s", n, links[n-1])
	return m
}

// hyperlinkAfter is the hyperlink left open once row has been written, given the one it started in ("" for none).
func hyperlinkAfter(open, row string) string {
	for {
		i := strings.Index(row, "\x1b]8;")
		if i < 0 {
			return open
		}
		row = row[i+4:]
		end := strings.IndexAny(row, "\x07\x1b")
		if end < 0 {
			return open
		}
		params := row[:end]
		if semi := strings.IndexByte(params, ';'); semi >= 0 {
			open = params[semi+1:]
		}
		row = row[end:]
	}
}
