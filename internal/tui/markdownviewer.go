package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// markdownViewerDoneMsg is returned after the external markdown viewer exits.
type markdownViewerDoneMsg struct {
	err error
}

// openInMarkdownViewer opens filePath in the configured rendered-markdown viewer
// (default "glow"), taking over Monocle's terminal via tea.ExecProcess. cleanup,
// when non-nil, runs after the viewer exits — used to remove a temp file written
// for an artifact that has no on-disk path.
func openInMarkdownViewer(filePath, configured string, cleanup func()) tea.Cmd {
	name, args := resolveMarkdownViewer(configured)
	args = append(args, filePath)
	cmd := exec.Command(name, args...)
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		if cleanup != nil {
			cleanup()
		}
		return markdownViewerDoneMsg{err: execErr}
	})
}

// resolveMarkdownViewer returns the viewer binary and any extra arguments. The
// configured value may include flags (e.g. "glow -p"). When unset it falls back
// to "glow", the canonical terminal markdown renderer.
func resolveMarkdownViewer(configured string) (string, []string) {
	if strings.TrimSpace(configured) != "" {
		parts := strings.Fields(configured)
		return parts[0], parts[1:]
	}
	return "glow", nil
}

// markdownExts are the file extensions treated as markdown for the viewer.
var markdownExts = map[string]bool{
	".md":       true,
	".markdown": true,
	".mdown":    true,
	".mkd":      true,
	".mkdn":     true,
}

// isMarkdownPath reports whether path has a markdown file extension.
func isMarkdownPath(path string) bool {
	return markdownExts[strings.ToLower(filepath.Ext(path))]
}

// writeArtifactTempMarkdown writes body to a temporary .md file and returns its
// path plus a cleanup func that removes it.
func writeArtifactTempMarkdown(body string) (string, func(), error) {
	f, err := os.CreateTemp("", "monocle-artifact-*.md")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	name := f.Name()
	return name, func() { os.Remove(name) }, nil
}
