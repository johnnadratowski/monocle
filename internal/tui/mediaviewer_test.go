package tui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestResolveMediaViewer(t *testing.T) {
	// Configured value (with a quoted arg) is parsed and used verbatim.
	name, args := resolveMediaViewer(`open -a "Google Chrome"`)
	if name != "open" || len(args) != 2 || args[1] != "Google Chrome" {
		t.Errorf("configured: got %q %v", name, args)
	}
	// Empty falls back to a platform Chrome launcher.
	name, args = resolveMediaViewer("")
	switch runtime.GOOS {
	case "darwin":
		if name != "open" || len(args) != 2 || args[1] != "Google Chrome" {
			t.Errorf("darwin default: got %q %v", name, args)
		}
	case "windows":
		if name != "cmd" {
			t.Errorf("windows default: got %q", name)
		}
	default:
		if name != "google-chrome" {
			t.Errorf("linux default: got %q", name)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		512:        "512 B",
		1024:       "1.0 KiB",
		1536:       "1.5 KiB",
		1048576:    "1.0 MiB",
		1073741824: "1.0 GiB",
	}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// writeTestPNG writes a small solid-color PNG and returns its path.
func writeTestPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 128, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "test.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImageDimensions(t *testing.T) {
	path := writeTestPNG(t, 20, 10)
	w, h, ok := imageDimensions(path)
	if !ok || w != 20 || h != 10 {
		t.Errorf("imageDimensions = (%d, %d, %v), want (20, 10, true)", w, h, ok)
	}
	if _, _, ok := imageDimensions(filepath.Join(t.TempDir(), "nope.png")); ok {
		t.Error("expected ok=false for a missing file")
	}
}

func TestAnsiImagePreview(t *testing.T) {
	path := writeTestPNG(t, 40, 40)
	lines := ansiImagePreview(path, 20, 10)
	if len(lines) == 0 {
		t.Fatal("expected preview lines")
	}
	// Each half-block row must fit within the requested column budget.
	for i, ln := range lines {
		if len(lines) > 10 {
			t.Fatalf("preview exceeded maxRows: %d rows", len(lines))
		}
		if !strings.Contains(ln, "▀") {
			t.Errorf("line %d missing half-block glyph", i)
		}
	}
	// A non-image returns nil rather than panicking.
	if got := ansiImagePreview(filepath.Join(t.TempDir(), "missing.png"), 20, 10); got != nil {
		t.Error("expected nil for missing image")
	}
}

func TestChangedMediaFile(t *testing.T) {
	repo := t.TempDir()
	// Write an image inside the repo.
	imgPath := writeTestPNG(t, 10, 10)
	data, _ := os.ReadFile(imgPath)
	if err := os.MkdirAll(filepath.Join(repo, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "assets", "logo.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// A changed media file on disk → media card.
	m := diffViewModel{repoRoot: repo, path: "assets/logo.png"}
	item, ok := m.changedMediaFile()
	if !ok || item.MediaType != "image" || item.MimeType != "image/png" {
		t.Fatalf("changedMediaFile = %+v ok=%v", item, ok)
	}
	if item.MediaPath != filepath.Join(repo, "assets/logo.png") {
		t.Errorf("MediaPath = %q", item.MediaPath)
	}

	// A non-media file → not a media card.
	if _, ok := (diffViewModel{repoRoot: repo, path: "main.go"}).changedMediaFile(); ok {
		t.Error("main.go should not be a media file")
	}
	// A media path that isn't on disk (e.g. deleted) → falls back.
	if _, ok := (diffViewModel{repoRoot: repo, path: "gone.png"}).changedMediaFile(); ok {
		t.Error("missing file should not render a card")
	}
	// Content mode (artifact) is handled separately, not here.
	if _, ok := (diffViewModel{repoRoot: repo, path: "assets/logo.png", contentMode: true}).changedMediaFile(); ok {
		t.Error("content mode should not use changedMediaFile")
	}
}

func TestRenderMediaCard(t *testing.T) {
	path := writeTestPNG(t, 30, 30)
	item := types.ContentItem{
		Title:     "Screenshot",
		MediaPath: path,
		MediaType: "image",
		MimeType:  "image/png",
	}
	lines := renderMediaCard(item, 80, 40)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Screenshot", "image/png", "Dimensions", "30 × 30", "Ctrl+p"} {
		if !strings.Contains(joined, want) {
			t.Errorf("card missing %q\n%s", want, joined)
		}
	}
}
