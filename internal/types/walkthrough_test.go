package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeWalkthrough(t *testing.T) {
	t.Run("keeps the agent's order and ids", func(t *testing.T) {
		got, err := NormalizeWalkthrough(Walkthrough{Title: " Tour ", Stops: []WalkthroughStop{
			{ID: "1.2", Title: "second"}, {ID: "1.1", Title: "first"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		// The tour is a reading order the agent chose; sorting ids would impose one.
		if got.Title != "Tour" || got.Stops[0].ID != "1.2" || got.Stops[1].ID != "1.1" {
			t.Errorf("got %+v, want the order sent", got)
		}
	})

	t.Run("numbers a stop sent without an id by its position", func(t *testing.T) {
		got, err := NormalizeWalkthrough(Walkthrough{Stops: []WalkthroughStop{{ID: "a"}, {Title: "no id"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got.Stops[1].ID != "2" {
			t.Errorf("id = %q, want its 1-based position", got.Stops[1].ID)
		}
	})

	// The id is what the reviewer types; two stops answering to "1.2" would make
	// both ambiguous, so this is refused rather than silently renamed.
	t.Run("refuses a duplicate id", func(t *testing.T) {
		_, err := NormalizeWalkthrough(Walkthrough{Stops: []WalkthroughStop{{ID: "1.2"}, {ID: " 1.2 "}}})
		if err == nil || !strings.Contains(err.Error(), `"1.2"`) {
			t.Errorf("err = %v, want a duplicate-id error naming the id", err)
		}
	})

	t.Run("squares up ranges and one-line titles", func(t *testing.T) {
		got, _ := NormalizeWalkthrough(Walkthrough{Stops: []WalkthroughStop{
			{ID: "x", Title: "line one\nline two", LineStart: 40, LineEnd: 3},
		}})
		s := got.Stops[0]
		if s.Title != "line one" || s.LineEnd != 40 {
			t.Errorf("got title %q end %d, want the first line and end clamped up to start", s.Title, s.LineEnd)
		}
	})

	t.Run("related entries are files and views have kinds", func(t *testing.T) {
		got, _ := NormalizeWalkthrough(Walkthrough{Stops: []WalkthroughStop{{
			ID:      "x",
			Related: []DocRef{{Doc: "a.go", StartLine: 4}, {Doc: "  "}, {Kind: DocRefArtifact, Doc: "b.go"}},
			Views: []StopView{
				{Target: "shots/login.png"}, {Target: "demo.webm"}, {Target: "https://x.test"},
				{Target: "NOTES.md"}, {Kind: "Video", Target: "clip.bin"}, {Kind: "image"},
			},
		}}})
		s := got.Stops[0]
		wantRelated := []DocRef{{Kind: DocRefFile, Doc: "a.go", StartLine: 4}, {Kind: DocRefFile, Doc: "b.go"}}
		if !reflect.DeepEqual(s.Related, wantRelated) {
			t.Errorf("related = %+v, want %+v", s.Related, wantRelated)
		}
		var kinds []string
		for _, v := range s.Views {
			kinds = append(kinds, v.Kind)
		}
		want := []string{StopViewImage, StopViewVideo, StopViewURL, StopViewMarkdown, StopViewVideo}
		if !reflect.DeepEqual(kinds, want) {
			t.Errorf("kinds = %v, want %v (the targetless view dropped)", kinds, want)
		}
	})

	t.Run("no stops is not an error", func(t *testing.T) {
		got, err := NormalizeWalkthrough(Walkthrough{})
		if err != nil || !got.Empty() {
			t.Errorf("got %+v %v, want an empty tour", got, err)
		}
	})
}

func TestWalkthroughStopIndexAndHeading(t *testing.T) {
	w := &Walkthrough{Stops: []WalkthroughStop{
		{ID: "1.1", Title: "Entry"}, {ID: "1.2", File: "db.go"}, {ID: "2"},
	}}
	if w.StopIndex("1.2") != 1 || w.StopIndex(" 2 ") != 2 || w.StopIndex("9") != -1 {
		t.Error("StopIndex should find stops by id, trimming what was typed")
	}
	var none *Walkthrough
	if none.StopIndex("1.1") != -1 || !none.Empty() {
		t.Error("a nil walkthrough has no stops")
	}
	for _, c := range []struct{ stop, want string }{
		{"1.1", "1.1 · Entry"}, {"1.2", "1.2 · db.go"}, {"2", "2"},
	} {
		if got := w.Stops[w.StopIndex(c.stop)].Heading(); got != c.want {
			t.Errorf("Heading(%s) = %q, want %q", c.stop, got, c.want)
		}
	}
}

// The walkthrough crosses three boundaries as JSON — the socket, the database
// and the on-stop command's environment — so its wire names are a contract.
func TestWalkthroughJSONRoundTrip(t *testing.T) {
	in := Walkthrough{Title: "T", Stops: []WalkthroughStop{{
		ID: "1.2", Title: "t", File: "a.go", LineStart: 3, LineEnd: 9, Note: "**n**",
		Related: []DocRef{{Kind: DocRefFile, Doc: "b.go", StartLine: 7}},
		Views:   []StopView{{Kind: StopViewVideo, Target: "demo.webm", Label: "Demo"}},
		Layout:  "review",
	}}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"id":"1.2"`, `"line_start":3`, `"line_end":9`, `"related":`, `"views":`, `"layout":"review"`, `"doc":"b.go"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("encoded tour %s is missing %s", data, key)
		}
	}
	var out Walkthrough
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip changed the tour:\n in  %+v\n out %+v", in, out)
	}
}
