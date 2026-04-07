package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderSplashFullTier(t *testing.T) {
	result := renderSplash(90, 30)
	if !strings.Contains(result, "◉") {
		t.Error("full splash should contain ◉ character")
	}
	if !strings.Contains(result, "monocle register") {
		t.Error("full splash should contain register instruction")
	}
	if !strings.Contains(result, "to submit your review") {
		t.Error("full splash should contain review hint")
	}
}

func TestRenderSplashSmallTier(t *testing.T) {
	result := renderSplash(30, 8)
	if !strings.Contains(result, "o_(◉)") {
		t.Error("small splash should contain o_(◉) text")
	}
	if !strings.Contains(result, "Press ? for help") {
		t.Error("small splash should contain help hint")
	}
}

func TestRenderSplashTierSelection(t *testing.T) {
	// Full tier requires width >= 80 and height >= 20
	full := renderSplash(80, 20)
	if !strings.Contains(full, "to submit your review") {
		t.Error("80x20 should use full tier")
	}

	// Below width threshold
	small := renderSplash(79, 20)
	if strings.Contains(small, "to submit your review") {
		t.Error("79x20 should use small tier")
	}

	// Below height threshold
	smallH := renderSplash(80, 19)
	if strings.Contains(smallH, "to submit your review") {
		t.Error("80x19 should use small tier")
	}
}

func TestRenderSplashZeroDimensions(t *testing.T) {
	result := renderSplash(0, 0)
	if result == "" {
		t.Error("zero dimensions should still return fallback content")
	}
}

func TestCenterBlockAlignment(t *testing.T) {
	lines := []string{"hello", "world"}
	result := centerBlock(lines, 20, 12)

	resultLines := strings.Split(result, "\n")

	// Should have vertical padding: (12-2)/3 = 3
	padTop := (12 - 2) / 3
	if len(resultLines) < padTop+2 {
		t.Errorf("expected at least %d lines, got %d", padTop+2, len(resultLines))
	}

	// Content lines should be horizontally centered
	for _, line := range resultLines[padTop:] {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		leftPad := len(line) - len(trimmed)
		if leftPad < 1 {
			t.Errorf("expected horizontal padding, got line: %q", line)
		}
	}
}

func TestCenterBlockWithStyledContent(t *testing.T) {
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	lines := []string{
		blue.Render("styled text"),
		"plain text",
	}
	result := centerBlock(lines, 40, 12)

	if result == "" {
		t.Error("centerBlock with styled content should produce output")
	}
	if !strings.Contains(result, "styled text") {
		t.Error("output should contain the styled text content")
	}
}

func TestCenterBlockZeroDimensions(t *testing.T) {
	lines := []string{"test"}
	result := centerBlock(lines, 0, 0)
	if result != "test" {
		t.Errorf("zero dimensions should return joined lines, got %q", result)
	}
}
