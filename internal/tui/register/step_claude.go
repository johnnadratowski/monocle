package register

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/tui"
)

// updateClaude routes key input for the Claude hooks step. Two toggles, one
// cursor. Space toggles the focused row; up/down moves. Advance leaves the
// step.
func updateClaude(m Model, key string) (tea.Model, tea.Cmd) {
	s := m.state
	switch {
	case tui.Matches(key, s.keys.Up) || key == "up":
		if s.claudeCursor > 0 {
			s.claudeCursor--
		}
	case tui.Matches(key, s.keys.Down) || key == "down":
		if s.claudeCursor < 1 {
			s.claudeCursor++
		}
	case tui.Matches(key, s.keys.WizardToggle):
		if s.claudeCursor == 0 && !s.planLocked() {
			s.planToggle = !s.planToggle
		} else if s.claudeCursor == 1 && !s.gateLocked() {
			s.gateToggle = !s.gateToggle
		}
	case tui.Matches(key, s.keys.WizardAdvance):
		m.state = s
		return m, advanceCmd()
	}
	m.state = s
	return m, nil
}

func (s WizardState) planLocked() bool {
	if s.mode == ModeRegister {
		return s.opts.SkipPlanHookLocked
	}
	return s.opts.KeepPlanHookLocked
}

func (s WizardState) gateLocked() bool {
	if s.mode == ModeRegister {
		return s.opts.SkipReviewGateLocked
	}
	return s.opts.KeepReviewGateLocked
}

// planInstalled reports whether, in the current state, plan hooks will end
// up installed/retained. Register: installed = !planToggle (toggle means
// "skip"). Unregister: retained = planToggle (toggle means "keep").
func (s WizardState) planInstalled() bool {
	if s.mode == ModeRegister {
		return !s.planToggle
	}
	return s.planToggle
}

func (s WizardState) gateInstalled() bool {
	if s.mode == ModeRegister {
		return !s.gateToggle
	}
	return s.gateToggle
}

// viewClaude renders the hooks step for both modes.
func viewClaude(s WizardState, width int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Faint(true)
	bold := lipgloss.NewStyle().Bold(true)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

	title := claudeTitleRegister
	subtitle := claudeSubtitleRegister
	if s.mode == ModeUnregister {
		title = claudeTitleUnregister
		subtitle = claudeSubtitleUnregister
	}
	b.WriteString(bold.Render(title) + "\n")
	b.WriteString(faint.Render(subtitle) + "\n\n")

	planLabel := planToggleRegister
	gateLabel := gateToggleRegister
	if s.mode == ModeUnregister {
		planLabel = planToggleUnregister
		gateLabel = gateToggleUnregister
	}

	b.WriteString(renderHookToggle(s, 0, planLabel, planToggleDesc, s.planInstalled(), s.planLocked(), planHookRows, width, cursorStyle, bold, faint))
	b.WriteString("\n")
	b.WriteString(renderHookToggle(s, 1, gateLabel, gateToggleDesc, s.gateInstalled(), s.gateLocked(), reviewGateRows, width, cursorStyle, bold, faint))

	b.WriteString("\n")
	explain := claudeExplain
	if s.mode == ModeUnregister {
		explain = claudeExplainUnregister
	}
	// Word-wrap against the available width before styling so the note flows
	// naturally in wide terminals instead of breaking at the hardcoded
	// newlines the source literal used to ship.
	b.WriteString(renderExplain(indentedWrap(explain, 0, width), faint))
	return b.String()
}

// renderExplain styles the compat note at the bottom of the Claude step,
// rendering code-like tokens (currently just the channels flag) in yellow —
// the same color the main TUI uses for MarkdownCode — while leaving the
// surrounding prose faint.
//
// The text is split per-line so a single `faint.Render` call never receives
// an embedded newline; lipgloss block-pads multi-line inputs, which pushed
// subsequent styled spans to the far right of the available width.
func renderExplain(text string, faint lipgloss.Style) string {
	code := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	codeTokens := []string{"--dangerously-load-development-channels"}

	var out strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		out.WriteString(styleLineTokens(line, faint, code, codeTokens))
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

// styleLineTokens renders a single line, substituting any occurrences of
// `tokens` with `tokenStyle` and leaving the rest in `baseStyle`.
func styleLineTokens(line string, baseStyle, tokenStyle lipgloss.Style, tokens []string) string {
	var out strings.Builder
	remaining := line
	for len(remaining) > 0 {
		next := -1
		matched := ""
		for _, tok := range tokens {
			i := strings.Index(remaining, tok)
			if i < 0 {
				continue
			}
			if next < 0 || i < next {
				next = i
				matched = tok
			}
		}
		if next < 0 {
			out.WriteString(baseStyle.Render(remaining))
			break
		}
		if next > 0 {
			out.WriteString(baseStyle.Render(remaining[:next]))
		}
		out.WriteString(tokenStyle.Render(matched))
		remaining = remaining[next+len(matched):]
	}
	return out.String()
}

func renderHookToggle(s WizardState, idx int, label, desc string, checked, locked bool, rows []hookRow, width int, cursorStyle, bold, faint lipgloss.Style) string {
	var b strings.Builder
	onRow := s.claudeCursor == idx
	// Match the agents step layout: rowCursor gutter (2 cols) + checkbox
	// glyph (1 col) + space (1 col) + label.
	b.WriteString(rowCursor(onRow))
	b.WriteString(checkbox(checked, locked))
	b.WriteString(" ")
	b.WriteString(highlightLabel(label, onRow))
	if locked {
		flag := "--no-plan-hook"
		if s.mode == ModeUnregister {
			flag = "--keep-plan-hook"
		}
		if idx == 1 {
			if s.mode == ModeUnregister {
				flag = "--keep-review-gate"
			} else {
				flag = "--no-review-gate"
			}
		}
		b.WriteString(lockNote(flag))
	}
	b.WriteString("\n")

	// Description indented to line up with the label text. We wrap + prefix
	// manually rather than using lipgloss PaddingLeft + Width because the
	// outer ModalBorder re-flows the content block and strips the inner
	// padding from continuation lines. Gutter: 2 cols cursor + 1 col
	// checkbox + 1 col space = 4 cols.
	b.WriteString(faint.Render(indentedWrap(desc, 4, width)))
	b.WriteString("\n")

	// Tool sublist indented at the same 7-col depth as the agents-step
	// path sublist so the two screens read as the same layout.
	for _, r := range rows {
		b.WriteString("       ")
		b.WriteString(faint.Render("• " + r.label + "  "))
		b.WriteString(faint.Render("(" + r.note + ")"))
		b.WriteString("\n")
	}
	return b.String()
}
