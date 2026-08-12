package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// blockNavModel builds a diff view whose lines are the given source, one
// context line each, so the structure motions have something real to walk.
func blockNavModel(src string) diffViewModel {
	var lines []diffViewLine
	for i, s := range strings.Split(strings.TrimPrefix(src, "\n"), "\n") {
		lines = append(lines, diffViewLine{
			kind:       types.DiffLineContext,
			content:    s,
			newLineNum: i + 1,
		})
	}
	return diffViewModel{lines: lines, height: len(lines) + 1, width: 80, tabSize: 4}
}

// goSrc is the shape the motions exist for: a func holding an if, holding a
// nested if. Line numbers are 0-based indices into the model.
//
//	0 func handle(r *Request) error {
//	1     if r == nil {
//	2         return errNil
//	3     }
//	4     if r.Body != "" {
//	5         if err := parse(r); err != nil {
//	6             return err
//	7         }
//	8         log("ok")
//	9     }
//	10    return nil
//	11 }
const goSrc = `
func handle(r *Request) error {
	if r == nil {
		return errNil
	}
	if r.Body != "" {
		if err := parse(r); err != nil {
			return err
		}
		log("ok")
	}
	return nil
}`

func TestJumpToEnclosingBlock(t *testing.T) {
	tests := []struct {
		name       string
		from, want int
	}{
		{"body of a nested if goes to the if", 6, 5},
		{"the nested if goes to the outer if", 5, 4},
		{"the outer if goes to the func", 4, 0},
		{"a statement after a nested block still goes to the outer if", 8, 4},
		{"a closing brace goes to what it closes", 7, 5},
		{"the func's own closing brace goes to the func", 11, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := blockNavModel(goSrc)
			m.cursor = tt.from
			if !m.JumpToEnclosingBlock() {
				t.Fatalf("expected a jump from line %d", tt.from)
			}
			if m.cursor != tt.want {
				t.Errorf("landed on %d (%q), want %d (%q)",
					m.cursor, m.lines[m.cursor].content, tt.want, m.lines[tt.want].content)
			}
		})
	}

	t.Run("already at the top level is a no-op", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 0
		if m.JumpToEnclosingBlock() {
			t.Errorf("expected no jump, cursor moved to %d", m.cursor)
		}
		if m.cursor != 0 {
			t.Errorf("cursor should not have moved, got %d", m.cursor)
		}
	})
}

func TestJumpToTopLevelBlock(t *testing.T) {
	for _, from := range []int{2, 4, 6, 8, 10, 11} {
		m := blockNavModel(goSrc)
		m.cursor = from
		if !m.JumpToTopLevelBlock() {
			t.Fatalf("expected a jump from line %d", from)
		}
		if m.cursor != 0 {
			t.Errorf("from %d landed on %d (%q), want the func at 0",
				from, m.cursor, m.lines[m.cursor].content)
		}
	}

	t.Run("on the top-level line itself is a no-op", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 0
		if m.JumpToTopLevelBlock() {
			t.Errorf("expected no jump, cursor moved to %d", m.cursor)
		}
	})
}

func TestJumpToBlockMatch(t *testing.T) {
	t.Run("a block's opening line jumps to its closing brace", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 0
		if !m.JumpToBlockMatch() || m.cursor != 11 {
			t.Errorf("func header should jump to its closing brace at 11, got %d", m.cursor)
		}
	})

	t.Run("a closing brace jumps back to the opening line", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 11
		if !m.JumpToBlockMatch() || m.cursor != 0 {
			t.Errorf("closing brace should jump back to the func at 0, got %d", m.cursor)
		}
	})

	t.Run("repeated presses toggle between a block's edges", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 5
		m.JumpToBlockMatch()
		if m.cursor != 7 {
			t.Fatalf("nested if should jump to its brace at 7, got %d", m.cursor)
		}
		m.JumpToBlockMatch()
		if m.cursor != 5 {
			t.Errorf("and back to the if at 5, got %d", m.cursor)
		}
	})

	t.Run("from inside a block it goes out to the opening line", func(t *testing.T) {
		m := blockNavModel(goSrc)
		m.cursor = 6 // `return err`, balanced
		if !m.JumpToBlockMatch() || m.cursor != 5 {
			t.Errorf("expected the enclosing if at 5, got %d", m.cursor)
		}
	})
}

func TestBlockNavIndentationLanguages(t *testing.T) {
	// Python has no closing delimiter, so the motions run entirely on indent.
	//
	//	0 def handle(r):
	//	1     if r is None:
	//	2         return ERR
	//	3     for k in r:
	//	4         if k:
	//	5             log(k)
	//	6     return None
	const py = `
def handle(r):
    if r is None:
        return ERR
    for k in r:
        if k:
            log(k)
    return None`

	t.Run("up one level walks the indent chain", func(t *testing.T) {
		m := blockNavModel(py)
		m.cursor = 5
		m.JumpToEnclosingBlock()
		if m.cursor != 4 {
			t.Fatalf("want the `if k:` at 4, got %d", m.cursor)
		}
		m.JumpToEnclosingBlock()
		if m.cursor != 3 {
			t.Fatalf("want the `for` at 3, got %d", m.cursor)
		}
		m.JumpToEnclosingBlock()
		if m.cursor != 0 {
			t.Errorf("want the `def` at 0, got %d", m.cursor)
		}
	})

	t.Run("top level jumps straight to the def", func(t *testing.T) {
		m := blockNavModel(py)
		m.cursor = 5
		if !m.JumpToTopLevelBlock() || m.cursor != 0 {
			t.Errorf("want the `def` at 0, got %d", m.cursor)
		}
	})

	t.Run("a header jumps to its last indented line", func(t *testing.T) {
		m := blockNavModel(py)
		m.cursor = 3 // `for k in r:`
		if !m.JumpToBlockMatch() || m.cursor != 5 {
			t.Errorf("want the block's last line at 5, got %d", m.cursor)
		}
	})
}

func TestBlockNavSkipsRemovedLines(t *testing.T) {
	// A rewritten condition: the old line's brace must not be counted, or the
	// depth never rebalances and `%` finds nothing.
	//
	//	0  func f() {
	//	1 -    if a {
	//	2 +    if b {
	//	3          x()
	//	4      }
	//	5  }
	m := blockNavModel("\nfunc f() {\n\tif a {\n\tif b {\n\t\tx()\n\t}\n}")
	m.lines[1].kind = types.DiffLineRemoved
	m.lines[1].newLineNum = 0
	m.lines[2].kind = types.DiffLineAdded

	m.cursor = 3 // x()
	if !m.JumpToEnclosingBlock() {
		t.Fatal("expected a jump out of the if body")
	}
	if m.cursor != 2 {
		t.Errorf("want the added `if b {` at 2, got %d (%q)", m.cursor, m.lines[m.cursor].content)
	}

	m.cursor = 0
	if !m.JumpToBlockMatch() || m.cursor != 5 {
		t.Errorf("func should match its closing brace at 5, got %d", m.cursor)
	}
}

func TestBlockNavNoTargetIsANoOp(t *testing.T) {
	m := blockNavModel("\npackage tui\n\nvar x = 1")
	for _, tt := range []struct {
		name string
		fn   func() bool
	}{
		{"match", m.JumpToBlockMatch},
		{"up", m.JumpToEnclosingBlock},
		{"top", m.JumpToTopLevelBlock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m.cursor = 2
			if tt.fn() {
				t.Errorf("flat top-level code has nowhere to go, cursor moved to %d", m.cursor)
			}
		})
	}

	t.Run("empty view", func(t *testing.T) {
		empty := &diffViewModel{}
		if empty.JumpToBlockMatch() || empty.JumpToEnclosingBlock() || empty.JumpToTopLevelBlock() {
			t.Error("an empty diff should have no structure to navigate")
		}
	})
}

func TestNetBracketDepth(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"opens a block", "func f() {", 1},
		{"closes a block", "}", -1},
		{"balanced call", "log(fmt.Sprintf(\"%d\", n))", 0},
		{"brackets in a string don't count", `s := "{{{"`, 0},
		{"brackets in a rune literal don't count", `if c == '{' {`, 1},
		{"escaped quote doesn't end the string", `s := "\"{"`, 0},
		{"line comment ends the scan", "x := 1 // }}}", 0},
		{"hash comment ends the scan", "x = 1  # }}}", 0},
		{"closes then opens", "} else {", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := netBracketDepth(tt.src); got != tt.want {
				t.Errorf("netBracketDepth(%q) = %d, want %d", tt.src, got, tt.want)
			}
		})
	}
}

func TestBlockNavIgnoresOverlays(t *testing.T) {
	// A comment box between the if and its body must not look like a dedent and
	// end the block early.
	m := blockNavModel("\nfunc f() {\n\tif a {\n\t\tx()\n\t\ty()\n\t}\n}")
	withComment := m
	withComment.lines = append([]diffViewLine{}, m.lines[:3]...)
	withComment.lines = append(withComment.lines, diffViewLine{isComment: true, content: "a note"})
	withComment.lines = append(withComment.lines, m.lines[3:]...)

	withComment.cursor = 4 // y(), now one past the comment box
	if !withComment.JumpToEnclosingBlock() {
		t.Fatal("expected a jump out of the if body")
	}
	if got := strings.TrimSpace(withComment.lines[withComment.cursor].content); got != "if a {" {
		t.Errorf("want the enclosing if, got %q", got)
	}
}
