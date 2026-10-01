package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestOnStopEnv(t *testing.T) {
	stop := types.WalkthroughStop{
		ID: "1.2", Title: "Where it lands", File: "db/q.go", LineStart: 4, Layout: "review",
		Views: []types.StopView{
			{Kind: types.StopViewImage, Target: "shots/login.png", Label: "Login"},
			{Kind: types.StopViewVideo, Target: "/abs/demo.webm"},
			{Kind: types.StopViewURL, Target: "https://example.test/x"},
			{Kind: types.StopViewMarkdown, Target: "docs/why.md"},
			{Kind: types.StopViewArtifact, Target: "plan-7"},
			{Kind: types.StopViewArtifact, Target: "gone"},
		},
	}
	artifacts := map[string]string{"plan-7": "/data/media/plan-7.png"}
	env, err := onStopEnv(stop, "/repo", func(id string) (string, bool) {
		p, ok := artifacts[id]
		return p, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 3 || env[0] != "MONOCLE_STOP_ID=1.2" || env[1] != "MONOCLE_REPO_ROOT=/repo" ||
		!strings.HasPrefix(env[2], "MONOCLE_STOP_JSON=") {
		t.Fatalf("env = %q", env)
	}

	var got types.WalkthroughStop
	if err := json.Unmarshal([]byte(strings.TrimPrefix(env[2], "MONOCLE_STOP_JSON=")), &got); err != nil {
		t.Fatalf("MONOCLE_STOP_JSON is not the stop as JSON: %v", err)
	}
	want := stop
	want.Views = []types.StopView{
		{Kind: types.StopViewImage, Target: "/repo/shots/login.png", Label: "Login"},
		{Kind: types.StopViewVideo, Target: "/abs/demo.webm"},
		{Kind: types.StopViewURL, Target: "https://example.test/x"},
		{Kind: types.StopViewMarkdown, Target: "/repo/docs/why.md"},
		{Kind: types.StopViewArtifact, Target: "/data/media/plan-7.png"},
		// An artifact that cannot be found keeps its id: the command can still
		// say which one was missing.
		{Kind: types.StopViewArtifact, Target: "gone"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stop JSON\n got  %+v\n want %+v", got, want)
	}
	if stop.Views[0].Target != "shots/login.png" {
		t.Error("resolving must not rewrite the tour's own copy of the stop")
	}
}

func TestExecOnStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the on-stop command runs under sh here")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")

	t.Run("gets the stop in its environment and runs in the repo", func(t *testing.T) {
		cmd := `printf '%s|%s|%s|%s\n' "$MONOCLE_STOP_ID" "$MONOCLE_REPO_ROOT" "$(pwd -P)" "$MONOCLE_STOP_JSON" >> ` + out
		env := []string{"MONOCLE_STOP_ID=1.2", "MONOCLE_REPO_ROOT=" + dir, `MONOCLE_STOP_JSON={"id":"1.2"}`}
		if err := execOnStop(cmd, dir, env, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(out)
		real, _ := filepath.EvalSymlinks(dir)
		if want := "1.2|" + dir + "|" + real + `|{"id":"1.2"}` + "\n"; string(data) != want {
			t.Errorf("command saw %q, want %q", data, want)
		}
	})

	t.Run("a failure says why, from stderr", func(t *testing.T) {
		err := execOnStop(`echo noise >&2; echo "stage: no such scene" >&2; exit 3`, dir, nil, 5*time.Second)
		if err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "stage: no such scene") {
			t.Errorf("err = %v, want the exit status and the last stderr line", err)
		}
	})

	// A hung command must not hang anything: it is killed at the timeout, and
	// so is whatever it started.
	t.Run("a hung command is killed at the timeout", func(t *testing.T) {
		start := time.Now()
		err := execOnStop(`sleep 30 & sleep 30`, dir, nil, 200*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("err = %v, want a timeout", err)
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("took %s to give up, want about the timeout", took)
		}
	})
}

func TestNoOnStopCommandRunsNothing(t *testing.T) {
	m := appModel{engine: &stubEngine{cfg: &types.Config{WalkthroughOnStop: "   "}}}
	if m.runOnStop(types.WalkthroughStop{ID: "1"}) != nil {
		t.Error("an empty walkthrough_on_stop should run nothing")
	}
}

func TestOnStopFailureShowsInTheStatusBar(t *testing.T) {
	m := appModel{}
	m, _ = m.handleOnStopDone(onStopDoneMsg{id: "1.2", err: os.ErrPermission})
	if !strings.Contains(m.statusBar.searchInfo, "on-stop 1.2 failed") {
		t.Errorf("status %q", m.statusBar.searchInfo)
	}
	m.statusBar.searchInfo = ""
	if m, _ = m.handleOnStopDone(onStopDoneMsg{id: "1.2"}); m.statusBar.searchInfo != "" {
		t.Errorf("success should be silent, got %q", m.statusBar.searchInfo)
	}
}

func TestViewCommandRefusesWhatItCannotOpen(t *testing.T) {
	m, _ := tourApp(t)
	m = typeCommand(t, m, "view")
	if m.statusBar.searchInfo != "1.1 has no views" {
		t.Errorf(":view on a stop with none said %q", m.statusBar.searchInfo)
	}
	m = pressKey(t, m, ".")
	for _, arg := range []string{"0", "2", "x"} {
		next, _ := m.openStopView(arg)
		if next.statusBar.searchInfo != "1.2 has views 1-1" {
			t.Errorf(":view %s said %q", arg, next.statusBar.searchInfo)
		}
	}
	m = pressKey(t, m, "W")
	if next, _ := m.openStopView(""); next.statusBar.searchInfo != "no tour stop to open a view from" {
		t.Errorf(":view with the tour off said %q", next.statusBar.searchInfo)
	}
}
