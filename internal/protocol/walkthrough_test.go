package protocol

import (
	"reflect"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// A tour crosses the socket twice: in SetWalkthroughMsg from the agent, and back
// out to the TUI inside the session. Both must carry every field of every stop;
// a dropped tag would lose a note or a view without any error.
func TestWalkthroughSurvivesTheWire(t *testing.T) {
	tour := types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{{
		ID: "1.2", Title: "Where it lands", File: "db/q.go", LineStart: 40, LineEnd: 52,
		Note:    "The **write** happens here.",
		Related: []types.DocRef{{Kind: types.DocRefFile, Doc: "api/h.go", StartLine: 12}},
		Views:   []types.StopView{{Kind: types.StopViewVideo, Target: "demo.webm", Label: "Demo"}},
		Layout:  "review",
	}}}

	roundTrip := func(t *testing.T, in any) any {
		t.Helper()
		data, err := Encode(in)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		out, err := Decode(data[:len(data)-1])
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	t.Run("set_walkthrough", func(t *testing.T) {
		got := roundTrip(t, &SetWalkthroughMsg{Type: TypeSetWalkthrough, Walkthrough: tour}).(*SetWalkthroughMsg)
		if !reflect.DeepEqual(got.Walkthrough, tour) {
			t.Errorf("got %+v, want %+v", got.Walkthrough, tour)
		}
	})

	t.Run("the session the TUI reads", func(t *testing.T) {
		in := &GetSessionResponse{Type: TypeGetSessionResponse, Session: &types.ReviewSession{
			ID: "s1", Walkthrough: &tour, WalkthroughStop: "1.2",
		}}
		got := roundTrip(t, in).(*GetSessionResponse)
		if !reflect.DeepEqual(got.Session.Walkthrough, &tour) || got.Session.WalkthroughStop != "1.2" {
			t.Errorf("got %+v at %q", got.Session.Walkthrough, got.Session.WalkthroughStop)
		}
	})
}
