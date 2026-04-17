package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestLayoutModeBreakpoint(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  layoutMode
	}{
		{"wide terminal selects horizontal", 120, layoutHorizontal},
		{"exactly at breakpoint selects horizontal", 110, layoutHorizontal},
		{"narrow terminal selects stacked", 109, layoutStacked},
		{"very narrow terminal selects stacked", 40, layoutStacked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewApp(nil)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 40})
			got := updated.(appModel).layout
			if got != tt.want {
				t.Errorf("width=%d: layout = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

func TestStackedSidebarHeight(t *testing.T) {
	tests := []struct {
		name             string
		totalHeight      int
		fileCount        int
		contentItemCount int
		want             int
	}{
		{"clamps to minimum 8", 50, 0, 0, 8},
		{"clamps small file count to minimum", 50, 6, 0, 8},
		{"includes content items", 50, 3, 3, 9},
		{"no hard cap uses 35pct", 50, 15, 5, 17},
		{"grows with tall terminal", 80, 12, 0, 13},
		{"caps at 35% of total height", 20, 15, 0, 8},
		{"40% cap doesn't go below min 8", 8, 0, 0, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stackedSidebarHeight(tt.totalHeight, tt.fileCount, tt.contentItemCount, 0)
			if got != tt.want {
				t.Errorf("stackedSidebarHeight(%d, %d, %d, 0) = %d, want %d",
					tt.totalHeight, tt.fileCount, tt.contentItemCount, got, tt.want)
			}
		})
	}
}

func TestWidthAllocationHorizontal(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutHorizontal {
		t.Fatalf("expected horizontal layout at width 120")
	}

	// Sidebar should be clamped to [30, 50]
	if app.sidebar.width < 30 || app.sidebar.width > 50 {
		t.Errorf("sidebar.width = %d, want [30, 50]", app.sidebar.width)
	}

	// Diff view should get the remaining space, with at least 80 chars
	sidebarOuter := app.sidebar.width + 2 // border
	expectedDiffW := 120 - sidebarOuter - 2
	if app.diffView.width != expectedDiffW {
		t.Errorf("diffView.width = %d, want %d", app.diffView.width, expectedDiffW)
	}
	if app.diffView.width < 80 {
		t.Errorf("diffView.width = %d, want >= 80", app.diffView.width)
	}
}

func TestStackedLayoutRenderedHeight(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutStacked {
		t.Fatalf("expected stacked layout at width 60")
	}

	// Add enough files to fill the sidebar to its allocated height.
	for i := 0; i < 15; i++ {
		app.sidebar.files = append(app.sidebar.files, types.ChangedFile{
			Path:   fmt.Sprintf("file%d.go", i),
			Status: types.FileModified,
		})
	}
	recalcStackedLayout(&app)

	v := app.View()
	rendered := v.Content
	lineCount := strings.Count(rendered, "\n") + 1
	if lineCount != app.height {
		t.Errorf("rendered height = %d, want %d (terminal height)", lineCount, app.height)
	}
}

func TestWidthAllocationStacked(t *testing.T) {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutStacked {
		t.Fatalf("expected stacked layout at width 60")
	}

	expectedW := 60 - 2 // full width minus border
	if app.sidebar.width != expectedW {
		t.Errorf("sidebar.width = %d, want %d", app.sidebar.width, expectedW)
	}
	if app.diffView.width != expectedW {
		t.Errorf("diffView.width = %d, want %d", app.diffView.width, expectedW)
	}
}

func TestLayoutTransitionOnResize(t *testing.T) {
	m := NewApp(nil)

	// Start wide → horizontal
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)
	if app.layout != layoutHorizontal {
		t.Fatal("expected horizontal at width 120")
	}

	// Resize narrow → stacked
	updated, _ = app.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	app = updated.(appModel)
	if app.layout != layoutStacked {
		t.Fatal("expected stacked at width 60")
	}

	// Resize wide again → horizontal
	updated, _ = app.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	app = updated.(appModel)
	if app.layout != layoutHorizontal {
		t.Fatal("expected horizontal at width 110")
	}
}

func TestLayoutModeBreakpointCustomMinDiffWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  layoutMode
	}{
		{"wide terminal selects horizontal", 140, layoutHorizontal},
		{"exactly at custom breakpoint selects horizontal", 130, layoutHorizontal},
		{"below custom breakpoint selects stacked", 129, layoutStacked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewApp(nil)
			m.minDiffWidth = 100
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 40})
			got := updated.(appModel).layout
			if got != tt.want {
				t.Errorf("minDiffWidth=100, width=%d: layout = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

func TestWidthAllocationCustomMinDiffWidth(t *testing.T) {
	m := NewApp(nil)
	m.minDiffWidth = 60
	// At width 100, breakpoint is 60+30=90, so horizontal layout
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app := updated.(appModel)

	if app.layout != layoutHorizontal {
		t.Fatalf("expected horizontal layout at width 100 with minDiffWidth=60")
	}
	if app.diffView.width < 60 {
		t.Errorf("diffView.width = %d, want >= 60", app.diffView.width)
	}
}

func TestLayoutConfigForceSideBySide(t *testing.T) {
	m := NewApp(nil)
	m.layoutConfig = "side-by-side"

	// Even at a narrow width (below breakpoint), should be horizontal
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	app := updated.(appModel)
	if app.layout != layoutHorizontal {
		t.Errorf("side-by-side config at width 60: layout = %d, want %d (horizontal)", app.layout, layoutHorizontal)
	}
}

func TestLayoutConfigForceStacked(t *testing.T) {
	m := NewApp(nil)
	m.layoutConfig = "stacked"

	// Even at a wide width (above breakpoint), should be stacked
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)
	if app.layout != layoutStacked {
		t.Errorf("stacked config at width 120: layout = %d, want %d (stacked)", app.layout, layoutStacked)
	}
}

func TestLayoutConfigAutoDefault(t *testing.T) {
	// Empty string (no config) should behave like "auto"
	m := NewApp(nil)
	if m.layoutConfig != "" {
		t.Fatalf("expected empty layoutConfig from NewApp(nil), got %q", m.layoutConfig)
	}

	// Wide → horizontal
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)
	if app.layout != layoutHorizontal {
		t.Errorf("auto at width 120: want horizontal, got %d", app.layout)
	}

	// Narrow → stacked
	updated, _ = app.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	app = updated.(appModel)
	if app.layout != layoutStacked {
		t.Errorf("auto at width 60: want stacked, got %d", app.layout)
	}
}

func TestOverlayOn(t *testing.T) {
	t.Run("preserves base content on both sides", func(t *testing.T) {
		// Build a base tall enough for the min topPad of 5
		row := "LLLLLLLLLLLLLLLLLLLLRRRRRRRRRRRRRRRRRRRR" // 40 chars
		var rows []string
		for i := 0; i < 20; i++ {
			rows = append(rows, row)
		}
		base := strings.Join(rows, "\n")
		overlay := "MMMMMMMMMM" // 10 chars wide, 1 line tall

		result := overlayOn(base, overlay, 40, 20)
		lines := strings.Split(result, "\n")

		// Find the overlay line
		overlayIdx := -1
		for i, l := range lines {
			if strings.Contains(l, "MMMMMMMMMM") {
				overlayIdx = i
				break
			}
		}
		if overlayIdx < 0 {
			t.Fatal("overlay not found in any line")
		}

		targetLine := lines[overlayIdx]

		// Left side should have L's preserved
		if !strings.HasPrefix(targetLine, "LLLLLLLLLLLLLLL") {
			t.Errorf("left side not preserved: %q", targetLine)
		}
		// Right side should have R's preserved
		if !strings.HasSuffix(targetLine, "RRRRRRRRRRRRRRR") {
			t.Errorf("right side not preserved: %q", targetLine)
		}

		// Non-overlay lines should be unchanged
		if lines[0] != row {
			t.Errorf("non-overlay line changed: %q", lines[0])
		}
	})

	t.Run("pads short base lines", func(t *testing.T) {
		var rows []string
		for i := 0; i < 20; i++ {
			rows = append(rows, "short")
		}
		base := strings.Join(rows, "\n")
		overlay := "OVR"

		result := overlayOn(base, overlay, 20, 20)
		lines := strings.Split(result, "\n")

		// Find the overlay line
		overlayIdx := -1
		for i, l := range lines {
			if strings.Contains(l, "OVR") {
				overlayIdx = i
				break
			}
		}
		if overlayIdx < 0 {
			t.Fatal("overlay not found in any line")
		}
		// Left padding should exist even though base is short
		ovrPos := strings.Index(lines[overlayIdx], "OVR")
		leftW := lipgloss.Width(lines[overlayIdx][:ovrPos])
		if leftW < 8 {
			t.Errorf("left padding too short: got %d, want >= 8", leftW)
		}
	})

	t.Run("multi-line overlay", func(t *testing.T) {
		base := strings.Repeat("AAAAAAAAAAAAAAAAAAAAAAAAA\n", 19) + "AAAAAAAAAAAAAAAAAAAAAAAAA"
		overlay := strings.Join([]string{
			"┌──────┐",
			"│ test │",
			"└──────┘",
		}, "\n")

		result := overlayOn(base, overlay, 25, 20)
		lines := strings.Split(result, "\n")

		// All three overlay lines should be present
		found := 0
		for _, l := range lines {
			if strings.Contains(l, "test") {
				found++
			}
		}
		if found != 1 {
			t.Errorf("expected 1 line with 'test', found %d", found)
		}

		// Non-overlay lines should still be all A's
		if lines[0] != "AAAAAAAAAAAAAAAAAAAAAAAAA" {
			t.Errorf("first line changed: %q", lines[0])
		}
	})
}

func TestCalcModalWidth(t *testing.T) {
	tests := []struct {
		name        string
		screenWidth int
		maxWidth    int
		want        int
	}{
		{"wide screen uses 2/3", 120, 0, 80},
		{"medium screen uses 2/3", 99, 0, 66},
		{"min 65 kicks in", 90, 0, 65},
		{"narrow screen clamps to screen-10", 70, 0, 60},
		{"very narrow clamps to screen-10", 40, 0, 30},
		{"maxWidth caps result", 120, 60, 60},
		{"narrow with maxWidth", 70, 80, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcModalWidth(tt.screenWidth, tt.maxWidth)
			if got != tt.want {
				t.Errorf("CalcModalWidth(%d, %d) = %d, want %d",
					tt.screenWidth, tt.maxWidth, got, tt.want)
			}
		})
	}
}
