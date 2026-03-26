package tui

import (
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
func openExternalEditor(body string, origin overlayKind) tea.Cmd {
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

	name, args := resolveEditor()
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

// resolveEditor returns the editor binary name and any extra arguments.
// It checks $VISUAL, then $EDITOR, falling back to "vi".
func resolveEditor() (string, []string) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if val := os.Getenv(env); val != "" {
			parts := strings.Fields(val)
			return parts[0], parts[1:]
		}
	}
	return "vi", nil
}
