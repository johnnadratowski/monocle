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
	case tui.Matches(key, s.keys.Up):
		if s.claudeCursor > 0 {
			s.claudeCursor--
		}
	case tui.Matches(key, s.keys.Down):
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

// codeTokens lists substrings in claudeExplain that should render in code
// color rather than faint prose.
var codeTokens = []string{"--dangerously-load-development-channels"}

// viewClaude renders the hooks step for both modes.
func viewClaude(s WizardState, width int) string {
	var b strings.Builder

	title := claudeTitleRegister
	subtitle := claudeSubtitleRegister
	if s.mode == ModeUnregister {
		title = claudeTitleUnregister
		subtitle = claudeSubtitleUnregister
	}
	b.WriteString(styleBold.Render(title) + "\n")
	b.WriteString(styleFaint.Render(subtitle) + "\n\n")

	planLabel := planToggleRegister
	gateLabel := gateToggleRegister
	if s.mode == ModeUnregister {
		planLabel = planToggleUnregister
		gateLabel = gateToggleUnregister
	}

	b.WriteString(renderHookToggle(s, 0, planLabel, planToggleDesc, s.planInstalled(), s.planLocked(), planHookRows, width))
	b.WriteString("\n")
	b.WriteString(renderHookToggle(s, 1, gateLabel, gateToggleDesc, s.gateInstalled(), s.gateLocked(), reviewGateRows, width))

	b.WriteString("\n")
	explain := claudeExplain
	if s.mode == ModeUnregister {
		explain = claudeExplainUnregister
	}
	// Word-wrap against the available width before styling so the note flows
	// naturally in wide terminals instead of breaking at hardcoded newlines.
	b.WriteString(renderExplain(indentedWrap(explain, 0, width)))
	return b.String()
}

// renderExplain styles the compat note at the bottom of the Claude step,
// rendering code-like tokens in yellow and the rest in faint prose. Splits
// per-line so lipgloss doesn't block-pad multi-line input — that padding
// pushed styled spans to the far right of the available width.
func renderExplain(text string) string {
	var out strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		out.WriteString(styleLineTokens(line, styleFaint, styleCode, codeTokens))
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

func renderHookToggle(s WizardState, idx int, label, desc string, checked, locked bool, rows []hookRow, width int) string {
	var b strings.Builder
	onRow := s.claudeCursor == idx
	// Match the agents step layout: rowCursor gutter (2 cols) + checkbox
	// glyph (1 col) + space (1 col) + label.
	b.WriteString(rowCursor(onRow))
	b.WriteString(checkbox(checked, locked))
	b.WriteString(" ")
	b.WriteString(highlightLabel(label, onRow))
	if locked {
		b.WriteString(lockNote(hookFlagName(s.mode, idx)))
	}
	b.WriteString("\n")

	// Description wraps + prefix manually — lipgloss PaddingLeft + Width
	// doesn't survive the outer ModalBorder re-flow. Gutter: 2 cursor +
	// 1 checkbox + 1 space = 4 cols to align with the label.
	b.WriteString(styleFaint.Render(indentedWrap(desc, 4, width)))
	b.WriteString("\n")

	// Tool sublist matches the agents-step path sublist depth (7 cols).
	for _, r := range rows {
		b.WriteString("       ")
		b.WriteString(styleFaint.Render("• " + r.label + "  "))
		b.WriteString(styleFaint.Render("(" + r.note + ")"))
		b.WriteString("\n")
	}
	return b.String()
}

// hookFlagName returns the CLI flag associated with toggle `idx` in the
// current wizard mode, for the "(via --flag)" lock annotation.
func hookFlagName(mode Mode, idx int) string {
	switch {
	case mode == ModeRegister && idx == 0:
		return "--no-plan-hook"
	case mode == ModeRegister && idx == 1:
		return "--no-review-gate"
	case mode == ModeUnregister && idx == 0:
		return "--keep-plan-hook"
	default:
		return "--keep-review-gate"
	}
}
