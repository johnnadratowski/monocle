package types

import (
	"encoding/json"
	"time"
)

// Config is the global config with a project's .monocle/config.json over it,
// except for the fields that name something to run (core.GlobalOnlyKeys: the
// editor and viewers, every hook command, the related editor's commands and
// arguments), which only the global config sets. A new field goes in
// core.GlobalOnlyKeys or core.ProjectKeys; a test keeps every field in one.
type Config struct {
	IgnorePatterns []string          `json:"ignore_patterns"`
	Keybindings    map[string]string `json:"keybindings"`
	DiffStyle      string            `json:"diff_style"`
	SidebarStyle   string            `json:"sidebar_style"`
	Theme          string            `json:"theme"`
	Layout         string            `json:"layout"`
	Wrap           bool              `json:"wrap"`
	TabSize        int               `json:"tab_size"`
	ContextLines   int               `json:"context_lines"`
	FullFileDiff   bool              `json:"full_file_diff"`  // show whole file (with diff coloring) instead of compact hunks
	Editor         string            `json:"editor"`          // external editor command; overrides $VISUAL/$EDITOR when set
	MarkdownViewer string            `json:"markdown_viewer"` // external rendered-markdown viewer for artifacts / .md files (default: glow)
	MediaViewer    string            `json:"media_viewer"`    // external viewer for media artifacts / files (default: Google Chrome)
	// WalkthroughOnStop is a shell command run (via sh -c, in the repo root,
	// fire-and-forget) on every guided-tour stop the reviewer settles on, with
	// MONOCLE_STOP_ID, MONOCLE_REPO_ROOT and MONOCLE_STOP_JSON in its
	// environment. It is how a stop's screenshots, recordings and window layout
	// get shown: Monocle itself knows nothing about windows. `:view N` runs it
	// too, adding MONOCLE_VIEW_INDEX and MONOCLE_VIEW_NAME for the one view
	// asked for. Empty runs nothing.
	WalkthroughOnStop string `json:"walkthrough_on_stop"`
	// WalkthroughNavigate is a shell command that takes over when ctrl+h/j/k/l
	// reaches the edge of Monocle's panes, in place of tmux select-pane. It runs
	// via sh -c with MONOCLE_NAV_DIR (left, down, up or right) and TMUX_PANE in
	// its environment, and moves focus itself: a tour's windows sit beside
	// Monocle but outside tmux. If it fails, the key goes to tmux as before.
	// Empty hands every edge to tmux.
	WalkthroughNavigate string `json:"walkthrough_navigate"`
	// WalkthroughAsk is a shell command that hands the agent a reference to the
	// lines the reviewer sent with the send key: the selection and the tagged
	// lines, else the cursor's line. It runs via sh -c in the repo root with
	// MONOCLE_ASK_JSON ({"tour", "stop", "repo", "refs": [{"path", "start",
	// "end", "side"}]}) and MONOCLE_REPO_ROOT in its environment, puts the
	// reference in the agent's prompt and moves focus there. Empty sends
	// nothing.
	WalkthroughAsk string `json:"walkthrough_ask"`
	// WalkthroughResolve is a shell command that says what the lines the
	// reviewer points at with the open-references key reference: the SQL file
	// a query is loaded from, the file a function lives in. It runs via sh -c
	// in the repo root with MONOCLE_RESOLVE_JSON (the shape of
	// MONOCLE_ASK_JSON) and MONOCLE_REPO_ROOT in its environment, and prints a
	// JSON array of {"path", "line"}, which Monocle opens in the related-files
	// pane. Empty opens nothing.
	WalkthroughResolve string `json:"walkthrough_resolve"`
	// CursorCommand is a shell command run when the diff cursor comes to rest
	// on a new file:line — about 200 ms after the last move, never twice in a
	// row for the same place, never while a modal is open — so something
	// outside Monocle can follow what the reviewer is looking at. It runs via
	// sh -c in the repo root with MONOCLE_FILE (repo-relative), MONOCLE_LINE
	// (the new-file line, 0 when the row has none) and MONOCLE_REPO_ROOT,
	// fire-and-forget, 2 s timeout, output discarded. Empty runs nothing.
	CursorCommand string `json:"cursor_command"`
	// CommentsCommand is a shell command run when the reviewer turns source-code
	// comment lines on or off with the comments key (m), so a walkthrough can
	// share the state with the windows beside Monocle (John 2026-10-27: "a global
	// thing in tour, where it hides for both and uses the same key from both
	// windows"). It runs via sh -c in the repo root with MONOCLE_COMMENTS ("on":
	// shown, "off": hidden) and MONOCLE_REPO_ROOT, fire-and-forget, 2 s timeout,
	// output discarded; never for a state pushed in (`monocle review comments`).
	// Set, m toggles shown and hidden (M still cycles shown, dimmed, hidden, and
	// is not shared); empty, m cycles the three as before.
	CommentsCommand string `json:"comments_command"`
	// HighlightColor is the background of the range an agent highlights
	// (highlight_range), as a colour lipgloss understands ("#2d3b6b", "4").
	// Empty uses the theme's.
	HighlightColor string `json:"highlight_color"`
	// RelatedEditorArgs are extra arguments appended to the editor command of
	// the related-files pane (what ctrl+], ctrl+g in a tour and open-editor
	// open) — `["-S", "/path/setup.vim"]`, say — so
	// whatever runs the tour can set the editor up. {owner} in them becomes
	// Monocle's own tmux pane id. With vim and nvim they are
	// left out, with a status-bar note, when they would take +cmd, -c and -S
	// past vim's limit of ten.
	RelatedEditorArgs []string `json:"related_editor_args"`
	// RelatedEditorAdd is a shell command that adds one file to the editor
	// already running in the related-files pane, so files the reviewer closed
	// there stay closed. With it set and the pane alive, a file opened from
	// Monocle (ctrl+], ctrl+g in a tour, open-editor) is added by
	// running it once per file, instead of respawning the pane. {file} is the
	// file's absolute path, {line} its line (0 for none), {owner} Monocle's
	// tmux pane id; each is substituted shell-quoted. Empty respawns.
	RelatedEditorAdd string `json:"related_editor_add"`
	// RelatedEditorPreview is a shell command that shows one file in a passing
	// preview in the editor already running in the related-files pane — a
	// popup it closes itself once the reviewer moves on — rather than adding
	// it. p (or f16) runs it for the first file walkthrough_resolve
	// finds. Placeholders as related_editor_add's. Empty, or no live pane:
	// the key opens the files as ctrl+] does.
	RelatedEditorPreview string `json:"related_editor_preview"`
	// RelatedOpen is a shell command that opens one file somewhere other than
	// the related-files pane — a popout window, say. With it set no pane is
	// split beside Monocle: what walkthrough_resolve finds (ctrl+] and p) runs
	// it for the first file found, and ctrl+g in a tour and open-editor run it
	// for theirs. Placeholders as related_editor_add's.
	// Empty keeps the related-files pane.
	RelatedOpen string `json:"related_open"`
	// EditorMode controls how Ctrl+g / Ctrl+o open a file: "terminal" (take over
	// Monocle's screen, the default), "tmux_vertical" (side-by-side split),
	// "tmux_horizontal" (stacked split), or "tmux_window" (new tmux window/tab).
	// tmux modes require running inside tmux; otherwise they fall back to
	// "terminal". Ctrl+Shift+g / Ctrl+Shift+o always use "terminal".
	EditorMode string `json:"editor_mode"`
	// EditorFocus controls whether a new tmux split/window takes focus. Defaults
	// to true (focus the editor); set false to keep focus on Monocle.
	EditorFocus          *bool              `json:"editor_focus"`
	ReviewFormat         ReviewFormatConfig `json:"review_format"`
	AutoFocusMode        bool               `json:"auto_focus_mode"`
	Mouse                *bool              `json:"mouse"`
	MinDiffWidth         int                `json:"min_diff_width"`
	CommentExpand        *bool              `json:"comment_expand"`
	CommentExpandDelay   int                `json:"comment_expand_delay"`
	MarkReviewedOnSubmit string             `json:"mark_reviewed_on_submit"` // "all" (default), "commented", "manual"
	ReviewTracking       bool               `json:"review_tracking"`         // enable reviewed state, snapshots, change detection (default: true)
	// IdleTimeout controls how long `monocle serve` stays running after the
	// last client disconnects (minus a 60s grace window). Serialised as a
	// Go duration string (e.g. "30m", "1h"); an empty/zero value uses the
	// default 30 min, and a negative value disables idle shutdown.
	IdleTimeout Duration `json:"idle_timeout,omitempty"`
}

// Duration is a time.Duration alias with JSON support so config files can
// hold human-readable values like "30m" or "1h" instead of nanosecond ints.
type Duration time.Duration

// UnmarshalJSON accepts either a quoted duration string ("30m") or a raw
// integer-nanosecond value so hand-written JSON and marshalled output both
// round-trip.
func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*d = 0
			return nil
		}
		dur, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = Duration(dur)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*d = Duration(n)
	return nil
}

// MarshalJSON writes the duration as a human-readable string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

type ReviewFormatConfig struct {
	IncludeSnippets bool `json:"include_snippets"`
	MaxSnippetLines int  `json:"max_snippet_lines"`
	IncludeSummary  bool `json:"include_summary"`
}
