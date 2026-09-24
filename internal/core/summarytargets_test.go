package core

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// checkSummaryTargets exists because the old failure was silent: a mistyped path
// stored fine, showed in the modal, and filtered to an empty review.
func TestUnmatchedTargetsAreReported(t *testing.T) {
	e, _ := summaryEngine(t)
	e.mu.Lock()
	e.current.ChangedFiles = []types.ChangedFile{{Path: "hello.go"}}
	e.current.ContentItems = []types.ContentItem{{ID: "plan", Content: "one\ntwo\nthree"}}
	e.mu.Unlock()

	cases := []struct {
		name   string
		target types.SummaryTarget
		want   string
	}{
		{"unknown file", types.SummaryTarget{Path: "nope.go"}, "not a file in this review"},
		{"unknown artifact", types.SummaryTarget{Artifact: "missing"}, "not in this review"},
		{"artifact range past its end", types.SummaryTarget{Artifact: "plan", LineStart: 99}, "past its end"},
		{"known file", types.SummaryTarget{Path: "hello.go"}, ""},
		{"known artifact", types.SummaryTarget{Artifact: "plan", LineStart: 2}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.checkSummaryTargets(e.current, []types.SummaryItem{
				{ID: "it", Text: "something", Targets: []types.SummaryTarget{tc.target}},
			})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("reported %v, want nothing", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("reported %v, want one mentioning %q", got, tc.want)
			}
		})
	}
}

// A range that lands outside every hunk is the case ott hit: right file, stale
// line numbers, and no sign anything was wrong.
func TestRangeOutsideEveryHunkIsReported(t *testing.T) {
	hunks := []types.DiffHunk{{NewStart: 10, NewCount: 5}, {NewStart: 90, NewCount: 3}}
	for _, tc := range []struct {
		name  string
		start int
		end   int
		hit   bool
	}{
		{"inside the first hunk", 11, 12, true},
		{"exactly the last line of a hunk", 14, 14, true},
		{"spanning both hunks", 5, 200, true},
		{"between the hunks", 40, 50, false},
		{"past the end", 500, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rangeHitsAHunk(types.SummaryTarget{Path: "a", LineStart: tc.start, LineEnd: tc.end}, hunks)
			if got != tc.hit {
				t.Errorf("hit = %v, want %v", got, tc.hit)
			}
		})
	}
}

// Reported, never refused: an agent that summarises before its last add_files
// would otherwise have a correct summary rejected.
func TestUnmatchedTargetsStillStoreTheSummary(t *testing.T) {
	e, _ := summaryEngine(t)
	r := e.handleSetReviewSummary(&protocol.SetReviewSummaryMsg{
		Type: protocol.TypeSetReviewSummary,
		Items: []protocol.SummaryItemEntry{
			{ID: "a", Text: "a fix", Targets: []protocol.SummaryTargetEntry{{Path: "nowhere.go"}}},
		},
	})
	if !r.Success || r.Count != 1 {
		t.Fatalf("response = %+v, want the summary stored anyway", r)
	}
	if len(r.Unmatched) != 1 {
		t.Fatalf("unmatched = %v, want the bad target named", r.Unmatched)
	}
	if !strings.Contains(r.Message, "matched nothing") {
		t.Errorf("message = %q, want it to mention the unmatched target", r.Message)
	}
}
