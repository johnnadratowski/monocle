package tui

// The jump list: vim's ctrl+o / ctrl+i, over review positions rather than
// buffer marks.
//
// Reading a diff is full of one-way trips. You follow a `[` to a chunk three
// files away, chase a `)` out to the enclosing func, land on a search match —
// and then want to be back where you were thinking. Without a jump list the
// only way back is to remember the file and the line, which is exactly the
// thing you were trying not to spend attention on.
//
// A position is recorded before a jump, never after, so the list holds the
// places you left rather than the places you arrived. That is what makes
// ctrl+o feel like undo for navigation.

// jumpPos is a place in the review worth returning to. Content artifacts have
// no path, so contentID distinguishes them from files.
type jumpPos struct {
	path      string // repo-relative file path ("" for an artifact)
	contentID string // artifact id ("" for a file)
	line      int    // 1-based new-file line number, 0 when unknown
}

func (p jumpPos) empty() bool { return p.path == "" && p.contentID == "" }

// sameTarget reports whether two positions are the same file or artifact,
// ignoring the line. Used to collapse repeated jumps within one file into a
// single entry rather than letting a run of `[` presses fill the list.
func (p jumpPos) sameTarget(q jumpPos) bool {
	return p.path == q.path && p.contentID == q.contentID
}

// maxJumpList bounds the history. Deep enough to cover a review's worth of
// wandering, shallow enough that ctrl+o never becomes an archaeology dig.
const maxJumpList = 50

// jumpList is a cursor into a stack of visited positions. Entries before the
// cursor are where you came from; entries after are where ctrl+o brought you
// back from, reachable again with ctrl+i.
type jumpList struct {
	entries []jumpPos
	// idx is the position ctrl+o would move to next. It equals len(entries)
	// when the list is "at the present" — no back-jumps taken yet.
	idx int
	// current is where the cursor is right now. It is not in entries; it is
	// what gets pushed onto the forward side when ctrl+o walks backwards, so
	// ctrl+i can return to it.
	current jumpPos
}

// push records a departure point. Call it with the position being LEFT, before
// the cursor moves.
//
// A new jump truncates the forward history, the same as vim and as every
// undo/redo stack: once you branch, the abandoned future is gone rather than
// silently reachable from a place it never followed.
func (j *jumpList) push(from jumpPos) {
	if from.empty() {
		return
	}
	// Walking within one file is not a jump worth recording twice; keep the
	// most recent line so ctrl+o returns to where you actually left off.
	if n := len(j.entries); n > 0 && j.entries[n-1].sameTarget(from) {
		j.entries[n-1] = from
		j.idx = len(j.entries)
		return
	}
	j.entries = append(j.entries[:min(j.idx, len(j.entries))], from)
	if len(j.entries) > maxJumpList {
		j.entries = j.entries[len(j.entries)-maxJumpList:]
	}
	j.idx = len(j.entries)
}

// back returns the previous position, or false at the oldest entry. The
// position being left is remembered so forward() can return to it.
func (j *jumpList) back(current jumpPos) (jumpPos, bool) {
	if j.idx <= 0 || j.idx > len(j.entries) {
		return jumpPos{}, false
	}
	if j.idx == len(j.entries) && !current.empty() {
		// First step back from the present: park the present on the forward
		// side so ctrl+i can undo the whole excursion.
		j.entries = append(j.entries, current)
	}
	j.idx--
	return j.entries[j.idx], true
}

// forward returns the next position, or false when already at the newest.
func (j *jumpList) forward() (jumpPos, bool) {
	if j.idx+1 >= len(j.entries) {
		return jumpPos{}, false
	}
	j.idx++
	return j.entries[j.idx], true
}

// reset clears the history. Used when the session changes underneath the list,
// where the recorded positions describe a review that is no longer open.
func (j *jumpList) reset() {
	j.entries = nil
	j.idx = 0
	j.current = jumpPos{}
}
