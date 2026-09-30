package main

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// Kong names a command after its FIELD, not its cmd tag, so a field called
// SetTour shipped as `set-tour` while every doc said set-walkthrough.
func TestTourCommandsParseByTheirDocumentedNames(t *testing.T) {
	for _, args := range [][]string{
		{"review", "set-walkthrough", "--file", "tour.json"},
		{"review", "set-walkthrough", "--clear"},
		{"review", "goto-stop", "1.2"},
	} {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(args); err != nil {
			t.Errorf("monocle %s: %v", strings.Join(args, " "), err)
		}
	}
}

func TestParseWalkthroughJSON(t *testing.T) {
	t.Run("a tour object", func(t *testing.T) {
		got, err := parseWalkthroughJSON([]byte(`{"title":"T","stops":[{"id":"1.1","file":"a.go","line_start":4,
			"related":[{"doc":"b.go","start_line":9}],"views":[{"kind":"video","target":"d.webm"}],"layout":"review"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		s := got.Stops[0]
		if got.Title != "T" || s.ID != "1.1" || s.LineStart != 4 || s.Related[0].StartLine != 9 || s.Views[0].Target != "d.webm" || s.Layout != "review" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a bare list of stops", func(t *testing.T) {
		got, err := parseWalkthroughJSON([]byte(` [{"id":"a"},{"id":"b"}] `))
		if err != nil || len(got.Stops) != 2 {
			t.Errorf("got %+v %v", got, err)
		}
	})

	// "line" instead of "line_start" would otherwise land the stop at the top of
	// its file with nothing to say why.
	t.Run("an unknown field is refused by name", func(t *testing.T) {
		_, err := parseWalkthroughJSON([]byte(`{"stops":[{"id":"a","line":4}]}`))
		if err == nil || !strings.Contains(err.Error(), `"line"`) {
			t.Errorf("err = %v, want the unknown field named", err)
		}
	})

	t.Run("nothing, or no stops, is a mistake with a way out", func(t *testing.T) {
		for _, in := range []string{"", "  ", `{"title":"x"}`, `[]`} {
			if _, err := parseWalkthroughJSON([]byte(in)); err == nil || !strings.Contains(err.Error(), "--clear") {
				t.Errorf("parse(%q) err = %v, want a pointer to --clear", in, err)
			}
		}
	})
}
