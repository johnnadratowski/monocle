package tui

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // register GIF decoder for previews/dimensions
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// mediaViewerDoneMsg is returned after the external media viewer exits.
type mediaViewerDoneMsg struct {
	err error
}

// openInMediaViewer opens filePath in the configured media viewer (default
// Google Chrome). GUI launchers (e.g. `open -a "Google Chrome"`) run detached so
// the TUI is not suspended (no flash to the terminal); a terminal viewer would
// take over the screen.
func openInMediaViewer(filePath, configured string) tea.Cmd {
	name, args := resolveMediaViewer(configured)
	args = append(args, filePath)
	return runViewer(name, args, func(err error) tea.Msg { return mediaViewerDoneMsg{err: err} })
}

// runViewer launches a viewer command. GUI launchers that return immediately are
// run detached (via a plain Cmd goroutine) so Monocle's alt-screen TUI is never
// suspended — avoiding a visible flash to the terminal and back. Terminal viewers
// (e.g. glow's pager) take over the screen via tea.ExecProcess.
func runViewer(name string, args []string, done func(error) tea.Msg) tea.Cmd {
	cmd := exec.Command(name, args...)
	if isGUILauncher(name, args) {
		return func() tea.Msg { return done(cmd.Run()) }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return done(err) })
}

// isGUILauncher reports whether a viewer command opens a separate GUI window and
// returns immediately (so it must not take over the terminal). Recognizes the
// platform "open" helpers and app-launch forms (`-a Foo`, `Foo.app`).
func isGUILauncher(name string, args []string) bool {
	switch filepath.Base(name) {
	case "open", "xdg-open", "start", "cmd", "gio":
		return true
	}
	for _, a := range args {
		if a == "-a" || strings.HasSuffix(strings.ToLower(a), ".app") {
			return true
		}
	}
	return false
}

// resolveMediaViewer returns the viewer binary and args. A configured value may
// include flags and quoted args (e.g. `open -a "Google Chrome"`). When unset it
// falls back to a platform-appropriate Google Chrome launcher.
func resolveMediaViewer(configured string) (string, []string) {
	if parts := splitCommandLine(configured); len(parts) > 0 {
		return parts[0], parts[1:]
	}
	return defaultChromeCommand()
}

// defaultChromeCommand returns a best-effort command to open a file in Google
// Chrome for the current OS.
func defaultChromeCommand() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{"-a", "Google Chrome"}
	case "windows":
		return "cmd", []string{"/c", "start", "chrome"}
	default:
		return "google-chrome", nil
	}
}

// mediaIcon returns a glyph for a media category.
func mediaIcon(category string) string {
	switch category {
	case "image":
		return "🖼"
	case "video":
		return "🎬"
	case "audio":
		return "🎵"
	default:
		return "📄"
	}
}

// renderMediaCard builds the lines shown for a media artifact: a title, a
// metadata block, and — for images Monocle can decode — an ANSI half-block
// preview. Lines already carry any ANSI styling and are rendered verbatim.
func renderMediaCard(item types.ContentItem, width, height int) []string {
	label := lipgloss.NewStyle().Faint(true)
	title := lipgloss.NewStyle().Bold(true)

	out := []string{
		title.Render(fmt.Sprintf("%s  %s", mediaIcon(item.MediaType), item.Title)),
		"",
	}

	rows := [][2]string{
		{"File", filepath.Base(item.MediaPath)},
		{"Type", mediaTypeString(item)},
	}
	if info, err := os.Stat(item.MediaPath); err == nil {
		rows = append(rows, [2]string{"Size", humanSize(info.Size())})
		rows = append(rows, [2]string{"Modified", info.ModTime().Format("2006-01-02 15:04")})
	}
	if item.MediaType == "image" {
		if w, h, ok := imageDimensions(item.MediaPath); ok {
			rows = append(rows, [2]string{"Dimensions", fmt.Sprintf("%d × %d px", w, h)})
		}
	}
	rows = append(rows, [2]string{"Stored", item.MediaPath})

	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s  %s", label.Render(fmt.Sprintf("%-10s", r[0])), r[1]))
	}

	// ANSI preview for decodable images.
	if item.MediaType == "image" {
		maxCols := width - 2
		if maxCols > 100 {
			maxCols = 100
		}
		maxRows := height - len(out) - 4
		if maxRows > 40 {
			maxRows = 40
		}
		if maxCols > 4 && maxRows > 2 {
			if preview := ansiImagePreview(item.MediaPath, maxCols, maxRows); len(preview) > 0 {
				out = append(out, "")
				out = append(out, preview...)
			}
		}
	}

	out = append(out, "", label.Render("Press Ctrl+p to open in the media viewer"))
	return out
}

// mediaTypeString renders the type/MIME description for the card.
func mediaTypeString(item types.ContentItem) string {
	if item.MimeType != "" {
		return fmt.Sprintf("%s (%s)", item.MediaType, item.MimeType)
	}
	return item.MediaType
}

// humanSize formats a byte count as a human-readable string.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// imageDimensions returns the pixel dimensions of an image without fully
// decoding it, when the format is supported.
func imageDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// ansiImagePreview renders an image as ANSI half-block rows (each character is
// two vertically-stacked pixels: foreground = top, background = bottom via the
// upper-half-block glyph ▀), scaled to fit within maxCols × maxRows. Returns nil
// when the image can't be decoded.
func ansiImagePreview(path string, maxCols, maxRows int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return nil
	}

	// Target pixel grid: maxCols wide, maxRows*2 tall (2 px per character row).
	scale := math.Min(float64(maxCols)/float64(iw), float64(maxRows*2)/float64(ih))
	if scale > 1 {
		scale = 1 // never upscale
	}
	ow := int(float64(iw) * scale)
	oh := int(float64(ih) * scale)
	if ow < 1 {
		ow = 1
	}
	if oh < 2 {
		oh = 2
	}
	if oh%2 == 1 {
		oh++
	}

	lines := make([]string, 0, oh/2)
	for y := 0; y < oh; y += 2 {
		var sb strings.Builder
		for x := 0; x < ow; x++ {
			top := samplePixel(img, b, x, y, ow, oh)
			bot := samplePixel(img, b, x, y+1, ow, oh)
			sb.WriteString(lipgloss.NewStyle().Foreground(top).Background(bot).Render("▀"))
		}
		lines = append(lines, sb.String())
	}
	return lines
}

// samplePixel nearest-neighbor samples the source image for output pixel (ox,oy)
// in an ow×oh grid, compositing any alpha over black.
func samplePixel(img image.Image, b image.Rectangle, ox, oy, ow, oh int) color.Color {
	sx := b.Min.X + ox*b.Dx()/ow
	sy := b.Min.Y + oy*b.Dy()/oh
	r, g, bl, a := img.At(sx, sy).RGBA()
	if a < 0xffff {
		r = r * a / 0xffff
		g = g * a / 0xffff
		bl = bl * a / 0xffff
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(bl>>8)))
}

// changedMediaFile reports whether the currently loaded changed file (m.path) is
// a media file present on disk, returning a synthetic ContentItem describing it
// so the binary-diff path can render a media card instead of a placeholder.
func (m diffViewModel) changedMediaFile() (types.ContentItem, bool) {
	if m.contentMode || m.path == "" || m.repoRoot == "" {
		return types.ContentItem{}, false
	}
	category, mimeType, ok := types.MediaInfo(m.path)
	if !ok {
		return types.ContentItem{}, false
	}
	full := filepath.Join(m.repoRoot, m.path)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return types.ContentItem{}, false // e.g. a deleted file — fall back to placeholder
	}
	return types.ContentItem{
		Title:     m.path,
		MediaPath: full,
		MediaType: category,
		MimeType:  mimeType,
	}, true
}

// buildMediaCardLines rebuilds the diff view's line list from the media card for
// the current width, storing each card line as a verbatim (already-styled) line.
func (m *diffViewModel) buildMediaCardLines() {
	width := m.width
	if width < 1 {
		width = 80
	}
	card := renderMediaCard(m.mediaItem, width, m.height)
	m.lines = make([]diffViewLine, 0, len(card))
	for i, ln := range card {
		m.lines = append(m.lines, diffViewLine{
			kind:       types.DiffLineContext,
			newLineNum: i + 1,
			content:    ln,
			verbatim:   true,
		})
	}
	m.mediaCardWidth = width
	m.ClearSearch()
}
