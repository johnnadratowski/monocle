package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// The engine owns a review's guided tour: the stops the agent sent, and which
// one the reviewer is on. The TUI moves between stops itself and reports where
// it went (SetWalkthroughStop); the agent moves it with goto_stop, which is the
// one path that pushes an event, because the TUI did not start that move.

// Walkthrough event statuses, carried in EventPayload.Status on
// EventWalkthroughChanged. ItemID holds the stop to show.
const (
	WalkthroughEventSet     = "set"     // a tour arrived or was replaced
	WalkthroughEventGoto    = "goto"    // the agent asked for a stop
	WalkthroughEventCleared = "cleared" // the agent withdrew the tour
)

// handleSetWalkthrough replaces the review's tour. Wholesale, like the summary:
// a re-send is the agent's new account of how to read the review. The reviewer
// keeps their place when the stop they are on survives the replace, and starts
// from the top when it does not.
func (e *Engine) handleSetWalkthrough(msg *protocol.SetWalkthroughMsg) *protocol.SetWalkthroughResponse {
	fail := func(message string) *protocol.SetWalkthroughResponse {
		return &protocol.SetWalkthroughResponse{Type: protocol.TypeSetWalkthroughResponse, Message: message}
	}
	w, err := types.NormalizeWalkthrough(msg.Walkthrough)
	if err != nil {
		return fail(err.Error())
	}

	e.mu.Lock()
	session := e.current
	if session == nil {
		e.mu.Unlock()
		return fail("no active session")
	}

	if w.Empty() {
		err := e.database.DeleteWalkthrough(session.ID)
		if err == nil {
			session.Walkthrough = nil
			session.WalkthroughStop = ""
		}
		e.mu.Unlock()
		if err != nil {
			return fail(err.Error())
		}
		e.emit(EventWalkthroughChanged, EventPayload{Kind: EventWalkthroughChanged, Status: WalkthroughEventCleared})
		return &protocol.SetWalkthroughResponse{
			Type: protocol.TypeSetWalkthroughResponse, Success: true, Message: "Walkthrough withdrawn.",
		}
	}

	current := session.WalkthroughStop
	if w.StopIndex(current) < 0 {
		current = w.Stops[0].ID
	}
	if err := e.database.SaveWalkthrough(session.ID, &w, current); err != nil {
		e.mu.Unlock()
		return fail(err.Error())
	}
	session.Walkthrough = &w
	session.WalkthroughStop = current
	inReview := reviewPathSet(session)
	repoRoot := session.RepoRoot
	e.mu.Unlock()

	warnings := walkthroughWarnings(w, func(p string) bool { return inReview[p] }, func(p string) bool {
		_, err := os.Stat(resolveRepoPath(repoRoot, p))
		return err == nil
	})

	e.emit(EventWalkthroughChanged, EventPayload{Kind: EventWalkthroughChanged, Status: WalkthroughEventSet, ItemID: current})

	message := fmt.Sprintf("Walkthrough set (%d stop(s)); the reviewer is on %s.", len(w.Stops), current)
	if len(warnings) > 0 {
		message += "\nWarnings:\n- " + strings.Join(warnings, "\n- ")
	}
	return &protocol.SetWalkthroughResponse{
		Type: protocol.TypeSetWalkthroughResponse, Success: true, Message: message,
		Count: len(w.Stops), Current: current, Warnings: warnings,
	}
}

// handleGotoStop moves the reviewer to a stop at the agent's request, which is
// how the agent answers a question about a stop by showing it.
func (e *Engine) handleGotoStop(msg *protocol.GotoStopMsg) *protocol.GotoStopResponse {
	fail := func(message string) *protocol.GotoStopResponse {
		return &protocol.GotoStopResponse{Type: protocol.TypeGotoStopResponse, Message: message}
	}
	id, prev, heading, err := e.moveToStop(msg.ID)
	if err != nil {
		return fail(err.Error())
	}
	e.emit(EventWalkthroughChanged, EventPayload{Kind: EventWalkthroughChanged, Status: WalkthroughEventGoto, ItemID: id})

	message := fmt.Sprintf("Moved the tour to %s.", heading)
	if prev != "" && prev != id {
		message += fmt.Sprintf(" (The reviewer was on %s.)", prev)
	}
	return &protocol.GotoStopResponse{Type: protocol.TypeGotoStopResponse, Success: true, Message: message}
}

// handleGotoLine shows the reviewer a file of the review at a new-file line,
// at the agent's request: a changed file or an added one, by its repo-relative
// path, an added file's name, or an absolute path. It does not move the tour.
// The TUI does the showing, so the engine checks what it can — the file is in
// the review, the line is a line — and announces where.
func (e *Engine) handleGotoLine(msg *protocol.GotoLineMsg) *protocol.GotoLineResponse {
	fail := func(message string) *protocol.GotoLineResponse {
		return &protocol.GotoLineResponse{Type: protocol.TypeGotoLineResponse, Message: message}
	}
	path := strings.TrimSpace(msg.Path)
	if path == "" {
		return fail("a path is required")
	}
	if msg.Line < 1 {
		return fail(fmt.Sprintf("line must be >= 1 (got %d)", msg.Line))
	}
	e.mu.RLock()
	session := e.current
	if session == nil {
		e.mu.RUnlock()
		return fail("no active session")
	}
	inReview := reviewPathSet(session)
	for _, af := range session.AdditionalFiles {
		inReview[af.Path] = true
	}
	repoRoot := session.RepoRoot
	e.mu.RUnlock()

	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(repoRoot, path); err == nil && inReview[rel] {
			path = rel
		}
	}
	if !inReview[path] {
		return fail(fmt.Sprintf("%s is not in the review: it must be a changed file or an added one (add it with add_files)", path))
	}
	e.emit(EventGotoLine, EventPayload{Kind: EventGotoLine, Path: path, Line: msg.Line})
	return &protocol.GotoLineResponse{
		Type: protocol.TypeGotoLineResponse, Success: true,
		Message: fmt.Sprintf("Showing the reviewer %s:%d.", path, msg.Line),
	}
}

// handleSetWalkthroughStop records a move the TUI already made.
func (e *Engine) handleSetWalkthroughStop(msg *protocol.SetWalkthroughStopMsg) *protocol.SetWalkthroughStopResponse {
	resp := &protocol.SetWalkthroughStopResponse{Type: protocol.TypeSetWalkthroughStopResponse}
	if err := e.SetWalkthroughStop(msg.ID); err != nil {
		resp.Error = err.Error()
	}
	return resp
}

// SetWalkthroughStop records the stop the reviewer is on, so a restart resumes
// there. It emits nothing: the TUI calls it after moving, and an event would
// send it back to where it already is.
func (e *Engine) SetWalkthroughStop(id string) error {
	_, _, _, err := e.moveToStop(id)
	return err
}

// moveToStop makes a stop current and persists it. It returns the stop's id,
// the stop the reviewer was on, and the stop's heading.
func (e *Engine) moveToStop(id string) (string, string, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	session := e.current
	if session == nil {
		return "", "", "", fmt.Errorf("no active session")
	}
	if session.Walkthrough.Empty() {
		return "", "", "", fmt.Errorf("no walkthrough: send one with set_walkthrough first")
	}
	idx := session.Walkthrough.StopIndex(id)
	if idx < 0 {
		return "", "", "", fmt.Errorf("no stop %q; the stops are %s", strings.TrimSpace(id), stopIDList(session.Walkthrough))
	}
	stop := session.Walkthrough.Stops[idx]
	prev := session.WalkthroughStop
	if err := e.database.SetWalkthroughStop(session.ID, stop.ID); err != nil {
		return "", "", "", fmt.Errorf("record stop: %w", err)
	}
	session.WalkthroughStop = stop.ID
	return stop.ID, prev, stop.Heading(), nil
}

// dropWalkthroughLocked removes the tour when the review it explains ends.
// Callers must hold e.mu.
func (e *Engine) dropWalkthroughLocked(sessionID string) error {
	if err := e.database.DeleteWalkthrough(sessionID); err != nil {
		return err
	}
	if e.current != nil && e.current.ID == sessionID {
		e.current.Walkthrough = nil
		e.current.WalkthroughStop = ""
	}
	return nil
}

// stopIDList renders a tour's ids for an error message that has to tell the
// agent what it could have asked for.
func stopIDList(w *types.Walkthrough) string {
	ids := make([]string, len(w.Stops))
	for i, s := range w.Stops {
		ids[i] = s.ID
	}
	return strings.Join(ids, ", ")
}

// reviewPathSet is every repo-relative path the reviewer can be shown: the
// changed files and the agent-attached ones. Callers must hold e.mu.
func reviewPathSet(session *types.ReviewSession) map[string]bool {
	set := make(map[string]bool, len(session.ChangedFiles)+len(session.AdditionalFiles))
	for _, f := range session.ChangedFiles {
		set[f.Path] = true
	}
	for _, af := range session.AdditionalFiles {
		set[af.Name] = true
		if rel, err := filepath.Rel(session.RepoRoot, af.Path); err == nil {
			set[rel] = true
		}
	}
	return set
}

// resolveRepoPath resolves a tour path against the repo root, leaving an
// absolute one alone.
func resolveRepoPath(repoRoot, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(repoRoot, p)
}

// walkthroughWarnings lists what in a tour the reviewer will not be able to see:
// a stop anchored to a file outside the review, and a related file that does
// not exist. The tour is still accepted — a stop's note is worth reading even
// when its anchor is wrong — but the agent is told, since it can fix it and
// the reviewer cannot.
func walkthroughWarnings(w types.Walkthrough, inReview, exists func(string) bool) []string {
	var out []string
	for _, s := range w.Stops {
		if s.File != "" && !inReview(s.File) {
			out = append(out, fmt.Sprintf("stop %s: %s is not in the review, so the diff cannot be shown there (add it with add_files, or fix the path)", s.ID, s.File))
		}
		for _, r := range s.Related {
			if !exists(r.Doc) {
				out = append(out, fmt.Sprintf("stop %s: related file %s does not exist", s.ID, r.Doc))
			}
		}
	}
	return out
}
