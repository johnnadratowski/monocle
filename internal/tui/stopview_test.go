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

func TestStopLinkGroups(t *testing.T) {
	got := stopLinkGroups(types.WalkthroughStop{
		Related: []types.DocRef{{Doc: "a.go", StartLine: 4}, {Doc: "b.go"}},
		Views: []types.StopView{
			{Kind: "image", Target: "shots/x.png"},
			{Kind: "url", Target: "https://x.test", Label: "Spec"},
		},
	}, &tourStatus{views: []viewState{viewOpen}})
	want := []linkGroup{
		{head: "Related:", hint: "click, or :related N", links: []noteLink{
			{label: "[1] a.go:4", act: tourRelatedMsg{arg: "1"}, middle: true},
			{label: "[2] b.go", act: tourRelatedMsg{arg: "2"}, middle: true},
		}},
		{head: "Views:", hint: "click, or :view N", links: []noteLink{
			{label: "[1] image x.png", act: tourViewMsg{arg: "1"}, state: viewOpen},
			{label: "[2] url Spec", act: tourViewMsg{arg: "2"}},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if rows, _ := layoutGroups(stopLinkGroups(types.WalkthroughStop{}, nil), 80); rows != nil {
		t.Errorf("a stop with nothing to link has rows %q", rows)
	}
	// A saved layout adds the label that resets it; a default one does not.
	layout := linkGroup{head: "Layout:", lead: "saved", hint: ":layout reset",
		links: []noteLink{{label: "reset", act: tourLayoutMsg{arg: "reset"}}}}
	saved := stopLinkGroups(types.WalkthroughStop{}, &tourStatus{layoutSaved: true})
	if len(saved) != 3 || !reflect.DeepEqual(saved[2], layout) {
		t.Errorf("with a saved layout: %+v", saved)
	}
	if got := stopLinkGroups(types.WalkthroughStop{}, &tourStatus{}); len(got) != 2 {
		t.Errorf("with the default layout: %+v", got)
	}
}

// TestLayoutKeepsEveryLabelWhole checks, at every width, that each label lands
// on one row exactly where its hit says — so a click there finds it, and sends
// what the label sends — and that no row is wider than the pane.
func TestLayoutKeepsEveryLabelWhole(t *testing.T) {
	views := []noteLink{
		{label: "[1] video Before / after", act: tourViewMsg{arg: "1"}},
		{label: "[2] url Spec", act: tourViewMsg{arg: "2"}},
		{label: "[3] image A much longer label for the table structure", act: tourViewMsg{arg: "3"}},
	}
	t.Run("unmarked", func(t *testing.T) {
		checkLinkLayout(t, linkGroup{head: "Views:", links: views, hint: "click, or :view N"})
	})
	marked := append([]noteLink(nil), views...)
	marked[0].state, marked[1].state, marked[2].state = viewNotOpened, viewOpen, viewHidden
	t.Run("marked", func(t *testing.T) { checkLinkLayout(t, linkGroup{head: "Views:", links: marked}) })
	related := []noteLink{
		{label: "[1] ui-web-b2b/ui/components/move-funds/move-funds.logic.js:66", act: tourRelatedMsg{arg: "1"}, middle: true},
		{label: "[2] server/a.go:4", act: tourRelatedMsg{arg: "2"}, middle: true},
	}
	t.Run("paths", func(t *testing.T) { checkLinkLayout(t, linkGroup{head: "Related:", links: related}) })
	t.Run("lead", func(t *testing.T) {
		checkLinkLayout(t, linkGroup{head: "Layout:", lead: "saved", links: []noteLink{{label: "reset", act: tourViewMsg{arg: "x"}}}})
	})
}

func checkLinkLayout(t *testing.T, g linkGroup) {
	t.Helper()
	for width := 12; width <= 140; width++ {
		rows, hits := layoutGroups([]linkGroup{g}, width)
		if len(hits) != len(g.links) {
			t.Fatalf("width %d: %d hits for %d links", width, len(hits), len(g.links))
		}
		for _, r := range rows {
			if w := lipgloss.Width(r); w > width-1 && width > 12 {
				t.Errorf("width %d: row %q is %d wide", width, ansi.Strip(r), w)
			}
		}
		for i, h := range hits {
			l := g.links[i]
			if !reflect.DeepEqual(h.act, l.act) {
				t.Errorf("width %d: hit %d sends %#v, want %#v", width, i, h.act, l.act)
			}
			got := ansi.Strip(ansi.Cut(rows[h.line], h.start, h.end))
			if got != l.label && (!strings.Contains(got, "…") || lipgloss.Width(got) != h.end-h.start) {
				t.Errorf("width %d: label %d reads %q at its hit, want %q or a cut of it", width, i+1, got, l.label)
			}
			// A path cut to fit keeps its file name and line, room allowing.
			if base := filepath.Base(l.label); l.middle && got != l.label && h.end-h.start > lipgloss.Width(base)+6 && !strings.HasSuffix(got, base) {
				t.Errorf("width %d: path label %d was cut to %q, losing %q", width, i+1, got, base)
			}
			// Its marker follows it, on the same row and unclickable — unless
			// the pane is too narrow for both, when the label is cut first.
			if marker := ansi.Strip(l.state.marker()); marker != "" {
				after := ansi.Strip(ansi.Cut(rows[h.line], h.end, h.end+1+len(marker)))
				if after != " "+marker && (got == l.label || width >= 40) {
					t.Errorf("width %d: label %d is followed by %q, want its marker %q", width, i+1, after, marker)
				}
			}
		}
	}
}

func TestLayoutGroupsStyleAndOrder(t *testing.T) {
	groups := []linkGroup{
		{head: "Related:", links: []noteLink{{label: "[1] a.go:4"}}, hint: "click, or :related N"},
		{head: "Views:", links: []noteLink{{label: "[1] video Demo"}}, hint: "click, or :view N"},
		{head: "Layout:", lead: "saved", links: []noteLink{{label: "reset"}}},
	}
	rows, _ := layoutGroups(groups, 80)
	var plain []string
	for _, r := range rows {
		plain = append(plain, ansi.Strip(r))
	}
	want := []string{" Related: [1] a.go:4  click, or :related N", " Views: [1] video Demo  click, or :view N", " Layout: saved · reset"}
	if !reflect.DeepEqual(plain, want) {
		t.Errorf("rows read %q\nwant      %q", plain, want)
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor)).Bold(true).Underline(true)
	for i, l := range []string{"[1] a.go:4", "[1] video Demo", "reset"} {
		if !strings.Contains(rows[i], label.Render(l)) {
			t.Errorf("row %d: %q is not bold, underlined, in the accent", i, l)
		}
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
			hx, hy := onScreen(t, m, "click, or :view N")
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

func TestParseViewStatus(t *testing.T) {
	for _, tc := range []struct {
		out  string
		n    int
		want *tourStatus
	}{
		{`{"views": {"view": "open", "view2": "hidden", "view3": "closed"}, "layout": "saved"}`, 3,
			&tourStatus{views: []viewState{viewOpen, viewHidden, viewNotOpened}, layoutSaved: true}},
		{"{\"views\": {\"view2\": \"open\"}, \"layout\": \"default\"}\n", 2,
			&tourStatus{views: []viewState{viewNotOpened, viewOpen}}},
		{`{"views": {"view": "shut", "view9": "open"}}`, 1, &tourStatus{views: []viewState{viewNotOpened}}},
		{`{"views": {}, "layout": "weird"}`, 2, &tourStatus{views: []viewState{viewNotOpened, viewNotOpened}}},
		{`{"views": {}, "layout": "saved"}`, 0, &tourStatus{views: []viewState{}, layoutSaved: true}},
	} {
		got, err := parseViewStatus([]byte(tc.out), tc.n)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseViewStatus(%q) = %+v, %v; want %+v", tc.out, got, err, tc.want)
		}
	}
	// Anything without a "views" object of strings says nothing — the flat
	// shape of an earlier draft included, so it cannot be misread as all closed.
	for _, out := range []string{"", "open", "null", `["open"]`, `{}`, `{"view": "open"}`, `{"layout": "saved"}`,
		`{"views": {"view": 1}}`, `{"views": ["open"]}`, `{"views": {}, "layout": 1}`, `{"views": {}`} {
		if got, err := parseViewStatus([]byte(out), 1); err == nil {
			t.Errorf("parseViewStatus(%q) = %v, want an error", out, got)
		}
	}
}

// statusApp is viewsAppIn with a view-status command (and, if given, an
// on-stop command) on stop 1.2, before anything has been asked about it.
func statusApp(t *testing.T, status, onStop string) appModel {
	t.Helper()
	skipWithoutSh(t)
	return viewsAppIn(t, &types.Config{WalkthroughViewStatus: status, WalkthroughOnStop: onStop}, 140, false)
}

// settle runs what resting on the current stop runs, to completion.
func settle(t *testing.T, m appModel) appModel {
	t.Helper()
	next, cmd := m.settleOnStop(tourSettledMsg{seq: m.tour.settle})
	return driveWithin(t, next, cmd, 0, 5*time.Second)
}

// screenText is the rendered screen without styling.
func screenText(m appModel) string { return ansi.Strip(m.View().Content) }

func TestViewMarkersComeFromTheStatusCommand(t *testing.T) {
	m := settle(t, statusApp(t, `printf '{"views": {"view": "open", "view2": "hidden"}, "layout": "default"}'`, ""))
	screen := screenText(m)
	for _, want := range []string{"[1] video Demo (open)", "[2] url Spec (hidden)"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	// The labels are still where a click finds them.
	x, y := onScreen(t, m, "[2] url Spec")
	if _, cmd := m.Update(leftClick(x, y)); cmd == nil || !reflect.DeepEqual(cmd(), tourViewMsg{arg: "2"}) {
		t.Error("a marked label is not clickable")
	}
}

func TestNoUsableStatusMeansNoMarker(t *testing.T) {
	for name, status := range map[string]string{
		"no command":      "",
		"a failure":       `printf '{"views": {"view": "open"}, "layout": "saved"}'; exit 1`,
		"not JSON":        `echo open`,
		"the flat shape":  `echo '{"view": "open"}'`,
		"a hung command":  `sleep 5`,
		"over 500ms":      `sleep 1; echo '{"views": {"view": "open"}, "layout": "saved"}'`,
		"stderr is noise": `echo '{"views": {"view": "open"}}' >&2`,
	} {
		t.Run(name, func(t *testing.T) {
			m := settle(t, statusApp(t, status, ""))
			if screen := screenText(m); strings.Contains(screen, "(open)") || strings.Contains(screen, "(not opened)") || strings.Contains(screen, "Layout:") {
				t.Errorf("a marker or the layout label with %s:\n%s", name, screen)
			}
		})
	}
}

func TestAFailedAskClearsTheMarkers(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(state, []byte(`{"views": {"view": "open"}, "layout": "saved"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := settle(t, statusApp(t, "cat "+state, ""))
	if !strings.Contains(screenText(m), "(open)") || !strings.Contains(screenText(m), "Layout: saved") {
		t.Fatalf("no marker and layout label to begin with:\n%s", screenText(m))
	}
	_ = os.Remove(state)
	m = settle(t, m)
	if strings.Contains(screenText(m), "(open)") || strings.Contains(screenText(m), "Layout:") {
		t.Errorf("a marker or the layout label outlived the status command failing:\n%s", screenText(m))
	}
}

// With an on-stop command, the ask waits for it: it is about to open this
// stop's views, and asking first would mark the last stop's windows.
func TestTheStatusAskFollowsTheOnStopCommand(t *testing.T) {
	dir := t.TempDir()
	state, out := filepath.Join(dir, "state"), filepath.Join(dir, "runs")
	onStop := recordRuns(out) + `; printf '{"views": {"view": "open"}}' > ` + state
	m := statusApp(t, "cat "+state, onStop)
	next, cmd := m.settleOnStop(tourSettledMsg{seq: m.tour.settle})
	if next.tour.statusSeq != m.tour.statusSeq {
		t.Error("asked for the view status before the on-stop command ran")
	}
	m = driveWithin(t, next, cmd, 0, 5*time.Second)
	if got := waitForRuns(t, out, 1); !reflect.DeepEqual(got, []string{"1.2|-|-"}) {
		t.Fatalf("on-stop runs %q", got)
	}
	if screen := screenText(m); !strings.Contains(screen, "[1] video Demo (open)") || !strings.Contains(screen, "[2] url Spec (not opened)") {
		t.Errorf("markers after the on-stop command:\n%s", screen)
	}
}

func TestClickingAViewReasksItsStatus(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	// The on-stop command opens the view it is asked for; the status command
	// reports it open.
	onStop := `[ -n "$MONOCLE_VIEW_NAME" ] && printf '{"views": {"%s": "open"}}' "$MONOCLE_VIEW_NAME" > ` + state
	m := settle(t, statusApp(t, "cat "+state+` 2>/dev/null || echo '{"views": {}}'`, onStop))
	if !strings.Contains(screenText(m), "[2] url Spec (not opened)") {
		t.Fatalf("before the click:\n%s", screenText(m))
	}
	x, y := onScreen(t, m, "[2] url Spec")
	next, cmd := m.Update(leftClick(x, y))
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if screen := screenText(m); !strings.Contains(screen, "[1] video Demo (not opened)") || !strings.Contains(screen, "[2] url Spec (open)") {
		t.Errorf("after clicking view 2:\n%s", screen)
	}
}

// A restore runs no on-stop command, so the status is asked at once — on a
// stop with no views too, since the answer also says whether the layout is
// saved.
func TestStatusIsAskedOnRestore(t *testing.T) {
	skipWithoutSh(t)
	m, _ := tourAppWith(t, viewsTour(), inertViewers(&types.Config{WalkthroughViewStatus: `echo '{"views": {}}'`}))
	if stop, _ := m.currentStop(); len(stop.Views) != 0 || m.tour.statusSeq != 1 {
		t.Errorf("restoring stop %s (%d views) asked %d times, want once", stop.ID, len(stop.Views), m.tour.statusSeq)
	}
}

// The status command gets the on-stop command's environment: the stop, the
// repo, and the stop as JSON.
func TestTheStatusCommandsEnvironment(t *testing.T) {
	status := `[ "$MONOCLE_STOP_ID" = 1.2 ] && [ "${MONOCLE_REPO_ROOT+set}" = set ] &&
		case "$MONOCLE_STOP_JSON" in *'"id":"1.2"'*) echo '{"views": {"view": "open"}}';; esac`
	m := settle(t, statusApp(t, status, ""))
	if !strings.Contains(screenText(m), "[1] video Demo (open)") {
		t.Errorf("the status command did not get its environment:\n%s", screenText(m))
	}
}

func TestAStaleStatusAnswerIsDropped(t *testing.T) {
	m := statusApp(t, `echo '{"views": {}}'`, "")
	m.tour.statusSeq = 2
	for name, msg := range map[string]viewStatusMsg{
		"an older ask":  {seq: 1, stop: "1.2", status: &tourStatus{views: []viewState{viewOpen, viewOpen}}},
		"another stop":  {seq: 2, stop: "1.1", status: &tourStatus{views: []viewState{viewOpen, viewOpen}}},
		"the right one": {seq: 2, stop: "1.2", status: &tourStatus{views: []viewState{viewHidden, viewHidden}}},
	} {
		got := screenText(m.handleViewStatus(msg))
		if marked := strings.Contains(got, "(open)") || strings.Contains(got, "(hidden)"); marked != (name == "the right one") {
			t.Errorf("%s: marked=%v\n%s", name, marked, got)
		}
	}
}

func TestClickingARelatedLabelIsRelatedN(t *testing.T) {
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			m := viewsAppIn(t, &types.Config{}, l.width, l.sidebarHidden)
			focus, cursor := m.focus, m.diffView.cursor
			for i, label := range []string{"[1] a.go:5", "[2] internal/deep/path/b.go:30"} {
				x, y := onScreen(t, m, label)
				want := m.executeCommand(fmt.Sprintf("related %d", i+1))()
				for _, dx := range []int{0, lipgloss.Width(label) - 1} {
					next, cmd := m.Update(leftClick(x+dx, y))
					if cmd == nil {
						t.Fatalf("a click on %q (+%d) did nothing", label, dx)
					}
					if got := cmd(); !reflect.DeepEqual(got, want) {
						t.Errorf("a click on %q (+%d) sent %#v, want %#v — what :related %d sends", label, dx, got, want, i+1)
					}
					if app := next.(appModel); app.focus != focus || app.diffView.cursor != cursor {
						t.Errorf("a click on %q moved focus or the diff cursor", label)
					}
				}
			}
		})
	}
}

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

func TestRelatedNBringsUpEveryFileWithNActive(t *testing.T) {
	m := viewsAppIn(t, &types.Config{Editor: "nvim"}, 140, false)
	plans := captureRelated(t) // after: tourApp clears TMUX for itself

	// Arriving at the stop opens its related files on the first, as before,
	// and leaves a zoomed window as it is.
	m = settle(t, m)
	// Asking for file 2 opens them all again with file 2's window active, and
	// unzooms the window so the pane can be seen.
	m = typeCommand(t, m, "related 2")
	if len(*plans) != 2 {
		t.Fatalf("%d plans, want the stop's and :related 2's", len(*plans))
	}
	files := []string{"a.go", "internal/deep/path/b.go"}
	for i, want := range []struct {
		last   string
		reveal bool
	}{{"1wincmd w", false}, {"2wincmd w", true}} {
		p := (*plans)[i]
		c := p.argv[len(p.argv)-1]
		if !reflect.DeepEqual(p.argv[3:5], files) || !strings.HasSuffix(c, "|"+want.last) || p.reveal != want.reveal {
			t.Errorf("plan %d: argv %q reveal %v; want both files, ending %q, reveal %v", i, p.argv, p.reveal, want.last, want.reveal)
		}
		// Keyboard focus stays with Monocle unless editor_focus says otherwise.
		if p.owner != "%99" || p.focus {
			t.Errorf("plan %d: owner %q focus %v, want beside %%99 without taking focus", i, p.owner, p.focus)
		}
	}
	_ = m
}

func TestRelatedCommandRefusesWhatItCannotOpen(t *testing.T) {
	m := viewsAppIn(t, &types.Config{}, 140, false) // on 1.2, outside tmux
	for arg, want := range map[string]string{
		"3": "1.2 has related files 1-2",
		"0": "1.2 has related files 1-2",
		"x": "1.2 has related files 1-2",
		"2": "related files: " + errRelatedNeedsTmux.Error(),
	} {
		if got := typeCommand(t, m, "related "+arg).statusBar.searchInfo; got != want {
			t.Errorf(":related %s said %q, want %q", arg, got, want)
		}
	}
	if got := typeCommand(t, pressKey(t, m, ","), "related").statusBar.searchInfo; got != "1.1 has no related files" {
		t.Errorf(":related on a stop with none said %q", got)
	}
	if got := typeCommand(t, pressKey(t, m, "W"), "related").statusBar.searchInfo; got != "no tour stop to open a related file from" {
		t.Errorf(":related with the tour off said %q", got)
	}
}

// layoutApp is statusApp on stop 1.2 with a view status read from a file, a
// layout-reset command that records its runs and puts the layout back, and
// the status asked once: the layout is saved.
func layoutApp(t *testing.T, reset string) (appModel, string) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.WriteFile(state, []byte(`{"views": {}, "layout": "saved"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := statusApp(t, "cat "+state, "")
	cfg := m.engine.GetConfig()
	cfg.WalkthroughLayoutReset = strings.ReplaceAll(reset, "STATE", state)
	return settle(t, m), dir
}

const recordingReset = `printf '%s|%s|%s\n' "$MONOCLE_STOP_ID" "${MONOCLE_REPO_ROOT+set}" "${MONOCLE_STOP_JSON:+json}" >> "$(dirname STATE)/resets"; ` +
	`printf '{"views": {}, "layout": "default"}' > STATE`

func TestASavedLayoutOffersAReset(t *testing.T) {
	m, _ := layoutApp(t, recordingReset)
	if !strings.Contains(screenText(m), "Layout: saved · reset  :layout reset") {
		t.Fatalf("no reset label:\n%s", screenText(m))
	}
	x, y := onScreen(t, m, "· reset")
	_, cmd := m.Update(leftClick(x+2, y))
	if cmd == nil {
		t.Fatal("a click on reset did nothing")
	}
	if got, want := cmd(), m.executeCommand("layout reset")(); !reflect.DeepEqual(got, want) {
		t.Errorf("a click on reset sent %#v, want %#v — what :layout reset sends", got, want)
	}
	// "Layout: saved" itself is not a label.
	lx, ly := onScreen(t, m, "Layout: saved")
	if _, cmd := m.Update(leftClick(lx+9, ly)); cmd != nil {
		t.Errorf("a click on \"saved\" did something: %#v", cmd())
	}
}

func TestLayoutResetRunsTheCommandThenAsksAgain(t *testing.T) {
	m, dir := layoutApp(t, recordingReset)
	asked := m.tour.statusSeq
	next, cmd := m.Update(tourLayoutMsg{arg: "reset"})
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	runs, _ := os.ReadFile(filepath.Join(dir, "resets"))
	if string(runs) != "1.2|set|json\n" {
		t.Errorf("reset runs %q, want one with the status command's environment", runs)
	}
	if m.tour.statusSeq != asked+1 || m.statusBar.searchInfo != "layout reset" {
		t.Errorf("after the reset: asked %d more times, status %q", m.tour.statusSeq-asked, m.statusBar.searchInfo)
	}
	if strings.Contains(screenText(m), "Layout:") {
		t.Errorf("the label outlived the layout going back to the default:\n%s", screenText(m))
	}
}

func TestLayoutCommand(t *testing.T) {
	m, _ := layoutApp(t, `echo "no such scene" >&2; exit 4`)
	for arg, want := range map[string]string{
		"":        "layout: saved — :layout reset puts it back",
		"frobble": "usage: :layout [reset]",
	} {
		if got := typeCommand(t, m, strings.TrimSpace("layout "+arg)).statusBar.searchInfo; got != want {
			t.Errorf(":layout %s said %q, want %q", arg, got, want)
		}
	}
	next, cmd := m.Update(tourLayoutMsg{arg: "reset"})
	failed := driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if want := "layout reset failed: exit status 4: no such scene"; failed.statusBar.searchInfo != want {
		t.Errorf("a failed reset said %q, want %q", failed.statusBar.searchInfo, want)
	}
	if failed.tour.statusSeq != m.tour.statusSeq+1 {
		t.Error("a failed reset did not ask the view status again")
	}
	m.engine.GetConfig().WalkthroughLayoutReset = ""
	if got := typeCommand(t, m, "layout reset").statusBar.searchInfo; got != "no walkthrough_layout_reset command configured" {
		t.Errorf("without a reset command :layout reset said %q", got)
	}
}

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

func TestLabelsStayInReachOfALongNote(t *testing.T) {
	m := longNoteApp(t)
	screen := screenText(m)
	if !strings.Contains(screen, "Note line 1.") || strings.Contains(screen, "Note line 60.") {
		t.Fatalf("the note should start at its top and not fit:\n%s", screen)
	}
	if title := rowWith(screen, "1.2 · Where it lands"); !strings.Contains(title, "↓ ") {
		t.Errorf("the title does not say how much is below: %q", title)
	}
	vx, vy := onScreen(t, m, "[2] url Spec")
	rx, ry := onScreen(t, m, "[1] a.go:5")
	// The labels are the pane's last rows: the box's bottom border is next.
	lines := strings.Split(screen, "\n")
	if ry != vy-1 || !strings.Contains(lines[vy+1], "└") {
		t.Errorf("labels at rows %d and %d; the row under them is %q, want the pane's bottom border", ry, vy, lines[vy+1])
	}

	// The wheel over the note scrolls the note, not the diff, and the labels
	// stay where they are, still clickable.
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
	if x, y := onScreen(t, m, "[2] url Spec"); x != vx || y != vy {
		t.Errorf("the labels moved with the note: (%d,%d) → (%d,%d)", vx, vy, x, y)
	}
	if title := rowWith(screen, "1.2 · Where it lands"); !strings.Contains(title, "↑ ") || strings.Contains(title, "↓") {
		t.Errorf("at the end the title should say only what is above: %q", title)
	}
	for label, want := range map[[2]int]tea.Msg{{vx, vy}: tourViewMsg{arg: "2"}, {rx, ry}: tourRelatedMsg{arg: "1"}} {
		if _, cmd := m.Update(leftClick(label[0], label[1])); cmd == nil || !reflect.DeepEqual(cmd(), want) {
			t.Errorf("after scrolling, a click at %v did not send %#v", label, want)
		}
	}
	// The last note line sits just above the labels' blank row, not under them.
	if _, ly := onScreen(t, m, "Note line 60."); ly != ry-2 {
		t.Errorf("the note's last line is on row %d, want %d (above the blank row over the labels)", ly, ry-2)
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

// A long note's pane stops at 40% of the window, box included; a short one
// stays the size of its note.
func TestTheNotePaneIsCappedAtFortyPercent(t *testing.T) {
	for _, height := range []int{30, 44, 60} {
		m := longNoteApp(t)
		m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: height})
		if outer := m.docPane.height + 2; outer > height*40/100 || outer < height*40/100-1 {
			t.Errorf("height %d: the long note's pane is %d rows, want 40%% (%d)", height, outer, height*40/100)
		}
	}
	m := viewsAppIn(t, &types.Config{}, 140, false) // "The write.": a short note
	if want := m.docPane.noteHeight(m.docPane.width); m.docPane.height != want {
		t.Errorf("a short note's pane is %d rows, want its own %d", m.docPane.height, want)
	}
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

// A tour note's title has a rule under it, and a click on a label still lands on
// the label (the rule moves the text down a row).
func TestANoteHasARuleUnderItsTitle(t *testing.T) {
	m := longNoteApp(t)
	lines := strings.Split(screenText(m), "\n")
	for i, l := range lines {
		if strings.Contains(l, "1.2 · Where it lands") {
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "────") {
				t.Fatalf("no rule under the note's title; next row %q", lines[i+1])
			}
			return
		}
	}
	t.Fatal("note title not on screen")
}
