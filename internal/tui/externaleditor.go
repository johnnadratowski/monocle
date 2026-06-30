package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// externalEditorRequestMsg is emitted by a modal when the user presses
// Ctrl+G to open the current text in an external editor.
type externalEditorRequestMsg struct {
	body   string
	origin overlayKind
}

// externalEditorResultMsg is returned after the external editor exits.
type externalEditorResultMsg struct {
	body   string
	origin overlayKind
	err    error
}

// openExternalEditor writes the body to a temp file, opens it in the user's
// $VISUAL/$EDITOR, and returns a tea.Cmd that suspends the TUI via
// tea.ExecProcess. When the editor exits, it reads back the file and returns
// an externalEditorResultMsg.
func openExternalEditor(body string, origin overlayKind, configured string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "monocle-*.md")
	if err != nil {
		return func() tea.Msg {
			return externalEditorResultMsg{origin: origin, err: err}
		}
	}

	if _, err := tmpFile.WriteString(body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return func() tea.Msg {
			return externalEditorResultMsg{origin: origin, err: err}
		}
	}
	tmpFile.Close()

	name, args := resolveEditor(configured)
	args = append(args, tmpFile.Name())
	cmd := exec.Command(name, args...)

	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		defer os.Remove(tmpFile.Name())

		if execErr != nil {
			return externalEditorResultMsg{origin: origin, err: execErr}
		}

		content, readErr := os.ReadFile(tmpFile.Name())
		if readErr != nil {
			return externalEditorResultMsg{origin: origin, err: readErr}
		}

		return externalEditorResultMsg{body: string(content), origin: origin}
	})
}

// openFileInEditorDoneMsg is returned after the external editor exits when
// editing an actual file (not a temp file for modal body text).
type openFileInEditorDoneMsg struct {
	err error
}

// editorOpenSpec describes how to open a file on disk in the external editor.
type editorOpenSpec struct {
	filePath   string
	line       int
	configured string // configured editor command ("" → $VISUAL/$EDITOR/vi)
	mode       string // "terminal" | "tmux_vertical" | "tmux_horizontal" | "tmux_window"
	focus      bool   // tmux modes: whether the new pane/window takes focus
}

// openFileInEditor opens the given file in the configured editor (or
// $VISUAL/$EDITOR) at the specified line number, taking over Monocle's terminal.
func openFileInEditor(filePath string, line int, configured string) tea.Cmd {
	return openFileInEditorWith(editorOpenSpec{filePath: filePath, line: line, configured: configured, mode: "terminal"})
}

// openFileInEditorWith opens a file according to spec. When a tmux mode is
// selected and Monocle is running inside tmux, the editor opens in a new tmux
// split or window without taking over Monocle's terminal. Otherwise (terminal
// mode, or not inside tmux) it takes over the terminal via tea.ExecProcess.
func openFileInEditorWith(spec editorOpenSpec) tea.Cmd {
	name, args := resolveEditor(spec.configured)
	if spec.line > 0 {
		args = append(args, fmt.Sprintf("+%d", spec.line))
	}
	args = append(args, spec.filePath)

	if tmuxArgs, ok := tmuxOpenArgs(spec.mode, spec.focus, append([]string{name}, args...)); ok {
		cmd := exec.Command("tmux", tmuxArgs...)
		// A tmux split/window launches into its own pane and returns immediately,
		// so this does not take over Monocle's terminal.
		return func() tea.Msg {
			return openFileInEditorDoneMsg{err: cmd.Run()}
		}
	}

	cmd := exec.Command(name, args...)
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return openFileInEditorDoneMsg{err: execErr}
	})
}

// tmuxOpenArgs builds the `tmux` arguments to open the editor invocation in a
// split or window. It returns ok=false for "terminal" mode (or any unknown mode)
// and when Monocle is not running inside tmux.
func tmuxOpenArgs(mode string, focus bool, argv []string) ([]string, bool) {
	if os.Getenv("TMUX") == "" {
		return nil, false
	}
	var out []string
	switch mode {
	case "tmux_vertical":
		out = []string{"split-window", "-h"} // side by side (left/right)
	case "tmux_horizontal":
		out = []string{"split-window", "-v"} // stacked (top/bottom)
	case "tmux_window":
		out = []string{"new-window"}
	default:
		return nil, false
	}
	if !focus {
		out = append(out, "-d") // create without switching focus to it
	}
	out = append(out, shellJoin(argv))
	return out, true
}

// shellJoin renders argv as a POSIX-sh command string, single-quoting as needed,
// so it can be passed as tmux's shell-command argument.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '/', '.', '_', '-', '+', '@', ':', ',', '=':
			continue
		}
		// Unsafe character — single-quote the whole token.
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// resolveEditor returns the editor binary name and any extra arguments.
// Precedence: the configured editor (config "editor" field), then $VISUAL, then
// $EDITOR, falling back to "vi". The value may include flags (e.g. "code --wait").
func resolveEditor(configured string) (string, []string) {
	candidates := []string{configured, os.Getenv("VISUAL"), os.Getenv("EDITOR")}
	for _, val := range candidates {
		if val != "" {
			parts := strings.Fields(val)
			return parts[0], parts[1:]
		}
	}
	return "vi", nil
}
