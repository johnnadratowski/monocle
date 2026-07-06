package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// markdownViewerDoneMsg is returned after the external markdown viewer exits.
type markdownViewerDoneMsg struct {
	err error
}

// openInMarkdownViewer opens filePath in the configured rendered-markdown viewer
// (default "glow -p"), taking over Monocle's terminal via tea.ExecProcess. GUI
// launchers (e.g. "open -a MacDown") return immediately, so the window stays open
// beside the terminal; terminal viewers (glow's pager) take over until quit.
func openInMarkdownViewer(filePath, configured string) tea.Cmd {
	name, args := resolveMarkdownViewer(configured)
	args = append(args, filePath)
	cmd := exec.Command(name, args...)
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return markdownViewerDoneMsg{err: execErr}
	})
}

// resolveMarkdownViewer returns the viewer binary and any extra arguments. The
// configured value may include flags (e.g. "open -a MacDown"). When unset it
// falls back to "glow -p" — glow's pager, which stays open and scrollable
// instead of rendering to stdout and exiting immediately.
func resolveMarkdownViewer(configured string) (string, []string) {
	if parts := splitCommandLine(configured); len(parts) > 0 {
		return parts[0], parts[1:]
	}
	return "glow", []string{"-p"}
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

// artifactTempMaxAge is how long an artifact temp file lingers before it's
// reaped. Long enough for a GUI viewer session, short enough not to accumulate.
const artifactTempMaxAge = time.Hour

// artifactTempDir returns (creating if needed) the directory that holds temp
// markdown files rendered from artifacts.
func artifactTempDir() (string, error) {
	dir := filepath.Join(os.TempDir(), "monocle-artifacts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// reapOldArtifactTemps removes artifact temp files older than maxAge. It runs on
// each launch instead of deleting immediately after the viewer exits: a GUI
// launcher returns before its app has read the file, so an immediate delete would
// race. Reaping on a delay avoids that while still bounding accumulation.
func reapOldArtifactTemps(dir string, maxAge time.Duration, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// writeArtifactTempMarkdown writes body to a .md file (named after title, so the
// viewer's window/title reads nicely) in the reaped artifact temp dir and returns
// its path. Old temp files are reaped first; the written file is not deleted
// immediately (see reapOldArtifactTemps).
func writeArtifactTempMarkdown(title, body string) (string, error) {
	dir, err := artifactTempDir()
	if err != nil {
		return "", err
	}
	reapOldArtifactTemps(dir, artifactTempMaxAge, time.Now())

	path := filepath.Join(dir, sanitizeFilename(title)+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// sanitizeFilename turns an artifact title into a safe file base name, keeping
// alphanumerics, spaces, dashes, underscores and dots, collapsing everything else
// to a dash. Falls back to "artifact" when nothing usable remains.
func sanitizeFilename(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(strings.TrimSpace(b.String()), "-.")
	if len(name) > 60 {
		name = strings.TrimRight(name[:60], "-. ")
	}
	if name == "" {
		return "artifact"
	}
	return name
}
