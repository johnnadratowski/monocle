package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestRelatedEditorArgv(t *testing.T) {
	files := []relatedFile{{path: "src/a.ts", line: 40}, {path: "db/b.sql", line: 12}}

	t.Run("nvim opens every file as a split, each at its line", func(t *testing.T) {
		got := relatedEditorArgv("nvim", files)
		want := []string{"nvim", "-o", "src/a.ts", "db/b.sql",
			"-c", "1wincmd w|exe 'normal! 40Gzz'|2wincmd w|exe 'normal! 12Gzz'|1wincmd w"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("vim keeps its configured flags", func(t *testing.T) {
		got := relatedEditorArgv("vim -u NONE", files[:1])
		want := []string{"vim", "-u", "NONE", "-o", "src/a.ts", "-c", "1wincmd w|exe 'normal! 40Gzz'|1wincmd w"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	// vim runs at most ten -c commands. One -c per step would break at five
	// files, so however many there are it stays one.
	t.Run("any number of files is one -c", func(t *testing.T) {
		var many []relatedFile
		for i := 1; i <= 7; i++ {
			many = append(many, relatedFile{path: "f.go", line: i})
		}
		got := relatedEditorArgv("/usr/local/bin/nvim", many)
		n := 0
		for _, a := range got {
			if a == "-c" {
				n++
			}
		}
		if n != 1 || !strings.Contains(got[len(got)-1], "7wincmd w|exe 'normal! 7Gzz'|1wincmd w") {
			t.Errorf("got %d -c in %q, want one covering all seven windows", n, got)
		}
	})

	t.Run("a file with no line is opened, not positioned", func(t *testing.T) {
		got := relatedEditorArgv("nvim", []relatedFile{{path: "a"}, {path: "b", line: 3}})
		want := []string{"nvim", "-o", "a", "b", "-c", "2wincmd w|exe 'normal! 3Gzz'|1wincmd w"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
		if got := relatedEditorArgv("nvim", []relatedFile{{path: "a"}}); !reflect.DeepEqual(got, []string{"nvim", "-o", "a"}) {
			t.Errorf("no lines at all should mean no -c, got %q", got)
		}
	})

	t.Run("another editor opens only the first file", func(t *testing.T) {
		got := relatedEditorArgv("hx", files)
		if want := []string{"hx", "+40", "src/a.ts"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
		got = relatedEditorArgv("code --wait", []relatedFile{{path: "x.go"}})
		if want := []string{"code", "--wait", "x.go"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("nothing to open is nothing to run", func(t *testing.T) {
		if got := relatedEditorArgv("nvim", nil); got != nil {
			t.Errorf("got %q", got)
		}
	})
}

func TestRelatedPaneArgs(t *testing.T) {
	argv := []string{"nvim", "-o", "a b.go", "-c", "1wincmd w|exe 'normal! 4Gzz'|1wincmd w"}
	cmd := shellJoin(argv)

	t.Run("a live pane is reused in place", func(t *testing.T) {
		got := relatedPaneArgs(relatedPanePlan{existing: "%9", owner: "%1", dir: "/repo", argv: argv, focus: true})
		want := []string{"respawn-pane", "-k", "-t", "%9", "-c", "/repo", cmd}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("with none, a split opens beside Monocle without taking focus", func(t *testing.T) {
		got := relatedPaneArgs(relatedPanePlan{owner: "%1", dir: "/repo", argv: argv})
		want := []string{"split-window", "-h", "-t", "%1", "-d", "-c", "/repo", "-P", "-F", "#{pane_id}", cmd}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("focus and the stacked mode are honoured on a split", func(t *testing.T) {
		got := relatedPaneArgs(relatedPanePlan{owner: "%1", dir: "/r", mode: "tmux_horizontal", focus: true, argv: argv})
		want := []string{"split-window", "-v", "-t", "%1", "-c", "/r", "-P", "-F", "#{pane_id}", cmd}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	// respawn-pane -k on Monocle's own pane would kill Monocle.
	t.Run("Monocle's own pane is never respawned", func(t *testing.T) {
		got := relatedPaneArgs(relatedPanePlan{existing: "%1", owner: "%1", dir: "/r", argv: argv})
		if got[0] != "split-window" {
			t.Errorf("got %q, want a split rather than respawning the owner", got)
		}
	})

	t.Run("the editor invocation survives the shell", func(t *testing.T) {
		got := relatedPaneArgs(relatedPanePlan{owner: "%1", dir: "/r", argv: argv})
		last := got[len(got)-1]
		// The command reaches tmux as one sh string: the space in the path and
		// the quotes inside the vim command must both be quoted for sh.
		if !strings.Contains(last, "'a b.go'") || !strings.Contains(last, `'1wincmd w|exe '\''normal! 4Gzz'\''|1wincmd w'`) {
			t.Errorf("shell command %q is not quoted for sh", last)
		}
	})
}

func TestOwnedRelatedPane(t *testing.T) {
	listing := "%1 \n%4 %1\n%7 %2\n%9 \n"
	if got := ownedRelatedPane(listing, "%1"); got != "%4" {
		t.Errorf("owner %%1 found %q, want %%4", got)
	}
	if got := ownedRelatedPane(listing, "%2"); got != "%7" {
		t.Errorf("owner %%2 found %q, want %%7 (each Monocle has its own)", got)
	}
	if got := ownedRelatedPane(listing, "%3"); got != "" {
		t.Errorf("owner %%3 found %q, want none", got)
	}
	if got := ownedRelatedPane("%1 %1\n", "%1"); got != "" {
		t.Errorf("a pane tagged with itself is the owner, never its related pane; got %q", got)
	}
	if got := ownedRelatedPane(listing, ""); got != "" {
		t.Errorf("no owner (not in tmux) found %q", got)
	}
}

func TestRelatedFocusDefaultsToMonocle(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		cfg  *bool
		want bool
	}{{nil, false}, {&yes, true}, {&no, false}} {
		m := appModel{engine: &stubEngine{cfg: &types.Config{EditorFocus: c.cfg}}}
		if got := m.relatedFocus(); got != c.want {
			t.Errorf("editor_focus %v: relatedFocus = %v, want %v", c.cfg, got, c.want)
		}
	}
}

// Outside tmux there is nowhere to open the files; the stop says so rather
// than taking over the terminal on every step.
func TestRelatedFilesOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	m := appModel{engine: &stubEngine{cfg: &types.Config{}}}
	stop := types.WalkthroughStop{ID: "1", Related: []types.DocRef{{Doc: "a.go", StartLine: 2}}}
	cmd := m.openRelated(stop)
	if cmd == nil {
		t.Fatal("a stop with related files should report why it cannot open them")
	}
	msg, ok := cmd().(relatedPaneMsg)
	if !ok || msg.err == nil || !strings.Contains(msg.err.Error(), "tmux") {
		t.Errorf("got %+v, want an error naming tmux", msg)
	}
	if m.openRelated(types.WalkthroughStop{ID: "2"}) != nil {
		t.Error("a stop with no related files leaves the pane alone")
	}
}

// The side effects run once the reviewer rests on a stop, not for every stop
// they pass on the way.
func TestSettleRunsOnlyForTheLatestStop(t *testing.T) {
	m, _ := tourApp(t)
	m = pressKey(t, m, ".")
	stale := m.tour.settle
	m = pressKey(t, m, ",")
	if m.tour.settle == stale {
		t.Fatal("each entry should take a new settle number")
	}
	if _, cmd := m.settleOnStop(tourSettledMsg{seq: stale}); cmd != nil {
		t.Error("a settle for a stop already left should do nothing")
	}
	// Outside tmux the latest stop (1.1, no related files) has nothing to open,
	// so only 1.2 would produce a command; step back there to check it does.
	m = pressKey(t, m, ".")
	if _, cmd := m.settleOnStop(tourSettledMsg{seq: m.tour.settle}); cmd == nil {
		t.Error("the current stop's settle should run its side effects")
	}
}
