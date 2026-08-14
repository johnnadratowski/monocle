package tui

import "sort"

// KeyMap defines all configurable keybindings. Each field holds one or more
// key strings that trigger that action. Users override individual actions
// via Config.Keybindings (map action name → key string).
type KeyMap struct {
	// Navigation
	Up       []string
	Down     []string
	Top      []string
	Bottom   []string
	HalfUp   []string
	HalfDown []string
	PrevFile []string
	NextFile []string
	Select   []string

	// Pane focus
	FocusSwap     []string
	FocusPaneN    map[string]int // key → pane number (1=sidebar, 2=diff)
	ToggleSidebar []string

	// Diff view
	ScrollDown      []string
	ScrollUp        []string
	ScrollLeft      []string
	ScrollRight     []string
	ScrollHome      []string
	ScrollFirstChar []string
	ScrollEnd       []string
	Wrap            []string
	ToggleDiff      []string
	ToggleFullDiff  []string
	ToggleOverlays  []string // hide/show inline comments + annotations
	HideComments    []string // dim/show source-code comment-only lines
	OpenDocRef      []string // open/cycle the cursor annotation's doc links in the doc pane
	YankLine        []string

	// Diff search
	SearchBackward []string // forward search uses FilterReviewed's `/` when the diff is focused
	SearchNext     []string
	SearchPrev     []string

	// Jump between review marks (inline comments + agent annotations) in the diff
	PrevMark []string
	NextMark []string

	// Jump list: return to where you were, vim's ctrl+o / ctrl+i
	JumpBack    []string
	JumpForward []string

	// Code-structure navigation in the diff (vim %, [{ and [[)
	BlockMatch []string // block start <-> end
	BlockUp    []string // out one level
	BlockTop   []string // out to the outermost enclosing block

	// Sidebar
	TreeMode       []string
	CollapseAll    []string
	ExpandAll      []string
	PrevSection    []string
	NextSection    []string
	FilterReviewed []string

	// Review actions
	Comment         []string
	FileComment     []string
	Suggest         []string
	Question        []string // add a comment already typed as a question
	Visual          []string
	Reviewed        []string
	Submit          []string
	Pause           []string
	ClearReview     []string
	DismissArtifact []string
	ToggleFocusMode []string

	// General
	OpenInEditor         []string
	OpenInEditorTakeover []string // always take over the terminal (ignore editor_mode)
	OpenInMarkdownViewer []string // open the current artifact / .md file in a rendered markdown viewer
	OpenTerminal         []string // open a terminal at the current file's directory
	OpenTerminalTakeover []string // same, always taking over the screen (ignore editor_mode)
	ShellCommand         []string // prompt for a shell command to run on the current file
	BaseRef              []string
	ArtifactVersions     []string
	CycleLayout          []string
	Refresh              []string
	Help                 []string
	Quit                 []string
	CommandMode          []string

	// Wizard (register TUI)
	WizardAdvance []string
	WizardBack    []string
	WizardToggle  []string
}

// DefaultKeyMap returns the built-in default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:       []string{"k", "up"},
		Down:     []string{"j", "down"},
		Top:      []string{"g"},
		Bottom:   []string{"G"},
		HalfUp:   []string{"ctrl+u"},
		HalfDown: []string{"ctrl+d"},
		PrevFile: []string{"["},
		NextFile: []string{"]"},
		Select:   []string{"enter"},

		FocusSwap:     []string{"tab"},
		FocusPaneN:    map[string]int{"1": 1, "2": 2},
		ToggleSidebar: []string{";"},

		ScrollDown:      []string{"J"},
		ScrollUp:        []string{"K"},
		ScrollLeft:      nil, // lowercase `h` still scrolls the focused diff; `H` is now Help
		ScrollRight:     []string{"L"},
		ScrollHome:      []string{"0"},
		ScrollFirstChar: []string{"^"},
		ScrollEnd:       []string{"$"},
		Wrap:            []string{"w"},
		ToggleDiff:      []string{"t"},
		ToggleFullDiff:  []string{"a"},
		ToggleOverlays:  []string{"O"},
		HideComments:    []string{"#"},
		OpenDocRef:      []string{"o"},
		YankLine:        []string{"y"},

		TreeMode:       []string{"f"},
		CollapseAll:    []string{"z"},
		ExpandAll:      []string{"e"},
		SearchBackward: []string{"?"},
		SearchNext:     []string{"n"},
		SearchPrev:     []string{"N"},

		PrevMark: []string{"<"},
		NextMark: []string{">"},

		BlockMatch: []string{"%"},
		BlockUp:    []string{"("},
		BlockTop:   []string{")"},

		PrevSection:    []string{"{"},
		NextSection:    []string{"}"},
		FilterReviewed: []string{"/"},

		Comment:         []string{"c"},
		FileComment:     []string{"C"},
		Suggest:         []string{"s"},
		Question:        []string{"Q"},
		Visual:          []string{"v"},
		Reviewed:        []string{"r"},
		Submit:          []string{"S"},
		Pause:           []string{"P"},
		ClearReview:     []string{"D"},
		DismissArtifact: []string{"x"},
		ToggleFocusMode: []string{"F"},

		OpenInEditor:         []string{"ctrl+g"},
		OpenInEditorTakeover: []string{"ctrl+shift+g"},

		JumpBack:             []string{"ctrl+o"},
		JumpForward:          []string{"ctrl+i"},
		OpenInMarkdownViewer: []string{"ctrl+p"},
		OpenTerminal:         []string{"ctrl+t"},
		OpenTerminalTakeover: []string{"ctrl+shift+t"},
		ShellCommand:         []string{"!"},
		BaseRef:              []string{"b"},
		ArtifactVersions:     []string{"B"},
		CycleLayout:          []string{"T"},
		Refresh:              []string{"R"},
		Help:                 []string{"H"},
		Quit:                 []string{"q"},
		CommandMode:          []string{":"},

		WizardAdvance: []string{"enter"},
		WizardBack:    []string{"shift+tab", "backspace"},
		WizardToggle:  []string{" ", "space"},
	}
}

// keyActions is the canonical registry of configurable actions: the config
// name a user writes in `keybindings`, and the KeyMap field it sets. It is the
// single place an action is registered — ApplyOverrides applies through it,
// ActionNames derives from it, and the docs are checked against it by
// TestDocsListEveryKeyAction. A separate hand-maintained list of names drifted
// out of sync with the code and the docs, which is what this replaces.
var keyActions = map[string]func(*KeyMap) *[]string{
	"up":                      func(km *KeyMap) *[]string { return &km.Up },
	"down":                    func(km *KeyMap) *[]string { return &km.Down },
	"top":                     func(km *KeyMap) *[]string { return &km.Top },
	"bottom":                  func(km *KeyMap) *[]string { return &km.Bottom },
	"half_up":                 func(km *KeyMap) *[]string { return &km.HalfUp },
	"half_down":               func(km *KeyMap) *[]string { return &km.HalfDown },
	"prev_file":               func(km *KeyMap) *[]string { return &km.PrevFile },
	"next_file":               func(km *KeyMap) *[]string { return &km.NextFile },
	"select":                  func(km *KeyMap) *[]string { return &km.Select },
	"focus_swap":              func(km *KeyMap) *[]string { return &km.FocusSwap },
	"toggle_sidebar":          func(km *KeyMap) *[]string { return &km.ToggleSidebar },
	"scroll_down":             func(km *KeyMap) *[]string { return &km.ScrollDown },
	"scroll_up":               func(km *KeyMap) *[]string { return &km.ScrollUp },
	"scroll_left":             func(km *KeyMap) *[]string { return &km.ScrollLeft },
	"scroll_right":            func(km *KeyMap) *[]string { return &km.ScrollRight },
	"scroll_home":             func(km *KeyMap) *[]string { return &km.ScrollHome },
	"scroll_first_char":       func(km *KeyMap) *[]string { return &km.ScrollFirstChar },
	"scroll_end":              func(km *KeyMap) *[]string { return &km.ScrollEnd },
	"wrap":                    func(km *KeyMap) *[]string { return &km.Wrap },
	"toggle_diff":             func(km *KeyMap) *[]string { return &km.ToggleDiff },
	"toggle_full_diff":        func(km *KeyMap) *[]string { return &km.ToggleFullDiff },
	"toggle_overlays":         func(km *KeyMap) *[]string { return &km.ToggleOverlays },
	"hide_comments":           func(km *KeyMap) *[]string { return &km.HideComments },
	"open_doc_ref":            func(km *KeyMap) *[]string { return &km.OpenDocRef },
	"yank_line":               func(km *KeyMap) *[]string { return &km.YankLine },
	"search_backward":         func(km *KeyMap) *[]string { return &km.SearchBackward },
	"search_next":             func(km *KeyMap) *[]string { return &km.SearchNext },
	"search_prev":             func(km *KeyMap) *[]string { return &km.SearchPrev },
	"prev_mark":               func(km *KeyMap) *[]string { return &km.PrevMark },
	"next_mark":               func(km *KeyMap) *[]string { return &km.NextMark },
	"block_match":             func(km *KeyMap) *[]string { return &km.BlockMatch },
	"block_up":                func(km *KeyMap) *[]string { return &km.BlockUp },
	"block_top":               func(km *KeyMap) *[]string { return &km.BlockTop },
	"tree_mode":               func(km *KeyMap) *[]string { return &km.TreeMode },
	"collapse_all":            func(km *KeyMap) *[]string { return &km.CollapseAll },
	"expand_all":              func(km *KeyMap) *[]string { return &km.ExpandAll },
	"prev_section":            func(km *KeyMap) *[]string { return &km.PrevSection },
	"next_section":            func(km *KeyMap) *[]string { return &km.NextSection },
	"filter_reviewed":         func(km *KeyMap) *[]string { return &km.FilterReviewed },
	"comment":                 func(km *KeyMap) *[]string { return &km.Comment },
	"file_comment":            func(km *KeyMap) *[]string { return &km.FileComment },
	"suggest":                 func(km *KeyMap) *[]string { return &km.Suggest },
	"question":                func(km *KeyMap) *[]string { return &km.Question },
	"visual":                  func(km *KeyMap) *[]string { return &km.Visual },
	"reviewed":                func(km *KeyMap) *[]string { return &km.Reviewed },
	"submit":                  func(km *KeyMap) *[]string { return &km.Submit },
	"pause":                   func(km *KeyMap) *[]string { return &km.Pause },
	"clear_review":            func(km *KeyMap) *[]string { return &km.ClearReview },
	"dismiss_artifact":        func(km *KeyMap) *[]string { return &km.DismissArtifact },
	"toggle_focus_mode":       func(km *KeyMap) *[]string { return &km.ToggleFocusMode },
	"open_in_editor":          func(km *KeyMap) *[]string { return &km.OpenInEditor },
	"open_in_editor_takeover": func(km *KeyMap) *[]string { return &km.OpenInEditorTakeover },
	"jump_back":               func(km *KeyMap) *[]string { return &km.JumpBack },
	"jump_forward":            func(km *KeyMap) *[]string { return &km.JumpForward },
	"open_in_markdown_viewer": func(km *KeyMap) *[]string { return &km.OpenInMarkdownViewer },
	"open_terminal":           func(km *KeyMap) *[]string { return &km.OpenTerminal },
	"open_terminal_takeover":  func(km *KeyMap) *[]string { return &km.OpenTerminalTakeover },
	"shell_command":           func(km *KeyMap) *[]string { return &km.ShellCommand },
	"base_ref":                func(km *KeyMap) *[]string { return &km.BaseRef },
	"artifact_versions":       func(km *KeyMap) *[]string { return &km.ArtifactVersions },
	"cycle_layout":            func(km *KeyMap) *[]string { return &km.CycleLayout },
	"refresh":                 func(km *KeyMap) *[]string { return &km.Refresh },
	"help":                    func(km *KeyMap) *[]string { return &km.Help },
	"quit":                    func(km *KeyMap) *[]string { return &km.Quit },
	"command_mode":            func(km *KeyMap) *[]string { return &km.CommandMode },
	"wizard_advance":          func(km *KeyMap) *[]string { return &km.WizardAdvance },
	"wizard_back":             func(km *KeyMap) *[]string { return &km.WizardBack },
	"wizard_toggle":           func(km *KeyMap) *[]string { return &km.WizardToggle },
}

// ActionNames returns every configurable action name, sorted.
func ActionNames() []string {
	names := make([]string, 0, len(keyActions))
	for name := range keyActions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ApplyOverrides merges user-configured keybinding overrides into the keymap.
// Each key in overrides is an action name (e.g. "quit"), value is the key
// string. Unknown action names are ignored.
func (km KeyMap) ApplyOverrides(overrides map[string]string) KeyMap {
	for action, key := range overrides {
		if field, ok := keyActions[action]; ok {
			*field(&km) = []string{key}
		}
	}
	return km
}

// Matches returns true if the key string matches any of the given bindings.
func Matches(key string, bindings []string) bool {
	for _, b := range bindings {
		if key == b {
			return true
		}
	}
	return false
}

// PrimaryLabel returns just the first key of a binding, for surfaces where
// listing every alias (j AND down, k AND up) is noise rather than information.
func PrimaryLabel(bindings []string) string {
	if len(bindings) == 0 {
		return ""
	}
	return bindings[0]
}

// Label returns the display label for a keybinding (first key, or joined with /).
func Label(bindings []string) string {
	if len(bindings) == 0 {
		return ""
	}
	if len(bindings) == 1 {
		return bindings[0]
	}
	return bindings[0] + "/" + bindings[1]
}
