package tui

import "testing"

func TestJumpList(t *testing.T) {
	a := jumpPos{path: "a.go", line: 10}
	b := jumpPos{path: "b.go", line: 20}
	c := jumpPos{path: "c.go", line: 30}

	t.Run("back walks departures newest first", func(t *testing.T) {
		var j jumpList
		j.push(a)
		j.push(b)
		got, ok := j.back(c)
		if !ok || got != b {
			t.Fatalf("first back = %v (%v), want %v", got, ok, b)
		}
		got, ok = j.back(c)
		if !ok || got != a {
			t.Fatalf("second back = %v (%v), want %v", got, ok, a)
		}
		if _, ok := j.back(c); ok {
			t.Error("expected no further history past the oldest entry")
		}
	})

	t.Run("forward returns to where back started", func(t *testing.T) {
		var j jumpList
		j.push(a)
		j.push(b)
		j.back(c)
		j.back(c)
		got, ok := j.forward()
		if !ok || got != b {
			t.Fatalf("forward = %v (%v), want %v", got, ok, b)
		}
		got, ok = j.forward()
		if !ok || got != c {
			t.Fatalf("forward should return to the present %v, got %v (%v)", c, got, ok)
		}
		if _, ok := j.forward(); ok {
			t.Error("expected nothing past the newest entry")
		}
	})

	t.Run("a new jump discards the abandoned future", func(t *testing.T) {
		var j jumpList
		j.push(a)
		j.push(b)
		j.back(c) // now sitting at b, with c reachable forward
		j.push(jumpPos{path: "d.go", line: 1})
		if _, ok := j.forward(); ok {
			t.Error("branching should drop the forward history, as vim and undo stacks do")
		}
	})

	t.Run("repeat jumps in one file collapse to the latest line", func(t *testing.T) {
		var j jumpList
		j.push(jumpPos{path: "a.go", line: 1})
		j.push(jumpPos{path: "a.go", line: 5})
		j.push(jumpPos{path: "a.go", line: 9})
		if len(j.entries) != 1 {
			t.Fatalf("expected one entry per file, got %d: %v", len(j.entries), j.entries)
		}
		if got, _ := j.back(b); got.line != 9 {
			t.Errorf("should return to the line actually left (9), got %d", got.line)
		}
	})

	t.Run("an empty position is not recorded", func(t *testing.T) {
		var j jumpList
		j.push(jumpPos{})
		if len(j.entries) != 0 {
			t.Errorf("nothing to return to should record nothing, got %v", j.entries)
		}
	})

	t.Run("artifacts are distinct from files", func(t *testing.T) {
		var j jumpList
		j.push(jumpPos{contentID: "plan-1", line: 3})
		j.push(a)
		got, ok := j.back(b)
		if !ok || got.contentID != "plan-1" {
			// a.go was pushed last, so the first back is a.go; the artifact is next.
			if got != a {
				t.Fatalf("unexpected first back: %v", got)
			}
		}
	})

	t.Run("history is bounded", func(t *testing.T) {
		var j jumpList
		for i := 0; i < maxJumpList*2; i++ {
			j.push(jumpPos{path: string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".go", line: i})
		}
		if len(j.entries) > maxJumpList {
			t.Errorf("entries = %d, want at most %d", len(j.entries), maxJumpList)
		}
	})
}
