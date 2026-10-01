package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
