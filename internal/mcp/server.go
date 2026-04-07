// Package mcp implements the Monocle MCP server, exposing review operations
// as MCP tools and optionally forwarding engine events as channel notifications.
package mcp

import (
	"context"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/josephschmitt/monocle/internal/adapters"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures the MCP server.
type Options struct {
	// EnableChannels adds the experimental claude/channel capability and
	// subscribes to engine events for push notifications alongside tools.
	EnableChannels bool

	// ChannelsOnly enables channels without registering any tools.
	// Used when the agent integrates via skills (CLI commands) instead of MCP tools.
	// Channel notifications reference CLI commands instead of tools.
	ChannelsOnly bool
}

// Run creates and runs the MCP server over stdio, blocking until the client
// disconnects or the process receives SIGINT/SIGTERM.
func Run(opts Options) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	wantChannels := opts.EnableChannels || opts.ChannelsOnly
	wantTools := !opts.ChannelsOnly

	serverOpts := &sdkmcp.ServerOptions{}

	// Set instructions based on mode
	switch {
	case wantTools && wantChannels:
		serverOpts.Instructions = toolInstructions + "\n" + channelToolInstructions
	case wantTools:
		serverOpts.Instructions = toolInstructions
	case wantChannels:
		serverOpts.Instructions = channelCLIInstructions
	}

	if wantChannels {
		serverOpts.Capabilities = &sdkmcp.ServerCapabilities{
			Experimental: map[string]any{
				"claude/channel": map[string]any{},
			},
		}
	}

	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "monocle",
		Version: version(),
	}, serverOpts)

	if wantTools {
		registerTools(server)
	}

	transport := &sdkmcp.StdioTransport{}

	// Always use Connect (not Run) so we get a session for ListRoots.
	ct := &capturingTransport{inner: transport}
	session, err := server.Connect(ctx, ct, nil)
	if err != nil {
		return err
	}

	// Resolve the engine socket path from MCP roots.
	// If MONOCLE_SOCKET is already set, this is a no-op.
	resolveSocketFromRoots(ctx, session)

	if wantChannels {
		engine := newEngineConn(ct.conn, opts.ChannelsOnly)
		defer engine.close()

		if p := session.InitializeParams(); p != nil && p.ClientInfo.Name != "" {
			engine.identify(p.ClientInfo.Name)
		}

		go engine.run(ctx)
	}

	return session.Wait()
}

// resolveSocketFromRoots queries the client for its roots and sets MONOCLE_SOCKET
// if not already set. This allows the MCP server to find the engine socket when
// launched from a context without the correct working directory (e.g., desktop apps).
func resolveSocketFromRoots(ctx context.Context, session *sdkmcp.ServerSession) {
	if os.Getenv("MONOCLE_SOCKET") != "" {
		return
	}

	res, err := session.ListRoots(ctx, nil)
	if err != nil || len(res.Roots) == 0 {
		return
	}

	for _, root := range res.Roots {
		path, err := url.Parse(root.URI)
		if err != nil || path.Scheme != "file" {
			continue
		}
		dir := path.Path
		repoRoot := adapters.FindRepoRoot(dir)
		socketPath := adapters.DefaultSocketPath(repoRoot)
		if _, err := os.Stat(socketPath); err == nil {
			os.Setenv("MONOCLE_SOCKET", socketPath)
			return
		}
	}
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

// channelToolInstructions tells agents to use MCP tools when receiving channel events.
const channelToolInstructions = `When you receive a feedback_submitted event, use the get_feedback tool to retrieve the review.
When you receive a pause_requested event, use the get_feedback tool with wait=true to block until the reviewer submits feedback.`

// channelCLIInstructions tells agents to use CLI commands when receiving channel events.
const channelCLIInstructions = `When you receive a feedback_submitted event, run ` + "`monocle review get-feedback`" + ` to retrieve the review.
When you receive a pause_requested event, run ` + "`monocle review get-feedback --wait`" + ` to block until the reviewer submits feedback.`
