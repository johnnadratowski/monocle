package types

import "testing"

func TestSummaryTargetCovers(t *testing.T) {
	cases := []struct {
		name   string
		target SummaryTarget
		path   string
		line   int
		want   bool
	}{
		{"a range covers its own lines", SummaryTarget{Path: "a.go", LineStart: 10, LineEnd: 20}, "a.go", 15, true},
		{"a range covers its first line", SummaryTarget{Path: "a.go", LineStart: 10, LineEnd: 20}, "a.go", 10, true},
		{"a range covers its last line", SummaryTarget{Path: "a.go", LineStart: 10, LineEnd: 20}, "a.go", 20, true},
		{"a range stops at its edges", SummaryTarget{Path: "a.go", LineStart: 10, LineEnd: 20}, "a.go", 21, false},
		{"another file is never covered", SummaryTarget{Path: "a.go", LineStart: 10, LineEnd: 20}, "b.go", 15, false},
		// A change that IS the file should not have to enumerate its lines.
		{"no range claims the whole file", SummaryTarget{Path: "a.go"}, "a.go", 9999, true},
		{"a whole-file target covers line zero", SummaryTarget{Path: "a.go"}, "a.go", 0, true},
		// A single line sent without an end is the common shorthand.
		{"a bare start is one line", SummaryTarget{Path: "a.go", LineStart: 7}, "a.go", 7, true},
		{"a bare start is only that line", SummaryTarget{Path: "a.go", LineStart: 7}, "a.go", 8, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.target.Covers(c.path, c.line); got != c.want {
				t.Errorf("Covers(%q, %d) = %v, want %v", c.path, c.line, got, c.want)
			}
		})
	}
}

func TestSummaryItemClaimsFile(t *testing.T) {
	item := SummaryItem{Targets: []SummaryTarget{
		{Path: "a.go", LineStart: 10, LineEnd: 20},
		{Path: "b.go"},
	}}
	// Claiming a file is weaker than covering a line in it: the sidebar fades on
	// the first question, the gutter colours on the second.
	if !item.ClaimsFile("a.go") {
		t.Error("a.go is targeted, even if only in part")
	}
	if item.Covers("a.go", 99) {
		t.Error("line 99 is outside the targeted range")
	}
	if item.ClaimsFile("c.go") {
		t.Error("c.go is not targeted at all")
	}
}

func TestNormalizeSummaryItems(t *testing.T) {
	t.Run("orders by the agent's reading order", func(t *testing.T) {
		got := NormalizeSummaryItems([]SummaryItem{
			{ID: "c", Text: "third", Order: 3},
			{ID: "a", Text: "first", Order: 1},
			{ID: "b", Text: "second", Order: 2},
		})
		for i, want := range []string{"a", "b", "c"} {
			if got[i].ID != want {
				t.Errorf("position %d = %q, want %q", i, got[i].ID, want)
			}
		}
	})

	// Equal orders must keep arrival order: items are colour-coded by position,
	// and an unstable sort would repaint them differently every round.
	t.Run("equal orders keep arrival order", func(t *testing.T) {
		got := NormalizeSummaryItems([]SummaryItem{
			{ID: "x", Text: "one"}, {ID: "y", Text: "two"}, {ID: "z", Text: "three"},
		})
		for i, want := range []string{"x", "y", "z"} {
			if got[i].ID != want {
				t.Errorf("position %d = %q, want %q", i, got[i].ID, want)
			}
		}
	})

	t.Run("fills in missing and duplicate ids", func(t *testing.T) {
		got := NormalizeSummaryItems([]SummaryItem{
			{Text: "no id"}, {ID: "dup", Text: "first dup"}, {ID: "dup", Text: "second dup"},
		})
		seen := map[string]bool{}
		for _, it := range got {
			if it.ID == "" {
				t.Error("every item needs an id to be selectable")
			}
			if seen[it.ID] {
				t.Errorf("duplicate id %q survived", it.ID)
			}
			seen[it.ID] = true
		}
	})

	t.Run("drops items with nothing to say", func(t *testing.T) {
		got := NormalizeSummaryItems([]SummaryItem{{ID: "a", Text: "   "}, {ID: "b", Text: "real"}})
		if len(got) != 1 || got[0].ID != "b" {
			t.Errorf("got %+v, want only the item with text", got)
		}
	})

	t.Run("drops targets with no path and squares up ranges", func(t *testing.T) {
		got := NormalizeSummaryItems([]SummaryItem{{ID: "a", Text: "t", Targets: []SummaryTarget{
			{Path: ""}, {Path: "a.go", LineStart: 10, LineEnd: 2},
		}}})
		if len(got[0].Targets) != 1 {
			t.Fatalf("targets = %+v, want the pathless one dropped", got[0].Targets)
		}
		if got[0].Targets[0].LineEnd != 10 {
			t.Errorf("an end before the start should clamp up to it, got %d", got[0].Targets[0].LineEnd)
		}
	})

	t.Run("no items is not an error", func(t *testing.T) {
		if got := NormalizeSummaryItems(nil); len(got) != 0 {
			t.Errorf("got %+v, want empty", got)
		}
	})
}
