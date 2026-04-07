package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// engineConn maintains a persistent socket connection to the Monocle engine
// for receiving push event notifications. It forwards events as MCP channel
// notifications through the captured stdio connection.
type engineConn struct {
	conn      mcp.Connection // MCP stdio connection for writing notifications
	agentName string
}

func newEngineConn(conn mcp.Connection) *engineConn {
	return &engineConn{conn: conn}
}

// run connects to the engine and listens for events, forwarding them as
// channel notifications. It reconnects with backoff on connection loss.
func (e *engineConn) run(ctx context.Context) {
	socketPath := os.Getenv("MONOCLE_SOCKET")
	if socketPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return
		}
		repoRoot := adapters.FindRepoRoot(cwd)
		socketPath = adapters.DefaultSocketPath(repoRoot)
	}

	delay := 2 * time.Second
	for {
		if err := e.connectAndListen(ctx, socketPath); err != nil {
			// Context cancelled — shutting down
			if ctx.Err() != nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			// Exponential backoff, cap at 10s
			delay = min(delay*2, 10*time.Second)
		}
	}
}

// connectAndListen opens a socket connection, sends a connect message to
// subscribe to events, and forwards incoming events as channel notifications.
func (e *engineConn) connectAndListen(ctx context.Context, socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Send connect message to subscribe to events
	connectMsg := protocol.ConnectMsg{
		Type: protocol.TypeConnect,
		Events: []string{
			"feedback_submitted",
			"pause_changed",
			"content_item_added",
			"additional_file_added",
		},
	}
	data, err := protocol.Encode(&connectMsg)
	if err != nil {
		return fmt.Errorf("encode connect: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write connect: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read connect response
	if !scanner.Scan() {
		return fmt.Errorf("no connect response")
	}
	// Verify it's a connect_response (don't need to parse fully)

	// Send identify if we have an agent name
	if e.agentName != "" {
		e.sendIdentify(conn)
	}

	// Event loop
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msg, err := protocol.Decode(line)
		if err != nil {
			continue
		}

		if notif, ok := msg.(*protocol.EventNotification); ok {
			e.handleEvent(ctx, notif)
		}
	}

	return scanner.Err()
}

// handleEvent converts an engine event to an MCP channel notification.
func (e *engineConn) handleEvent(ctx context.Context, notif *protocol.EventNotification) {
	var content string
	var meta map[string]any

	switch notif.Event {
	case "feedback_submitted":
		msg, _ := notif.Payload["message"].(string)
		if msg == "" {
			msg = "Your reviewer has submitted feedback. Use the get_feedback tool to retrieve it."
		}
		content = msg
		meta = map[string]any{"event": "feedback_submitted"}

	case "pause_changed":
		status, _ := notif.Payload["status"].(string)
		if status != "pause_requested" {
			return
		}
		content = "Your reviewer has requested you pause and wait for feedback. " +
			"Use the get_feedback tool with wait=true to block until feedback is ready."
		meta = map[string]any{"event": "pause_requested"}

	default:
		return
	}

	e.sendChannelNotification(ctx, content, meta)
}

// sendChannelNotification sends a channel notification through the MCP connection.
func (e *engineConn) sendChannelNotification(ctx context.Context, content string, meta map[string]any) {
	params, err := json.Marshal(map[string]any{
		"content": content,
		"meta":    meta,
	})
	if err != nil {
		return
	}

	notif := &jsonrpc.Request{
		Method: "notifications/claude/channel",
		Params: params,
	}

	_ = e.conn.Write(ctx, notif)
}

// sendIdentify sends an identify message to the engine.
func (e *engineConn) sendIdentify(conn net.Conn) {
	msg := protocol.IdentifyMsg{
		Type:  protocol.TypeIdentify,
		Agent: e.agentName,
	}
	data, _ := protocol.Encode(&msg)
	conn.Write(data)
}

// identify sets the agent name and is called after the MCP handshake completes.
func (e *engineConn) identify(name string) {
	e.agentName = name
}

// close is a no-op — the engine socket connection is closed by connectAndListen's defer.
func (e *engineConn) close() {}
