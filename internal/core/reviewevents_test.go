package core

import (
	"reflect"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/types"
)

// eventsEngine is setupTestEngine without the socket: a session on one
// changed file, a.go.
func eventsEngine(t *testing.T) *Engine {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	stub := &gitStub{
		repoRoot:   "/tmp/test-repo",
		currentRef: "abc123def456abc123def456abc123def456abc1",
		files:      []types.ChangedFile{{Path: "a.go", Status: types.FileModified}},
	}
	cfg := DefaultConfig()
	e := &Engine{
		database:       database,
		git:            stub,
		server:         NewSocketServer(),
		feedback:       NewFeedbackQueue(),
		sessions:       NewSessionManager(database, stub),
		formatter:      NewReviewFormatter(func(string, int, int) string { return "" }, cfg.ReviewFormat),
		autoAdvanceRef: true,
		subscribers:    make(map[EventKind]map[int]EventCallback),
	}
	e.cfg.Store(cfg)
	e.server.SetEngine(e)
	if _, err := e.StartSession(SessionOptions{Agent: "test", RepoRoot: stub.repoRoot}); err != nil {
		t.Fatalf("start session: %v", err)
	}
	return e
}

// take returns the events seen since the last take.
func (l *eventLog) take() []EventPayload {
	l.mu.Lock()
	defer l.mu.Unlock()
	got := l.events
	l.events = nil
	return got
}

// record subscribes to kinds the way a client showing the review would: each
// event reads the session again.
func record(e *Engine, kinds ...EventKind) *eventLog {
	l := &eventLog{}
	for _, k := range kinds {
		e.On(k, func(p EventPayload) {
			_ = e.GetSession()
			l.mu.Lock()
			l.events = append(l.events, p)
			l.mu.Unlock()
		})
	}
	return l
}

// settle runs a change and fails if it has not returned within two seconds:
// an event emitted while the engine holds its lock never returns, since the
// subscriber's read waits on that lock.
func settle(t *testing.T, what string, change func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- change() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: still running after 2s (an event emitted under the engine's lock?)", what)
	}
}

func wantEvents(t *testing.T, what string, got []EventPayload, want ...EventPayload) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got  %+v\n want %+v", what, got, want)
	}
}

var commentKinds = []EventKind{EventCommentAdded, EventCommentEdited, EventCommentDeleted, EventCommentResolved}

func TestCommentEvents(t *testing.T) {
	e := eventsEngine(t)
	log := record(e, commentKinds...)

	var c *types.ReviewComment
	settle(t, "add", func() (err error) {
		c, err = e.AddComment(CommentTarget{TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 3, LineEnd: 4, StopID: "s1"}, types.CommentQuestion, "why?")
		return err
	})
	at := func(kind EventKind, status string) EventPayload {
		return EventPayload{Kind: kind, ItemID: c.ID, Path: "a.go", Line: 3, LineEnd: 4, Status: status}
	}
	wantEvents(t, "add", log.take(), at(EventCommentAdded, ""))

	settle(t, "edit", func() error { _, err := e.EditComment(c.ID, types.CommentIssue, "why skip it?"); return err })
	wantEvents(t, "edit", log.take(), at(EventCommentEdited, ""))

	settle(t, "resolve", func() error { return e.ResolveComment(c.ID) })
	wantEvents(t, "resolve", log.take(), at(EventCommentResolved, "resolved"))
	settle(t, "reopen", func() error { return e.ResolveComment(c.ID) })
	wantEvents(t, "reopen", log.take(), at(EventCommentResolved, "open"))

	settle(t, "delete", func() error { return e.DeleteComment(c.ID) })
	wantEvents(t, "delete", log.take(), at(EventCommentDeleted, ""))

	settle(t, "add on the file", func() (err error) {
		c, err = e.AddComment(CommentTarget{TargetType: types.TargetFile, TargetRef: "a.go"}, types.CommentNote, "fine")
		return err
	})
	wantEvents(t, "add on the file", log.take(), EventPayload{Kind: EventCommentAdded, ItemID: c.ID, Path: "a.go"})
	settle(t, "clear", e.ClearComments)
	wantEvents(t, "clear: every comment", log.take(), EventPayload{Kind: EventCommentDeleted})
}

func TestCommentEventsOnlyForAChange(t *testing.T) {
	e := eventsEngine(t)
	log := record(e, commentKinds...)
	if _, err := e.EditComment("nope", types.CommentNote, "x"); err == nil {
		t.Fatal("edit of an unknown comment: want an error")
	}
	if err := e.ResolveComment("nope"); err == nil {
		t.Fatal("resolve of an unknown comment: want an error")
	}
	e.mu.Lock()
	e.current = nil
	e.mu.Unlock()
	if _, err := e.AddComment(CommentTarget{TargetType: types.TargetFile, TargetRef: "a.go"}, types.CommentNote, "x"); err == nil {
		t.Fatal("add with no session: want an error")
	}
	if err := e.DeleteComment("nope"); err == nil {
		t.Fatal("delete with no session: want an error")
	}
	if err := e.ClearComments(); err == nil {
		t.Fatal("clear with no session: want an error")
	}
	wantEvents(t, "failed changes", log.take())
}

func TestReviewedEvents(t *testing.T) {
	e := eventsEngine(t)
	if err := e.SubmitContentForReview("plan", "Plan", "# Plan", "markdown", true); err != nil {
		t.Fatalf("submit content: %v", err)
	}
	log := record(e, EventReviewedChanged)
	changed := func(path, item, status string) EventPayload {
		return EventPayload{Kind: EventReviewedChanged, Path: path, ItemID: item, Status: status}
	}

	settle(t, "mark", func() error { return e.MarkReviewed("a.go") })
	wantEvents(t, "mark", log.take(), changed("a.go", "", "on"))
	settle(t, "unmark", func() error { return e.UnmarkReviewed("a.go") })
	wantEvents(t, "unmark", log.take(), changed("a.go", "", "off"))

	settle(t, "mark content", func() error { return e.MarkContentReviewed("plan") })
	wantEvents(t, "mark content", log.take(), changed("", "plan", "on"))
	settle(t, "unmark content", func() error { return e.UnmarkContentReviewed("plan") })
	wantEvents(t, "unmark content", log.take(), changed("", "plan", "off"))

	settle(t, "mark all", e.MarkAllReviewed)
	wantEvents(t, "mark all", log.take(), changed("", "", "on"))
	settle(t, "reset all", e.ResetAllReviewed)
	wantEvents(t, "reset all", log.take(), changed("", "", "off"))

	// With review tracking off the marks change nothing, so nothing fires.
	cfg := DefaultConfig()
	cfg.ReviewTracking = false
	e.cfg.Store(cfg)
	for _, change := range []func() error{
		func() error { return e.MarkReviewed("a.go") }, func() error { return e.UnmarkReviewed("a.go") },
		func() error { return e.MarkContentReviewed("plan") }, func() error { return e.UnmarkContentReviewed("plan") },
		e.MarkAllReviewed, e.ResetAllReviewed,
	} {
		settle(t, "tracking off", change)
	}
	wantEvents(t, "tracking off", log.take())

	e.cfg.Store(DefaultConfig())
	e.mu.Lock()
	e.current = nil
	e.mu.Unlock()
	if err := e.MarkReviewed("a.go"); err == nil {
		t.Fatal("mark with no session: want an error")
	}
	wantEvents(t, "no session", log.take())
}

// A full clear takes every comment and every mark with it.
func TestClearReviewEvents(t *testing.T) {
	e := eventsEngine(t)
	if _, err := e.AddComment(CommentTarget{TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 1, LineEnd: 1}, types.CommentNote, "x"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := e.MarkReviewed("a.go"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	log := record(e, append(commentKinds, EventReviewedChanged)...)
	settle(t, "clear review", e.ClearReview)
	wantEvents(t, "clear review", log.take(), EventPayload{Kind: EventCommentDeleted}, EventPayload{Kind: EventReviewedChanged, Status: "off"})
}

// What a socket subscriber receives for them.
func TestReviewEventPayloadOnTheWire(t *testing.T) {
	got := eventPayloadMap(EventPayload{Kind: EventCommentResolved, ItemID: "c1", Path: "a.go", Line: 3, LineEnd: 4, Status: "resolved"})
	want := map[string]any{"message": "", "status": "resolved", "path": "a.go", "item_id": "c1", "line": 3, "line_end": 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("comment: got %v, want %v", got, want)
	}
	got = eventPayloadMap(EventPayload{Kind: EventReviewedChanged, Path: "a.go", Status: "on"})
	want = map[string]any{"message": "", "status": "on", "path": "a.go", "item_id": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reviewed: got %v, want %v", got, want)
	}
}
