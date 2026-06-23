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

// openFileInEditor opens the given file in the configured editor (or
// $VISUAL/$EDITOR) at the specified line number. Unlike openExternalEditor, this
// opens the actual file on disk rather than a temp file copy.
func openFileInEditor(filePath string, line int, configured string) tea.Cmd {
	name, args := resolveEditor(configured)
	if line > 0 {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	args = append(args, filePath)
	cmd := exec.Command(name, args...)
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return openFileInEditorDoneMsg{err: execErr}
	})
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
