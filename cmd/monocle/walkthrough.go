package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// ReviewSetWalkthroughCmd sends a guided tour from a JSON file. A tour is a list
// of stops each carrying related files and views — too nested for flags, and
// long enough that an agent writes it to a file anyway.
type ReviewSetWalkthroughCmd struct {
	WorkDirFlag
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
	File   string `help:"Tour JSON: {\"title\":\"...\",\"stops\":[{\"id\":\"1.1\",\"title\":\"...\",\"file\":\"a.go\",\"line_start\":10,\"line_end\":20,\"note\":\"...\",\"related\":[{\"doc\":\"b.go\",\"start_line\":5}],\"views\":[{\"kind\":\"video\",\"target\":\"demo.webm\"}],\"layout\":\"review\"}]}. A bare array of stops also works. Reads stdin when omitted or -." short:"f" default:""`
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
