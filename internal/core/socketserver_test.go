package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

func setupTestEngine(t *testing.T) (*Engine, string) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	stub := &gitStub{
		repoRoot:   "/tmp/test-repo",
		currentRef: "abc123def456abc123def456abc123def456abc1",
		files: []types.ChangedFile{
			{Path: "test.go", Status: types.FileModified},
		},
	}
	cfg := DefaultConfig()
	server := NewSocketServer()
	feedback := NewFeedbackQueue()
	engine := &Engine{
		database:       database,
		git:            stub,
		server:         server,
		feedback:       feedback,
		sessions:       NewSessionManager(database, stub),
		formatter:      NewReviewFormatter(func(string, int, int) string { return "" }, cfg.ReviewFormat),
		autoAdvanceRef: true,
		subscribers:    make(map[EventKind]map[int]EventCallback),
	}
	engine.cfg.Store(cfg)
	server.SetEngine(engine)

	// Start a session so the engine is usable
	_, err = engine.StartSession(SessionOptions{
		Agent:    "test",
		RepoRoot: stub.repoRoot,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	// Start socket server (use /tmp with short hash to stay within macOS 104-byte socket path limit)
	hash := sha256.Sum256([]byte(t.Name()))
	socketPath := fmt.Sprintf("/tmp/monocle-test-%s.sock", hex.EncodeToString(hash[:])[:8])
	if err := engine.StartServer(socketPath); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { engine.Shutdown() })

	return engine, socketPath
}

func TestSocketServer_OneShot(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	// Connect and send a one-shot review-status request
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus}
	data, _ := protocol.Encode(&msg)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !scanner.Scan() {
		t.Fatal("no response")
	}

	decoded, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp, ok := decoded.(*protocol.GetReviewStatusResponse)
	if !ok {
		t.Fatalf("expected *GetReviewStatusResponse, got %T", decoded)
	}
	if resp.Status != "no_feedback" {
		t.Errorf("status = %q, want %q", resp.Status, "no_feedback")
	}
}

// TestSocketServer_AwaitReviewBoundedWait verifies the end-to-end fail-fast
// path over the real socket: with a dirty session and no reviewer submitting,
// a blocking AwaitReview that carries a max-wait bound returns promptly with
// HasActivity=true and no verdict, so the Stop hook ends the turn normally
// instead of blocking for the installed hook timeout.
func TestSocketServer_AwaitReviewBoundedWait(t *testing.T) {
	engine, socketPath := setupTestEngine(t)
	engine.handleMarkActivity(&protocol.MarkActivityMsg{Type: protocol.TypeMarkActivity})

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := protocol.AwaitReviewMsg{Type: protocol.TypeAwaitReview, Wait: true, MaxWaitMs: 100}
	data, _ := protocol.Encode(&msg)

	start := time.Now()
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("bounded await-review did not return promptly: %v", elapsed)
	}

	decoded, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp, ok := decoded.(*protocol.AwaitReviewResponse)
	if !ok {
		t.Fatalf("expected *AwaitReviewResponse, got %T", decoded)
	}
	if !resp.HasActivity {
		t.Error("expected HasActivity=true (session was dirty)")
	}
	if resp.Action != "" {
		t.Errorf("expected no verdict on bounded timeout, got %q", resp.Action)
	}
}

func TestSocketServer_Subscription(t *testing.T) {
	engine, socketPath := setupTestEngine(t)

	// Connect and subscribe
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send subscribe message
	sub := protocol.SubscribeMsg{
		Type:   protocol.TypeSubscribe,
		Events: []string{string(EventFeedbackSubmitted)},
	}
	data, _ := protocol.Encode(&sub)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read ack
	if !scanner.Scan() {
		t.Fatal("no ack")
	}
	decoded, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	ack, ok := decoded.(*protocol.SubscribeResponse)
	if !ok {
		t.Fatalf("expected *SubscribeResponse, got %T", decoded)
	}
	if !ack.Success {
		t.Fatal("expected success ack")
	}

	// Emit an event from the engine
	engine.emit(EventFeedbackSubmitted, EventPayload{
		Kind:    EventFeedbackSubmitted,
		Message: "## Review — Changes Requested",
		Status:  "request_changes",
	})

	// Read the event notification
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if !scanner.Scan() {
		t.Fatal("no event notification received")
	}

	decoded, err = protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	notif, ok := decoded.(*protocol.EventNotification)
	if !ok {
		t.Fatalf("expected *EventNotification, got %T", decoded)
	}
	if notif.Event != string(EventFeedbackSubmitted) {
		t.Errorf("event = %q, want %q", notif.Event, EventFeedbackSubmitted)
	}
	if notif.Payload["message"] != "## Review — Changes Requested" {
		t.Errorf("payload.message = %q", notif.Payload["message"])
	}
}

func TestSocketServer_SubscriptionWithRequests(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	// Connect and subscribe
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	sub := protocol.SubscribeMsg{
		Type:   protocol.TypeSubscribe,
		Events: []string{string(EventFeedbackSubmitted)},
	}
	data, _ := protocol.Encode(&sub)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read ack
	if !scanner.Scan() {
		t.Fatal("no ack")
	}

	// Send a request/response message on the same connection
	reqMsg := protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus}
	data, _ = protocol.Encode(&reqMsg)
	conn.Write(data)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if !scanner.Scan() {
		t.Fatal("no response to request")
	}

	decoded, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp, ok := decoded.(*protocol.GetReviewStatusResponse)
	if !ok {
		t.Fatalf("expected *GetReviewStatusResponse, got %T", decoded)
	}
	if resp.Status != "no_feedback" {
		t.Errorf("status = %q, want %q", resp.Status, "no_feedback")
	}
}

func TestSocketServer_SubscriberCount(t *testing.T) {
	engine, socketPath := setupTestEngine(t)

	// Initially no subscribers
	if got := engine.GetSubscriberCount(); got != 0 {
		t.Fatalf("initial subscriber count = %d, want 0", got)
	}

	// Connect and subscribe
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	sub := protocol.SubscribeMsg{
		Type:   protocol.TypeSubscribe,
		Events: []string{string(EventFeedbackSubmitted)},
	}
	data, _ := protocol.Encode(&sub)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read ack
	if !scanner.Scan() {
		t.Fatal("no ack")
	}

	// Wait briefly for the count to update
	time.Sleep(50 * time.Millisecond)

	if got := engine.GetSubscriberCount(); got != 1 {
		t.Errorf("subscriber count after connect = %d, want 1", got)
	}

	// Disconnect
	conn.Close()

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	if got := engine.GetSubscriberCount(); got != 0 {
		t.Errorf("subscriber count after disconnect = %d, want 0", got)
	}
}

func TestSocketServer_ConnectionChangedEvent(t *testing.T) {
	engine, socketPath := setupTestEngine(t)

	// Subscribe to connection events
	events := make(chan EventPayload, 10)
	engine.On(EventConnectionChanged, func(e EventPayload) {
		events <- e
	})

	// Connect and subscribe
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	sub := protocol.SubscribeMsg{
		Type:   protocol.TypeSubscribe,
		Events: []string{string(EventFeedbackSubmitted)},
	}
	data, _ := protocol.Encode(&sub)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read ack
	if !scanner.Scan() {
		t.Fatal("no ack")
	}

	// Should get a connection event for connect
	select {
	case e := <-events:
		if e.Status != "1" {
			t.Errorf("connect event status = %q, want %q", e.Status, "1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no connect event received")
	}

	// Disconnect
	conn.Close()

	// Should get a connection event for disconnect
	select {
	case e := <-events:
		if e.Status != "0" {
			t.Errorf("disconnect event status = %q, want %q", e.Status, "0")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no disconnect event received")
	}
}

func TestSocketServer_SubmitEmitsFeedbackEvent(t *testing.T) {
	engine, socketPath := setupTestEngine(t)

	// Add a comment so Submit generates actual feedback
	_, err := engine.AddComment(CommentTarget{
		TargetType: types.TargetFile,
		TargetRef:  "test.go",
		LineStart:  1,
		LineEnd:    1,
	}, types.CommentIssue, "Fix this")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}

	// Connect and subscribe to feedback events
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	sub := protocol.SubscribeMsg{
		Type:   protocol.TypeSubscribe,
		Events: []string{string(EventFeedbackSubmitted)},
	}
	data, _ := protocol.Encode(&sub)
	conn.Write(data)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Read ack
	if !scanner.Scan() {
		t.Fatal("no ack")
	}

	// Submit the review
	if err = engine.Submit(types.ActionRequestChanges, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Should receive an EventNotification
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// May get EventFeedbackStatusChanged first, then EventFeedbackSubmitted
	// Read until we find the feedback_submitted event
	found := false
	for i := 0; i < 5; i++ {
		if !scanner.Scan() {
			break
		}
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		if raw["type"] == protocol.TypeEventNotification && raw["event"] == string(EventFeedbackSubmitted) {
			found = true
			payload, _ := raw["payload"].(map[string]any)
			msg, _ := payload["message"].(string)
			if msg == "" {
				t.Error("expected non-empty feedback message in event payload")
			}
			break
		}
	}
	if !found {
		t.Error("did not receive feedback_submitted event notification")
	}
}

// The review events as a client outside this module sees them: plain NDJSON
// on the socket, a passive subscriber making its own requests on the same
// connection. Each change answers its request and announces itself, so any
// other client showing the review can show it at once. Written against the
// wire's own strings, so renaming a constant cannot pass it silently.
func TestSocketServer_ReviewEventsContract(t *testing.T) {
	_, socketPath := setupTestEngine(t)
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	reviewEventsContract(t, conn, "test.go")
}

// The same contract over an in-memory connection, for where no socket can be
// bound.
func TestReviewEventsContractOverAPipe(t *testing.T) {
	e := eventsEngine(t)
	conn, served := net.Pipe()
	defer conn.Close()
	go e.server.handleConnection(served)
	reviewEventsContract(t, conn, "a.go")
}

func reviewEventsContract(t *testing.T, conn net.Conn, file string) {
	t.Helper()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	send := func(line string) {
		t.Helper()
		if _, err := conn.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("write %s: %v", line, err)
		}
	}
	next := func() map[string]any {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if !scanner.Scan() {
			t.Fatalf("connection ended: %v", scanner.Err())
		}
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatalf("decode %s: %v", scanner.Bytes(), err)
		}
		return m
	}
	// exchange sends a request and returns its answer and the event it fired,
	// in whichever order they arrive.
	exchange := func(req, respType, event string) (resp, payload map[string]any) {
		t.Helper()
		send(req)
		for resp == nil || payload == nil {
			m := next()
			switch {
			case m["type"] == respType:
				resp = m
			case m["type"] == "event_notification" && m["event"] == event:
				payload, _ = m["payload"].(map[string]any)
			default:
				t.Fatalf("after %s: unexpected %v", req, m)
			}
		}
		if e, _ := resp["error"].(string); e != "" {
			t.Fatalf("%s: %s", req, e)
		}
		return resp, payload
	}
	want := func(what string, payload map[string]any, fields map[string]any) {
		t.Helper()
		for k, v := range fields {
			if payload[k] != v {
				t.Errorf("%s: payload[%q] = %v, want %v (payload %v)", what, k, payload[k], v, payload)
			}
		}
	}

	send(`{"type":"subscribe","events":["comment_added","comment_edited","comment_deleted","comment_resolved","reviewed_changed"],"passive":true}`)
	if ack := next(); ack["type"] != "subscribe_response" || ack["success"] != true || ack["protocol_version"] != float64(1) {
		t.Fatalf("ack = %v, want success on protocol 1", ack)
	}

	resp, ev := exchange(`{"type":"add_comment","target_type":"file","target_ref":"`+file+`","line_start":3,"line_end":4,"comment_type":"question","body":"why?","stop_id":"s1"}`,
		"add_comment_response", "comment_added")
	comment, _ := resp["comment"].(map[string]any)
	id, _ := comment["ID"].(string) // ReviewComment carries no json tags
	if id == "" {
		t.Fatalf("add_comment_response = %v, want a comment with an id", resp)
	}
	want("comment_added", ev, map[string]any{"item_id": id, "path": file, "line": float64(3), "line_end": float64(4)})

	_, ev = exchange(`{"type":"edit_comment","comment_id":"`+id+`","comment_type":"issue","body":"why skip it?"}`, "edit_comment_response", "comment_edited")
	want("comment_edited", ev, map[string]any{"item_id": id, "path": file})
	_, ev = exchange(`{"type":"resolve_comment","comment_id":"`+id+`"}`, "resolve_comment_response", "comment_resolved")
	want("comment_resolved", ev, map[string]any{"item_id": id, "status": "resolved"})
	_, ev = exchange(`{"type":"delete_comment","comment_id":"`+id+`"}`, "delete_comment_response", "comment_deleted")
	want("comment_deleted", ev, map[string]any{"item_id": id, "path": file})

	_, ev = exchange(`{"type":"mark_reviewed","path":"`+file+`"}`, "mark_reviewed_response", "reviewed_changed")
	want("mark_reviewed", ev, map[string]any{"path": file, "status": "on"})
	_, ev = exchange(`{"type":"unmark_reviewed","path":"`+file+`"}`, "unmark_reviewed_response", "reviewed_changed")
	want("unmark_reviewed", ev, map[string]any{"path": file, "status": "off"})
}
