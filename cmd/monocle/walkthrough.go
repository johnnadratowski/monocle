package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/tui"
	"github.com/josephschmitt/monocle/internal/types"
)

// ReviewSetWalkthroughCmd sends a guided tour from a JSON file. A tour is a list
// of stops each carrying views and calls — too nested for flags, and
// long enough that an agent writes it to a file anyway.
type ReviewSetWalkthroughCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	File   string `help:"Tour JSON: {\"title\":\"...\",\"stops\":[{\"id\":\"1.1\",\"title\":\"...\",\"file\":\"a.go\",\"line_start\":10,\"line_end\":20,\"note\":\"...\",\"related\":[{\"doc\":\"b.go\",\"start_line\":5}],\"views\":[{\"kind\":\"video\",\"target\":\"demo.webm\"}],\"calls\":[{\"stop\":\"1.2\",\"symbol\":\"save\",\"line\":14}],\"layout\":\"review\"}]}. A bare array of stops also works. Reads stdin when omitted or -." short:"f" default:""`
	Clear  bool   `help:"Withdraw the tour (equivalent to sending no stops)" default:"false"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewSetWalkthroughCmd) Run() error {
	var tour types.Walkthrough
	if !cmd.Clear {
		data, err := readTourSource(cmd.File)
		if err != nil {
			return fmt.Errorf("set-walkthrough: %w", err)
		}
		if tour, err = parseWalkthroughJSON(data); err != nil {
			return fmt.Errorf("set-walkthrough: %w", err)
		}
	}

	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.SetWalkthroughMsg{Type: protocol.TypeSetWalkthrough, Walkthrough: tour}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("set-walkthrough: %w", err)
	}
	r, ok := resp.(*protocol.SetWalkthroughResponse)
	if !ok {
		return fmt.Errorf("set-walkthrough: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// ReviewGotoStopCmd moves the reviewer to a stop by its id.
type ReviewGotoStopCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	ID     string `arg:"" help:"The stop's id, e.g. 1.2"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewGotoStopCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.GotoStopMsg{Type: protocol.TypeGotoStop, ID: cmd.ID}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("goto-stop: %w", err)
	}
	r, ok := resp.(*protocol.GotoStopResponse)
	if !ok {
		return fmt.Errorf("goto-stop: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// ReviewGotoLineCmd shows the reviewer a file of the review at a line.
type ReviewGotoLineCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	Path   string `arg:"" help:"A changed file or an added one: repo-relative, an added file's name, or absolute"`
	Line   int    `arg:"" help:"The new-file line, from 1"`
	Top    *int   `help:"Put the line this many rows below the top of the diff pane (default: centred)"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewGotoLineCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.GotoLineMsg{Type: protocol.TypeGotoLine, Path: cmd.Path, Line: cmd.Line, Top: cmd.Top}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("goto-line: %w", err)
	}
	r, ok := resp.(*protocol.GotoLineResponse)
	if !ok {
		return fmt.Errorf("goto-line: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// ReviewHighlightCmd sets the review's one highlighted range of lines, or
// clears it.
type ReviewHighlightCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	Path   string `arg:"" optional:"" help:"A changed file or an added one: repo-relative, an added file's name, or absolute"`
	Start  int    `arg:"" optional:"" help:"The range's first new-file line, from 1"`
	End    int    `arg:"" optional:"" help:"The range's last new-file line"`
	Clear  bool   `help:"Remove the highlighted range" default:"false"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewHighlightCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	msg := &protocol.HighlightRangeMsg{Type: protocol.TypeHighlightRange, Path: cmd.Path, Start: cmd.Start, End: cmd.End, Clear: cmd.Clear}
	resp, err := c.Request(msg, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("highlight: %w", err)
	}
	r, ok := resp.(*protocol.HighlightRangeResponse)
	if !ok {
		return fmt.Errorf("highlight: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// ReviewOpenEditorCmd opens any file of the repo at a line in the editor beside
// the reviewer's Monocle.
type ReviewOpenEditorCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	Path   string `arg:"" help:"Any file of the repo: repo-relative or absolute"`
	Line   int    `arg:"" help:"The line, from 1"`
	Full   bool   `help:"Fill the window with the editor (zoom its pane)" default:"false"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewOpenEditorCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.OpenEditorMsg{Type: protocol.TypeOpenEditor, Path: cmd.Path, Line: cmd.Line, Full: cmd.Full}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("open-editor: %w", err)
	}
	r, ok := resp.(*protocol.OpenEditorResponse)
	if !ok {
		return fmt.Errorf("open-editor: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// connectReview dials the engine for a repo, exiting with the client's own
// message when none is running — the same contract as every review command.
func connectReview(socket, workdir string) (*client.Client, error) {
	socketPath, err := resolveSocketForWorkDir(socket, workdir)
	if err != nil {
		return nil, err
	}
	c, err := client.Connect(socketPath)
	if err != nil {
		if errors.Is(err, client.ErrNotRunning) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return nil, err
	}
	return c, nil
}

func readTourSource(path string) ([]byte, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// parseWalkthroughJSON reads a tour file. Unknown fields are refused rather than
// ignored: a stop written with "line" instead of "line_start" would otherwise
// land at the top of the file with no hint why.
func parseWalkthroughJSON(data []byte) (types.Walkthrough, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return types.Walkthrough{}, errors.New("no tour given (pass --file, pipe JSON on stdin, or use --clear)")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var tour types.Walkthrough
	if trimmed[0] == '[' {
		if err := dec.Decode(&tour.Stops); err != nil {
			return types.Walkthrough{}, fmt.Errorf("parse stops: %w", err)
		}
	} else if err := dec.Decode(&tour); err != nil {
		return types.Walkthrough{}, fmt.Errorf("parse tour: %w", err)
	}
	// A file with no stops is far more likely a mistake than a withdrawal, which
	// has its own flag.
	if len(tour.Stops) == 0 {
		return types.Walkthrough{}, errors.New(`tour has no stops (use --clear to withdraw a tour)`)
	}
	return tour, nil
}

// ReviewCapsCmd asks the engine which TUIs are attached and what each
// understands. It answers for the running processes, not the binary on disk:
// a tour started before an upgrade still runs the old TUI.
type ReviewCapsCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewCapsCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	// An engine older than get_frontends closes the connection unanswered:
	// an error here, so a driver sends nothing.
	resp, err := c.Request(&protocol.GetFrontendsMsg{Type: protocol.TypeGetFrontends}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("caps: %w", err)
	}
	r, ok := resp.(*protocol.GetFrontendsResponse)
	if !ok {
		return fmt.Errorf("caps: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	fmt.Print(capsText(r))
	return nil
}

// capsText is caps without --json: a line per TUI, then what all of them have.
func capsText(r *protocol.GetFrontendsResponse) string {
	if len(r.Frontends) == 0 {
		return "no Monocle TUI attached\n"
	}
	var b strings.Builder
	for _, f := range r.Frontends {
		fmt.Fprintf(&b, "tui pid %d (%s): %s\n", f.Pid, f.Version, strings.Join(f.Features, " "))
	}
	fmt.Fprintf(&b, "all: %s\n", strings.Join(r.Features, " "))
	return b.String()
}

// frontendInfo is how this process reports itself as a TUI to the engine.
func frontendInfo() protocol.FrontendInfo {
	return protocol.FrontendInfo{Pid: os.Getpid(), Version: version, Features: tui.Features}
}

// ReviewDocCmd hides or shows the doc pane in the reviewer's TUI, through the
// engine: nothing is typed into the TUI, and one too old to know it ignores it.
type ReviewDocCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	State  string `arg:"" enum:"on,off,toggle" help:"on, off or toggle"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewDocCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.DocPaneMsg{Type: protocol.TypeDocPane, State: cmd.State}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("doc: %w", err)
	}
	r, ok := resp.(*protocol.DocPaneResponse)
	if !ok {
		return fmt.Errorf("doc: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}

// ReviewCommentsCmd shows or hides source-code comment lines in the reviewer's
// TUI, through the engine, as its `:comments` does: a walkthrough's stage pushes
// its tour-wide comment state with it when the regions explorer's key changes it.
type ReviewCommentsCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	State  string `arg:"" enum:"on,off,toggle" help:"on (comments shown), off (hidden) or toggle"`
	JSON   bool   `help:"Output as JSON" default:"false"`
}

func (cmd *ReviewCommentsCmd) Run() error {
	c, err := connectReview(cmd.Socket, cmd.WorkDir)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Request(&protocol.CommentsMsg{Type: protocol.TypeComments, State: cmd.State}, client.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("comments: %w", err)
	}
	r, ok := resp.(*protocol.CommentsResponse)
	if !ok {
		return fmt.Errorf("comments: unexpected response %T", resp)
	}
	if cmd.JSON {
		return printJSON(r)
	}
	if !r.Success {
		return fmt.Errorf("%s", r.Message)
	}
	fmt.Println(r.Message)
	return nil
}
