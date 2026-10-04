package tui

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

var f17Key = tea.KeyPressMsg{Code: tea.KeyF17}

// orderRelated stands in for the tmux side of the related pane, recording each
// plan, and writing "respawn" to order when the respawn actually runs.
func orderRelated(t *testing.T, order string) *[]relatedPanePlan {
	t.Helper()
	t.Setenv("TMUX", filepath.Join(t.TempDir(), "no-server")+",1,0")
	t.Setenv("TMUX_PANE", "%99")
	var plans []relatedPanePlan
	orig := showRelated
	showRelated = func(_ string, p relatedPanePlan) tea.Cmd {
		plans = append(plans, p)
		return func() tea.Msg {
			f, _ := os.OpenFile(order, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			_, _ = io.WriteString(f, "respawn\n")
			_ = f.Close()
			return relatedPaneMsg{pane: "%7", files: p.files}
		}
	}
	t.Cleanup(func() { showRelated = orig })
	return &plans
}

// Resetting the layout brings the stop's related files back, fresh — not what
// the pane held, which had files added and closed — and only then runs
// walkthrough_layout_reset: by key (f17), by :layout reset, by the label.
func TestLayoutResetRespawnsTheStopsRelatedFilesFirst(t *testing.T) {
	for _, how := range []struct {
		name string
		do   func(appModel) (tea.Model, tea.Cmd)
	}{
		{"f17", func(m appModel) (tea.Model, tea.Cmd) { return m.Update(f17Key) }},
		{":layout reset or the label", func(m appModel) (tea.Model, tea.Cmd) { return m.Update(tourLayoutMsg{arg: "reset"}) }},
	} {
		t.Run(how.name, func(t *testing.T) {
			m, dir := layoutApp(t, `echo reset >> "$(dirname STATE)/order"`)
			order := filepath.Join(dir, "order")
			plans := orderRelated(t, order)
			m.tour.paneFiles = []relatedFile{{"a.go", 5}, {"added.sql", 0}}
			next, cmd := how.do(m)
			m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)

			data, _ := os.ReadFile(order)
			if got := strings.Fields(string(data)); !reflect.DeepEqual(got, []string{"respawn", "reset"}) {
				t.Errorf("ran %q, want the respawn, then the reset command", got)
			}
			if len(*plans) != 1 {
				t.Fatalf("%d plans, want one respawn", len(*plans))
			}
			p := (*plans)[0]
			want := []relatedFile{{"a.go", 5}, {"internal/deep/path/b.go", 30}}
			if !reflect.DeepEqual(p.files, want) || p.selectPane {
				t.Errorf("respawned with %+v (focus %v), want the stop's own %+v, focus left alone", p.files, p.selectPane, want)
			}
			if !reflect.DeepEqual(m.tour.paneFiles, want) || m.statusBar.searchInfo != "layout reset" {
				t.Errorf("after the reset the pane holds %+v, status %q", m.tour.paneFiles, m.statusBar.searchInfo)
			}
		})
	}

	// With no reset command the related files still come back.
	t.Run("no reset command", func(t *testing.T) {
		m, dir := layoutApp(t, "")
		order := filepath.Join(dir, "order")
		orderRelated(t, order)
		next, cmd := m.Update(f17Key)
		m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
		data, _ := os.ReadFile(order)
		if strings.TrimSpace(string(data)) != "respawn" || !strings.Contains(m.statusBar.searchInfo, "no walkthrough_layout_reset") {
			t.Errorf("ran %q, status %q: want the respawn and the note", data, m.statusBar.searchInfo)
		}
	})
}

// f17 never fires while typing.
func TestLayoutResetKeyNeverFiresWhileTyping(t *testing.T) {
	for _, opens := range []string{":", "/", "!", "c"} {
		m, dir := layoutApp(t, `echo reset >> "$(dirname STATE)/order"`)
		plans := orderRelated(t, filepath.Join(dir, "order"))
		m = pressKey(t, m, opens)
		next, cmd := m.Update(f17Key)
		_ = driveWithin(t, next.(appModel), cmd, 0, 2*time.Second)
		if _, err := os.Stat(filepath.Join(dir, "order")); err == nil || len(*plans) != 0 {
			t.Errorf("f17 after %q reset the layout", opens)
		}
	}
}

// A remapper sends f17 as ESC[31~; through Bubble Tea's own reader it must be
// the layout-reset key.
func TestF17BytesAreTheLayoutResetKey(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	rec := &keyRecorder{want: 1}
	p := tea.NewProgram(rec, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithoutSignals(), tea.WithoutRenderer())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	go func() { _, _ = w.Write([]byte("\x1b[31~")) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatalf("the program never saw the key; it saw %v", rec.keys)
	}
	if len(rec.keys) != 1 || !Matches(rec.keys[0], DefaultKeyMap().LayoutReset) {
		t.Errorf("ESC[31~ arrived as %q, want the layout-reset key", rec.keys)
	}
}
