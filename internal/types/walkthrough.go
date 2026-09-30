package types

import (
	"fmt"
	"strconv"
	"strings"
)

// A review is read in the order the files happen to sort, but the agent that
// wrote it knows the order it should be read in, and what to say at each point.
// A Walkthrough is that: an ordered list of stops, each a place in the change
// plus what the agent wants to say about it. The reviewer steps through it with
// a key while the agent answers questions by stop id ("1.2 — why is x hoisted?").
type Walkthrough struct {
	Title string            `json:"title,omitempty"`
	Stops []WalkthroughStop `json:"stops"`
}

// WalkthroughStop is one place the tour stops.
type WalkthroughStop struct {
	// ID is the label the reviewer sees and types (`:stop 1.2`), so it is
	// unique within a walkthrough. Free text: "1.2", "setup", "3b" all work.
	ID    string `json:"id"`
	Title string `json:"title,omitempty"` // one line
	// File is repo-relative. Empty is a stop with no code anchor — a stop that
	// exists to show a video or a table, say.
	File      string `json:"file,omitempty"`
	LineStart int    `json:"line_start,omitempty"` // new-file line; 0 = the file as a whole
	LineEnd   int    `json:"line_end,omitempty"`
	Note      string `json:"note,omitempty"` // markdown, shown in the doc pane
	// Related are repo files to open, at their lines, in the editor split —
	// typically where a change starts when the stop shows where it lands.
	Related []DocRef   `json:"related,omitempty"`
	Views   []StopView `json:"views,omitempty"`
	// Layout is an opaque scene name, passed through to the on-stop command.
	// Monocle itself never interprets it.
	Layout string `json:"layout,omitempty"`
}

// StopView is something to show alongside a stop that is not code: a
// screenshot, a recording, a rendered document, a page. Monocle does not open
// views itself on a stop — it hands them to the on-stop command, and opens one
// on request (`:view`).
type StopView struct {
	Kind   string `json:"kind"`   // one of the StopView* kinds
	Target string `json:"target"` // path (absolute or repo-relative), URL, or artifact id
	Label  string `json:"label,omitempty"`
}

// The view kinds a stop can carry.
const (
	StopViewImage    = "image"
	StopViewVideo    = "video"
	StopViewMarkdown = "markdown"
	StopViewURL      = "url"
	StopViewArtifact = "artifact"
)

// IsPathView reports whether a view's target names a file on disk, and so can
// be resolved against the repo root.
func (v StopView) IsPathView() bool {
	switch v.Kind {
	case StopViewImage, StopViewVideo, StopViewMarkdown:
		return true
	}
	return false
}

// Label for a stop as the reviewer reads it in a header: "1.2 · Title".
func (s WalkthroughStop) Heading() string {
	title := s.Title
	if title == "" {
		title = s.File
	}
	if title == "" {
		return s.ID
	}
	return s.ID + " · " + title
}

// StopIndex returns the position of the stop with the given id, or -1.
func (w *Walkthrough) StopIndex(id string) int {
	if w == nil {
		return -1
	}
	id = strings.TrimSpace(id)
	for i, s := range w.Stops {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// Empty reports whether there is a tour to take.
func (w *Walkthrough) Empty() bool { return w == nil || len(w.Stops) == 0 }

// NormalizeWalkthrough trims what the agent sent into something the rest of
// the system can rely on: every stop has a unique id, ranges run forwards,
// related files are file refs, and views have a kind. It refuses rather than
// repairs a duplicate id, because the id is what the reviewer types and the
// agent answers to — two stops called "1.2" would make both ambiguous.
func NormalizeWalkthrough(w Walkthrough) (Walkthrough, error) {
	out := Walkthrough{Title: strings.TrimSpace(w.Title), Stops: make([]WalkthroughStop, 0, len(w.Stops))}
	seen := make(map[string]bool, len(w.Stops))
	for i, s := range w.Stops {
		s.ID = strings.TrimSpace(s.ID)
		if s.ID == "" {
			// A stop the agent forgot to number is still a stop; its position
			// is the obvious label.
			s.ID = strconv.Itoa(i + 1)
		}
		if seen[s.ID] {
			return Walkthrough{}, fmt.Errorf("duplicate stop id %q: ids are what the reviewer types, so each must be unique", s.ID)
		}
		seen[s.ID] = true
		s.Title = strings.TrimSpace(strings.SplitN(s.Title, "\n", 2)[0])
		s.File = strings.TrimSpace(s.File)
		if s.LineStart < 0 {
			s.LineStart = 0
		}
		if s.LineEnd < s.LineStart {
			s.LineEnd = s.LineStart
		}
		s.Layout = strings.TrimSpace(s.Layout)

		related := make([]DocRef, 0, len(s.Related))
		for _, r := range s.Related {
			r.Doc = strings.TrimSpace(r.Doc)
			if r.Doc == "" {
				continue
			}
			// Related entries are opened in an editor, so they can only be files.
			r.Kind = DocRefFile
			related = append(related, r)
		}
		s.Related = related

		views := make([]StopView, 0, len(s.Views))
		for _, v := range s.Views {
			v.Kind = strings.ToLower(strings.TrimSpace(v.Kind))
			v.Target = strings.TrimSpace(v.Target)
			if v.Target == "" {
				continue
			}
			if v.Kind == "" {
				v.Kind = GuessStopViewKind(v.Target)
			}
			views = append(views, v)
		}
		s.Views = views
		out.Stops = append(out.Stops, s)
	}
	return out, nil
}

// GuessStopViewKind infers a view's kind from its target when the agent left
// it out: a URL is a url, a media file is its media category, a .md file is
// markdown. Anything else is a url, which is what a viewer is most likely to
// open sensibly.
func GuessStopViewKind(target string) string {
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return StopViewURL
	}
	if cat, _, ok := MediaInfo(target); ok && (cat == "image" || cat == "video") {
		return cat
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return StopViewMarkdown
	}
	return StopViewURL
}
