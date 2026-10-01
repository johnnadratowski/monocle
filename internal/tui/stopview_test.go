package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// viewsTour is a tour whose second stop carries two views.
func viewsTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1.1", Title: "Where it starts", File: "a.go", LineStart: 5, Note: "The entry."},
		{ID: "1.2", Title: "Where it lands", File: "b.go", LineStart: 30, Note: "The write.",
			Views: []types.StopView{
				{Kind: types.StopViewVideo, Target: "demo.webm", Label: "Demo"},
				{Kind: types.StopViewURL, Target: "https://example.test/spec", Label: "Spec"},
			}},
	}}
}

// inertViewers keeps a test from ever launching a real viewer (the default media
// viewer is a browser): `true` is not a GUI launcher, so it would be handed to
// the Bubble Tea runtime to run, and there is none in a test.
func inertViewers(cfg *types.Config) *types.Config {
	cfg.MediaViewer, cfg.MarkdownViewer = "true", "true"
	return cfg
}

// recordRuns is an on-stop command that appends one line per run to out: the
// stop, and the view it was asked for ("-" when it was not asked for one).
func recordRuns(out string) string {
	return `printf '%s|%s|%s\n' "$MONOCLE_STOP_ID" "${MONOCLE_VIEW_INDEX:--}" "${MONOCLE_VIEW_NAME:--}" >> ` + out
}

// waitForRuns waits for the on-stop command to have run n times and returns
// its runs. The command runs in the background, as it does in the app.
func waitForRuns(t *testing.T, out string, n int) []string {
	t.Helper()
	var runs []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		data, _ := os.ReadFile(out)
		runs = strings.Fields(string(data))
		if len(runs) >= n {
			break
		}
	}
	return runs
}

func skipWithoutSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the on-stop command runs under sh here")
	}
}

func TestViewCommandAsksTheOnStopCommandForTheView(t *testing.T) {
	skipWithoutSh(t)
	out := filepath.Join(t.TempDir(), "runs")
	m, _ := tourAppWith(t, viewsTour(), inertViewers(&types.Config{WalkthroughOnStop: recordRuns(out)}))
	m = pressKey(t, m, ".")
	m = typeCommand(t, m, "view 2")
	waitForRuns(t, out, 1)
	m = typeCommand(t, m, "view")
	got := waitForRuns(t, out, 2)
	if want := []string{"1.2|2|view2", "1.2|1|view"}; !reflect.DeepEqual(got, want) {
		t.Errorf("on-stop runs %q, want %q", got, want)
	}
	_ = m
}

func TestStopViewName(t *testing.T) {
	for n, want := range map[int]string{1: "view", 2: "view2", 10: "view10"} {
		if got := stopViewName(n); got != want {
			t.Errorf("stopViewName(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAFailedViewRunSaysWhichView(t *testing.T) {
	m := appModel{}
	m = m.handleOnStopDone(onStopDoneMsg{id: "1.2", view: 2, err: os.ErrPermission})
	if want := "view 2 of 1.2 failed: permission denied"; m.statusBar.searchInfo != want {
		t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
	}
}

func TestStopLinks(t *testing.T) {
	got := stopLinks(types.WalkthroughStop{Views: []types.StopView{
		{Kind: "image", Target: "shots/x.png"},
		{Kind: "url", Target: "https://x.test", Label: "Spec"},
	}})
	want := []noteLink{{label: "[1] image x.png"}, {label: "[2] url Spec"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := stopLinks(types.WalkthroughStop{}); got != nil {
		t.Errorf("a stop with no views has links %+v", got)
	}
}

// TestLayoutLinksKeepsEveryLabelWhole checks, at every width, that each label
// lands on one row exactly where its hit says — so a click there finds it —
// and that no row is wider than the pane.
func TestLayoutLinksKeepsEveryLabelWhole(t *testing.T) {
	links := []noteLink{{label: "[1] video Before / after"}, {label: "[2] url Spec"}, {label: "[3] image A much longer label for the table structure"}}
	for width := 12; width <= 140; width++ {
		rows, hits := layoutLinks(links, width)
		if len(hits) != len(links) {
			t.Fatalf("width %d: %d hits for %d links", width, len(hits), len(links))
		}
		for _, r := range rows {
			if w := lipgloss.Width(r); w > width-1 && width > 12 {
				t.Errorf("width %d: row %q is %d wide", width, ansi.Strip(r), w)
			}
		}
		for i, h := range hits {
			if h.n != i+1 {
				t.Errorf("width %d: hit %d is for link %d", width, i, h.n)
			}
			got := ansi.Strip(ansi.Cut(rows[h.line], h.start, h.end))
			want := links[i].label
			if lipgloss.Width(want) > h.end-h.start {
				want = ansi.Truncate(want, h.end-h.start, "…")
			}
			if got != want {
				t.Errorf("width %d: label %d reads %q at its hit, want %q", width, i+1, got, want)
			}
		}
	}
}

func TestLayoutLinksStyle(t *testing.T) {
	rows, _ := layoutLinks([]noteLink{{label: "[1] video Demo"}}, 80)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor)).Bold(true).Underline(true).Render("[1] video Demo")
	if len(rows) != 1 || !strings.Contains(rows[0], label) {
		t.Errorf("rows %q, want the label bold, underlined, in the accent", rows)
	}
	if got := ansi.Strip(rows[0]); got != " Views: [1] video Demo  "+linkHint {
		t.Errorf("row reads %q", got)
	}
}

// onScreen finds text on the rendered screen and returns where a mouse click on
// its first cell arrives: its column, and its row plus the one-row origin offset
// every mouse coordinate carries (TestComputePaneLayoutMatchesRenderedView).
func onScreen(t *testing.T, m appModel, text string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(m.View().Content, "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, text); i >= 0 {
			return ansi.StringWidth(plain[:i]), row + 1
		}
	}
	t.Fatalf("%q is not on screen", text)
	return 0, 0
}

func leftClick(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// layouts are the ways the doc pane can sit on screen: beside the sidebar,
// under it, and with it hidden in either.
var layouts = []struct {
	name          string
	width         int
	sidebarHidden bool
}{
	{"side by side", 140, false},
	{"stacked", 90, false},
	{"side by side, sidebar hidden", 140, true},
	{"stacked, sidebar hidden", 90, true},
}

// viewsAppIn is tourAppWith(viewsTour) laid out one of those ways, on stop 1.2.
func viewsAppIn(t *testing.T, cfg *types.Config, width int, sidebarHidden bool) appModel {
	t.Helper()
	m, _ := tourAppWith(t, viewsTour(), inertViewers(cfg))
	m = updateApp(t, m, tea.WindowSizeMsg{Width: width, Height: 44})
	m.sidebarHidden = sidebarHidden
	recalcPaneDimensions(&m)
	return pressKey(t, m, ".")
}

func TestDocPaneRegionIsWhereItRenders(t *testing.T) {
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			m := viewsAppIn(t, &types.Config{}, l.width, l.sidebarHidden)
			x, y := onScreen(t, m, "1.2 · Where it lands") // the title, after a one-column margin
			got := computePaneLayout(&m).doc
			if want := (paneRegion{x - 1, y, m.docPane.width, m.docPane.height}); got != want {
				t.Errorf("doc region %+v, want %+v", got, want)
			}
		})
	}
}

func TestClickingAViewLabelIsViewN(t *testing.T) {
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			m := viewsAppIn(t, &types.Config{}, l.width, l.sidebarHidden)
			focus, cursor := m.focus, m.diffView.cursor
			for i, label := range []string{"[1] video Demo", "[2] url Spec"} {
				x, y := onScreen(t, m, label)
				want := m.executeCommand(fmt.Sprintf("view %d", i+1))()
				for _, dx := range []int{0, lipgloss.Width(label) - 1} {
					next, cmd := m.Update(leftClick(x+dx, y))
					if cmd == nil {
						t.Fatalf("a click on %q (+%d) did nothing", label, dx)
					}
					if got := cmd(); !reflect.DeepEqual(got, want) {
						t.Errorf("a click on %q (+%d) sent %#v, want %#v — what :view %d sends", label, dx, got, want, i+1)
					}
					if app := next.(appModel); app.focus != focus || app.diffView.cursor != cursor {
						t.Errorf("a click on %q moved focus or the diff cursor", label)
					}
				}
			}
			// Off the labels a click in the pane does nothing: past the end
			// of one, the separator before the next, the hint, the note, the title.
			x1, y1 := onScreen(t, m, "[1] video Demo")
			x2, _ := onScreen(t, m, "[2] url Spec")
			hx, hy := onScreen(t, m, linkHint)
			nx, ny := onScreen(t, m, "The write.")
			tx, ty := onScreen(t, m, "1.2 · Where it lands")
			for _, p := range [][2]int{{x1 + lipgloss.Width("[1] video Demo"), y1}, {x2 - 2, y1}, {x2 - 1, y1}, {hx, hy}, {nx, ny}, {tx, ty}} {
				if _, cmd := m.Update(leftClick(p[0], p[1])); cmd != nil {
					t.Errorf("a click at %v, off the labels, did something: %#v", p, cmd())
				}
			}
		})
	}
}

func TestClickingAViewLabelAsksTheOnStopCommandForIt(t *testing.T) {
	skipWithoutSh(t)
	out := filepath.Join(t.TempDir(), "runs")
	m := viewsAppIn(t, &types.Config{WalkthroughOnStop: recordRuns(out)}, 140, false)
	x, y := onScreen(t, m, "[2] url Spec")
	m = updateApp(t, m, leftClick(x, y))
	if got, want := waitForRuns(t, out, 1), []string{"1.2|2|view2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("on-stop runs %q, want %q", got, want)
	}
}

func TestAClickFindsALabelInAScrolledNote(t *testing.T) {
	tour := viewsTour()
	tour.Stops[1].Note = strings.Repeat("A long note.\n\n", 30)
	m, _ := tourAppWith(t, tour, inertViewers(&types.Config{}))
	m = pressKey(t, m, ".")
	for i := 0; i < 100; i++ {
		m.docPane.scrollDown()
	}
	x, y := onScreen(t, m, "[2] url Spec")
	_, cmd := m.Update(leftClick(x, y))
	if cmd == nil {
		t.Fatal("a click on a label scrolled into view did nothing")
	}
	if got := cmd(); !reflect.DeepEqual(got, tourViewMsg{arg: "2"}) {
		t.Errorf("sent %#v, want view 2", got)
	}
}

func TestMouseOffMeansNoClick(t *testing.T) {
	off := false
	m := viewsAppIn(t, &types.Config{Mouse: &off}, 140, false)
	x, y := onScreen(t, m, "[2] url Spec")
	if _, cmd := m.Update(leftClick(x, y)); cmd != nil {
		t.Errorf("with mouse off a click did something: %#v", cmd())
	}
}
