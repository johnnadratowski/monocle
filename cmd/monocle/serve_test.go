package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
)

// TestCLIParsesWithoutIdleTimeoutFlag guards against a regression where
// ServeCmd.IdleTimeout had a default:"" tag that Kong rejected as an
// invalid duration during CLI setup — breaking every subcommand, not
// just `monocle serve`, because Kong validates all defaults upfront.
func TestCLIParsesWithoutIdleTimeoutFlag(t *testing.T) {
	// Building the parser exercises default-tag validation on every
	// field. If ServeCmd's --idle-timeout regressed to an invalid default
	// this call would fail.
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong setup failed (likely a bad default on some flag): %v", err)
	}
	if _, err := parser.Parse([]string{"hooks", "on-stop", "--agent", "claude"}); err != nil {
		t.Fatalf("parse hooks on-stop: %v", err)
	}
}

func TestPidFilePath(t *testing.T) {
	cases := []struct {
		socket string
		want   string
	}{
		{"/tmp/monocle-abc123.sock", "/tmp/monocle-abc123.pid"},
		{"/tmp/custom", "/tmp/custom.pid"},
	}
	for _, tc := range cases {
		got := pidFilePath(tc.socket)
		if got != tc.want {
			t.Errorf("pidFilePath(%q) = %q, want %q", tc.socket, got, tc.want)
		}
	}
}

func TestWriteReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := writePIDFile(path); err != nil {
		t.Fatalf("write: %v", err)
	}

	pid, err := readPIDFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}

	removePIDFile(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("pid file still exists after remove: %v", err)
	}
}

// goto-line's --top is optional: absent is nil, so "line at the top row"
// (--top 0) and "no placement" stay apart.
func TestGotoLineTopFlag(t *testing.T) {
	for _, c := range []struct {
		args []string
		want *int
	}{
		{[]string{"review", "goto-line", "a.go", "12"}, nil},
		{[]string{"review", "goto-line", "a.go", "12", "--top", "3"}, intPtr(3)},
		{[]string{"review", "goto-line", "a.go", "12", "--top", "0"}, intPtr(0)},
	} {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(c.args); err != nil {
			t.Fatalf("parse %v: %v", c.args, err)
		}
		got := cli.Review.GotoLine.Top
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%v: top %v, want %v", c.args, got, c.want)
		}
	}
}

func intPtr(n int) *int { return &n }

// highlight takes a path and both ends, or --clear alone.
func TestHighlightArgs(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"review", "highlight", "a.go", "3", "9"}); err != nil {
		t.Fatal(err)
	}
	if h := cli.Review.Highlight; h.Path != "a.go" || h.Start != 3 || h.End != 9 || h.Clear {
		t.Errorf("parsed %+v", h)
	}
	cli = CLI{}
	parser, _ = kong.New(&cli)
	if _, err := parser.Parse([]string{"review", "highlight", "--clear"}); err != nil {
		t.Fatal(err)
	}
	if !cli.Review.Highlight.Clear {
		t.Error("--clear did not parse")
	}
}

// open-editor takes a path and a line, and --full.
func TestOpenEditorArgs(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"review", "open-editor", "lib/util.go", "12", "--full"}); err != nil {
		t.Fatal(err)
	}
	if o := cli.Review.OpenEditor; o.Path != "lib/util.go" || o.Line != 12 || !o.Full {
		t.Errorf("parsed %+v", o)
	}
}
