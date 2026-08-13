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
//
// It carries the view the position was recorded in, because a line number only
// means something inside a particular view. A line recorded in whole-file mode
// often does not exist in the compact diff — it is an unchanged line the hunks
// omit — so returning to it without also returning to the view lands nowhere
// and reads as "ctrl+o is broken".
type jumpPos struct {
	path      string // repo-relative file path ("" for an artifact)
	contentID string // artifact id ("" for a file)
	line      int    // 1-based new-file line number, 0 when unknown
	fullFile  bool   // whole-file diff was on
}

func (p jumpPos) empty() bool { return p.path == "" && p.contentID == "" }

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
//
// Duplicates are removed by EXACT position, which is vim's rule. An earlier
// version collapsed every entry for the same file into one, which quietly made
// ctrl+o a single-step operation: three `]` presses inside a file left one
// entry, so the first ctrl+o worked and the rest had nowhere to go.
func (j *jumpList) push(from jumpPos) {
	if from.empty() {
		return
	}
	j.entries = j.entries[:min(j.idx, len(j.entries))]
	for i, e := range j.entries {
		if e.samePlace(from) {
			j.entries = append(j.entries[:i], j.entries[i+1:]...)
			break
		}
	}
	j.entries = append(j.entries, from)
	if len(j.entries) > maxJumpList {
		j.entries = j.entries[len(j.entries)-maxJumpList:]
	}
	j.idx = len(j.entries)
}

// samePlace compares where a position is, ignoring the view it was seen in, so
// revisiting a line in a different mode moves the existing entry rather than
// adding a near-duplicate one line apart.
func (p jumpPos) samePlace(q jumpPos) bool {
	return p.path == q.path && p.contentID == q.contentID && p.line == q.line
}

// back returns the previous position, or false at the oldest entry. The
// position being left is remembered so forward() can return to it.
func (j *jumpList) back(current jumpPos) (jumpPos, bool) {
	if j.idx <= 0 || j.idx > len(j.entries) {
		return jumpPos{}, false
	}
	if j.idx == len(j.entries) && !current.empty() {
		// First step back from the present: park the present on the forward
		// side so ctrl+i can undo the whole excursion. Skip it when the present
		// is already the newest entry, which would otherwise make the first
		// ctrl+i a no-op that appears to do nothing.
		if n := len(j.entries); n == 0 || !j.entries[n-1].samePlace(current) {
			j.entries = append(j.entries, current)
		}
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
