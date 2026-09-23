// Package client provides a socket client for communicating with a running
// Monocle engine via its Unix domain socket.
package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/protocol"
)

// ErrNotRunning is returned when the Monocle socket is not reachable.
var ErrNotRunning = errors.New("monocle is not running — start it with 'monocle'")

// Client communicates with a running Monocle engine over a Unix domain socket.
//
// The engine answers a non-subscribing connection one request at a time and
// then closes it. A caller that sends a second request on the same Client would
// therefore be writing into a socket the engine had already closed — a broken
// pipe, with the first request having quietly succeeded. That is not a contract
// callers should have to know, so the Client redials for them.
type Client struct {
	conn       net.Conn
	scanner    *bufio.Scanner
	socketPath string
	// spent records that this connection has already carried a request, so the
	// engine has closed its end even though nothing here has failed yet.
	spent bool
}

// Connect dials the Unix domain socket at the given path.
func Connect(socketPath string) (*Client, error) {
	if _, err := os.Stat(socketPath); errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotRunning
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, ErrNotRunning
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	return &Client{conn: conn, scanner: scanner, socketPath: socketPath}, nil
}

// redial replaces a spent connection with a fresh one. Errors are returned to
// the caller of Request, which is where a dial failure belongs.
func (c *Client) redial() error {
	if c.socketPath == "" {
		return errors.New("connection already used and no socket path to redial")
	}
	c.conn.Close()
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return ErrNotRunning
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	c.conn, c.scanner, c.spent = conn, scanner, false
	return nil
}

// ConnectDefault resolves the socket path from the current working directory
// and connects. Respects the MONOCLE_SOCKET environment variable.
func ConnectDefault() (*Client, error) {
	socketPath := adapters.ResolveSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("get cwd: unable to resolve socket path")
	}
	return Connect(socketPath)
}

// ConnectWithOverride connects using the override path if non-empty, otherwise
// falls back to ConnectDefault.
func ConnectWithOverride(socketOverride string) (*Client, error) {
	if socketOverride != "" {
		return Connect(socketOverride)
	}
	return ConnectDefault()
}

// Request sends a protocol message and reads the response. The caller provides
// a timeout; use 0 for no deadline (blocking operations).
func (c *Client) Request(msg any, timeout time.Duration) (any, error) {
	data, err := protocol.Encode(msg)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	// The engine closed its end after the previous exchange; write into a fresh
	// connection rather than a dead one.
	if c.spent {
		if err := c.redial(); err != nil {
			return nil, err
		}
	}
	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	c.spent = true

	if timeout > 0 {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
	} else {
		c.conn.SetReadDeadline(time.Time{}) // no deadline
	}

	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		return nil, errors.New("connection closed by server")
	}

	resp, err := protocol.Decode(c.scanner.Bytes())
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return resp, nil
}

// RequestWithContext sends msg and blocks for the response with no read
// deadline, but abandons the wait and closes the connection if ctx is cancelled
// first. Use it for blocking waits (get_feedback --wait) instead of Request with
// timeout 0.
//
// Closing the connection on cancellation is the load-bearing behaviour: it both
// unblocks this read AND signals the engine's disconnect watcher
// (watchConnClose) to release its blocking wait WITHOUT consuming feedback. A
// plain Request(msg, 0) ignores ctx, so an aborted tool call would leave the
// connection open, the engine would keep waiting, and a later submission would
// be consumed and marked delivered into a response nobody reads — silently
// losing the verdict. This makes an aborted wait leave the verdict queued for
// the next poll.
func (c *Client) RequestWithContext(ctx context.Context, msg any) (any, error) {
	data, err := protocol.Encode(msg)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	if c.spent {
		if err := c.redial(); err != nil {
			return nil, err
		}
	}
	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	c.spent = true
	c.conn.SetReadDeadline(time.Time{}) // no deadline; ctx governs cancellation

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = c.conn.Close()
		case <-done:
		}
	}()

	if !c.scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		return nil, errors.New("connection closed by server")
	}

	resp, err := protocol.Decode(c.scanner.Bytes())
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return resp, nil
}

// AckFeedback confirms receipt of a two-phase delivery so the engine commits it
// (advances the round, clears comments, marks the submission delivered).
//
// It dials a fresh connection because the engine closes a one-shot connection
// after each response. Best-effort by design: if the ack never lands, the
// delivery lease expires and the verdict returns to the queue for redelivery —
// a duplicate, never a loss — so callers have nothing useful to do with an
// error and it is intentionally swallowed.
func AckFeedback(socketPath, deliveryID string) {
	if deliveryID == "" {
		return
	}
	c, err := Connect(socketPath)
	if err != nil {
		return
	}
	defer c.Close()
	_, _ = c.Request(
		&protocol.AckFeedbackMsg{Type: protocol.TypeAckFeedback, DeliveryID: deliveryID},
		DefaultTimeout,
	)
}

// AckFeedbackDefault is AckFeedback against the default-resolved socket.
func AckFeedbackDefault(deliveryID string) {
	if deliveryID == "" {
		return
	}
	AckFeedback(adapters.ResolveSocketPath(), deliveryID)
}

// DefaultTimeout is the read deadline for non-blocking requests.
const DefaultTimeout = 30 * time.Second

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
