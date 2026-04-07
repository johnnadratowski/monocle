// Package mcp implements the Monocle MCP server, exposing review operations
// as MCP tools and optionally forwarding engine events as channel notifications.
package mcp

import (
	"context"
	"os/signal"
	"syscall"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures the MCP server.
type Options struct {
	// EnableChannels adds the experimental claude/channel capability and
	// subscribes to engine events for push notifications.
	EnableChannels bool
}

// Run creates and runs the MCP server over stdio, blocking until the client
// disconnects or the process receives SIGINT/SIGTERM.
func Run(opts Options) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	serverOpts := &sdkmcp.ServerOptions{
		Instructions: toolInstructions,
	}

	if opts.EnableChannels {
		serverOpts.Capabilities = &sdkmcp.ServerCapabilities{
			Experimental: map[string]any{
				"claude/channel": map[string]any{},
			},
		}
		serverOpts.Instructions = toolInstructions + "\n" + channelInstructions
	}

	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "monocle",
		Version: version(),
	}, serverOpts)

	registerTools(server)

	transport := &sdkmcp.StdioTransport{}

	if opts.EnableChannels {
		// Use Connect instead of Run so we can capture the connection
		// for sending custom channel notifications.
		ct := &capturingTransport{inner: transport}
		session, err := server.Connect(ctx, ct, nil)
		if err != nil {
			return err
		}

		engine := newEngineConn(ct.conn)
		defer engine.close()

		// Identify agent after handshake
		if p := session.InitializeParams(); p != nil && p.ClientInfo.Name != "" {
			engine.identify(p.ClientInfo.Name)
		}

		go engine.run(ctx)

		return session.Wait()
	}

	return server.Run(ctx, transport)
}

// capturingTransport wraps a Transport to capture the Connection for sending
// custom notifications (used for MCP channel support).
type capturingTransport struct {
	inner sdkmcp.Transport
	conn  sdkmcp.Connection
}

func (t *capturingTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.conn = conn
	return conn, nil
}

// version returns the binary version, falling back to "dev".
func version() string {
	if Version != "" {
		return Version
	}
	return "dev"
}

// Version is set by the main package before calling Run.
var Version string

const toolInstructions = `Use the review_status tool to check if feedback is pending.
Use the get_feedback tool to retrieve review feedback.
Use the send_artifact tool to send content for review.
Use the add_files tool to add files to the review.`

const channelInstructions = `When you receive a feedback_submitted event, use the get_feedback tool to retrieve the review.
When you receive a pause_requested event, use the get_feedback tool with wait=true to block until the reviewer submits feedback.`
