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
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// viewsTour is a tour whose second stop carries two views.
func viewsTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1.1", Title: "Where it starts", File: "a.go", LineStart: 5, Note: "The entry."},
		{ID: "1.2", Title: "Where it lands", File: "b.go", LineStart: 30, Note: "The write.",
			Related: []types.DocRef{{Doc: "a.go", StartLine: 5}, {Doc: "internal/deep/path/b.go", StartLine: 30}},
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
	m, _ = m.handleOnStopDone(onStopDoneMsg{id: "1.2", view: 2, err: os.ErrPermission})
	if want := "view 2 of 1.2 failed: permission denied"; m.statusBar.searchInfo != want {
		t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
	}
}

// anchoredCallsTour is callsTour with every stop on a line of the review.
// callsTour is a tour whose first stop calls three others.
func callsTour() *types.Walkthrough {
	return &types.Walkthrough{Stops: []types.WalkthroughStop{
		{ID: "2.1", Title: "The order route", Calls: []types.StopCall{
			{Stop: "4.1", Symbol: "reserveStock"},
			{Stop: "2.2", Symbol: "createOrder", Line: 48},
			{Stop: "3"},
		}},
		{ID: "2.2", Title: "Creating the order"},
		{ID: "3", Title: "The stock update"},
		{ID: "4.1", Title: "The reservation"},
	}}
}

func anchoredCallsTour() *types.Walkthrough {
	tour := callsTour()
	for i, at := range []struct {
		file string
		line int
	}{{"a.go", 5}, {"b.go", 30}, {"b.go", 50}, {"a.go", 20}} {
		tour.Stops[i].File, tour.Stops[i].LineStart = at.file, at.line
	}
	return tour
}

func TestCallCommand(t *testing.T) {
	m, _ := tourAppWith(t, anchoredCallsTour(), inertViewers(&types.Config{}))
	for _, c := range []struct{ arg, notice string }{{"9", "2.1 has calls 1-3"}, {"x", "2.1 has calls 1-3"}} {
		if m = typeCommand(t, m, "call "+c.arg); m.tour.index != 0 || m.statusBar.searchInfo != c.notice {
			t.Errorf(":call %s gave index %d %q, want no move and %q", c.arg, m.tour.index, m.statusBar.searchInfo, c.notice)
		}
	}
	m = typeCommand(t, m, "call") // the first call: 4.1
	if stop, _ := m.currentStop(); stop.ID != "4.1" || m.diffView.path != "a.go" || cursorLine(m) != 20 {
		t.Fatalf(":call left the tour on %s at %s:%d, want 4.1 at a.go:20", stop.ID, m.diffView.path, cursorLine(m))
	}
	if m = typeCommand(t, m, "call 1"); m.statusBar.searchInfo != "4.1 calls no other stop" {
		t.Errorf(":call from a stop with no calls said %q", m.statusBar.searchInfo)
	}
	// A call followed is a detour: back returns from it.
	if m = updateApp(t, m, backspaceKey); m.tour.index != 0 {
		t.Errorf("back after following a call left index %d, want the stop that called", m.tour.index)
	}
}

// onScreen finds text on the rendered screen and returns where a mouse click on
// its first cell arrives: its column and row, which are the View's
// (TestComputePaneLayoutMatchesRenderedView).
func onScreen(t *testing.T, m appModel, text string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(m.View().Content, "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, text); i >= 0 {
			return ansi.StringWidth(plain[:i]), row
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

// settle runs what resting on the current stop runs, to completion.
func settle(t *testing.T, m appModel) appModel {
	t.Helper()
	next, cmd := m.settleOnStop(tourSettledMsg{seq: m.tour.settle})
	return driveWithin(t, next, cmd, 0, 5*time.Second)
}

// screenText is the rendered screen without styling.
func screenText(m appModel) string { return ansi.Strip(m.View().Content) }

// captureRelated stands in for the tmux side of the related-files pane — the
// test pretends to be inside tmux, with a server that does not exist — and
// records each plan it is handed.
func captureRelated(t *testing.T) *[]relatedPanePlan {
	t.Helper()
	t.Setenv("TMUX", filepath.Join(t.TempDir(), "no-server")+",1,0")
	t.Setenv("TMUX_PANE", "%99")
	var plans []relatedPanePlan
	orig := showRelated
	showRelated = func(_ string, p relatedPanePlan) tea.Cmd {
		plans = append(plans, p)
		return nil
	}
	t.Cleanup(func() { showRelated = orig })
	return &plans
}

const recordingReset = `printf '%s|%s|%s\n' "$MONOCLE_STOP_ID" "${MONOCLE_REPO_ROOT+set}" "${MONOCLE_STOP_JSON:+json}" >> "$(dirname STATE)/resets"; ` +
	`printf '{"views": {}, "layout": "default"}' > STATE`

// longNoteApp is viewsAppIn's stop 1.2 with a note far longer than the pane.
func longNoteApp(t *testing.T) appModel {
	t.Helper()
	tour := viewsTour()
	var note strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&note, "Note line %d.\n", i)
	}
	tour.Stops[1].Note = note.String()
	m, _ := tourAppWith(t, tour, inertViewers(&types.Config{}))
	return pressKey(t, m, ".")
}

func wheel(x, y int, down bool) tea.MouseWheelMsg {
	b := tea.MouseWheelUp
	if down {
		b = tea.MouseWheelDown
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: b}
}

// A note longer than its pane starts at its top; the wheel over it scrolls the
// note, not the diff, down to its last line and back up; the title says how
// much is out of sight. The note is all the pane holds: no labels under it.
func TestTheWheelScrollsALongNote(t *testing.T) {
	m := longNoteApp(t)
	screen := screenText(m)
	if !strings.Contains(screen, "Note line 1.") || strings.Contains(screen, "Note line 60.") {
		t.Fatalf("the note should start at its top and not fit:\n%s", screen)
	}
	if title := rowWith(screen, "1.2 · Where it lands"); !strings.Contains(title, "↓ ") {
		t.Errorf("the title does not say how much is below: %q", title)
	}
	for _, label := range []string{"Calls:", "Related:", "View:", "Views:", "[1] a.go:5", "url Spec", "layout"} {
		if strings.Contains(screen, label) {
			t.Errorf("the note's pane still lists %q:\n%s", label, screen)
		}
	}
	nx, ny := onScreen(t, m, "Note line 2.")
	diffOffset := m.diffView.offset
	for i := 0; i < 30; i++ {
		m = updateApp(t, m, wheel(nx, ny, true))
	}
	screen = screenText(m)
	if !strings.Contains(screen, "Note line 60.") || strings.Contains(screen, "Note line 1.") {
		t.Fatalf("the wheel did not scroll the note to its end:\n%s", screen)
	}
	if m.diffView.offset != diffOffset {
		t.Error("the wheel over the note scrolled the diff")
	}
	if title := rowWith(screen, "1.2 · Where it lands"); !strings.Contains(title, "↑ ") || strings.Contains(title, "↓") {
		t.Errorf("at the end the title should say only what is above: %q", title)
	}
	// The note's last line sits over the pane's last row, the marker saying
	// it is the end; the box's bottom border is next.
	lines := strings.Split(screen, "\n")
	if _, ly := onScreen(t, m, "Note line 60."); !strings.Contains(lines[ly+1], "end of note") || !strings.Contains(lines[ly+2], "└") {
		t.Errorf("under the note's last line: %q, then %q; want the end marker, then the pane's bottom border", lines[ly+1], lines[ly+2])
	}
	for i := 0; i < 30; i++ {
		m = updateApp(t, m, wheel(nx, ny, false))
	}
	if !strings.Contains(screenText(m), "Note line 1.") {
		t.Error("the wheel did not scroll the note back up")
	}
}

// rowWith is the screen row holding text.
func rowWith(screen, text string) string {
	for _, l := range strings.Split(screen, "\n") {
		if strings.Contains(l, text) {
			return l
		}
	}
	return ""
}

// TestNoteThumb pins the notes pane's scrollbar: none when the note fits; a thumb
// sized by the share in view; at the top, the middle and the very bottom as the
// note scrolls.
func TestNoteThumb(t *testing.T) {
	cases := []struct {
		name                string
		rows, total, offset int
		want                []bool
	}{
		{"fits", 4, 4, 0, nil},
		{"top", 4, 8, 0, []bool{true, true, false, false}},
		{"middle", 4, 8, 2, []bool{false, true, true, false}},
		{"end", 4, 8, 4, []bool{false, false, true, true}},
		{"tiny share keeps one row", 4, 100, 0, []bool{true, false, false, false}},
	}
	for _, c := range cases {
		got := noteThumb(c.rows, c.total, c.offset)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: noteThumb(%d,%d,%d) = %v, want %v", c.name, c.rows, c.total, c.offset, got, c.want)
		}
	}
}

// A note taller than its pane shows a scrollbar thumb on the row it is at; a note
// that fits shows none.
func TestALongNoteShowsAScrollbar(t *testing.T) {
	m := longNoteApp(t)
	top := rowWith(screenText(m), "Note line 1.")
	if !strings.HasSuffix(strings.TrimRight(top, " │"), "█") && !strings.Contains(top, "█") {
		t.Errorf("the note's first row has no scrollbar thumb at the top: %q", top)
	}
	nx, ny := onScreen(t, m, "Note line 2.")
	for i := 0; i < 30; i++ {
		m = updateApp(t, m, wheel(nx, ny, true))
	}
	if row := rowWith(screenText(m), "Note line 1."); row != "" {
		t.Fatalf("the note did not scroll: %q", row)
	}
	if last := rowWith(screenText(m), "Note line 60."); !strings.Contains(last, "█") {
		t.Errorf("at the end the thumb should sit on the last note row: %q", last)
	}
}

// The note's scroll hint is right-justified in the title bar, apart from the title.
func TestTheScrollHintSitsAtTheRightOfTheTitle(t *testing.T) {
	m := longNoteApp(t)
	row := rowWith(screenText(m), "1.2 · Where it lands")
	inner := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(row, " "), "│"), " ")
	if !strings.HasSuffix(inner, "lines") {
		t.Errorf("the hint is not at the right end of the title bar: %q", row)
	}
	if i := strings.Index(row, "Where it lands"); !strings.Contains(row[i:], "     ↓") {
		t.Errorf("the hint is not set apart from the title: %q", row)
	}
}

// = cycles the doc pane against the diff: from the diff, the diff biggest (the
// doc smallest), then the doc biggest, then the usual split; from the doc pane,
// the doc biggest first.
func TestPaneSizeCycles(t *testing.T) {
	m := longNoteApp(t)
	usual := m.docPane.height
	m = pressKey(t, m, "=")
	small := m.docPane.height
	m = pressKey(t, m, "=")
	big := m.docPane.height
	m = pressKey(t, m, "=")
	if !(small < usual && big > usual && m.docPane.height == usual) {
		t.Errorf("from the diff: usual %d → %d → %d → %d, want smaller, bigger, usual", usual, small, big, m.docPane.height)
	}
	m.focus = focusDoc
	m = pressKey(t, m, "=")
	if m.docPane.height <= usual {
		t.Errorf("from the doc pane the first press should make it biggest: %d (usual %d)", m.docPane.height, usual)
	}
}
