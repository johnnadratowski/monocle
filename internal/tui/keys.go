package tui

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
	FocusSwap      []string
	FocusPaneN     map[string]int // key → pane number (1=sidebar, 2=diff)
	ToggleSidebar  []string

	// Diff view
	ScrollDown  []string
	ScrollUp    []string
	ScrollLeft  []string
	ScrollRight []string
	ScrollHome      []string
	ScrollFirstChar []string
	ScrollEnd       []string
	Wrap        []string
	ToggleDiff  []string

	// Sidebar
	TreeMode       []string
	CollapseAll    []string
	ExpandAll      []string
	PrevSection    []string
	NextSection    []string
	FilterReviewed []string

	// Review actions
	Comment      []string
	FileComment  []string
	Suggest      []string
	Visual       []string
	Reviewed     []string
	Submit       []string
	Pause        []string
	ClearReview  []string
	ToggleFocusMode []string

	// General
	OpenInEditor     []string
	BaseRef          []string
	ArtifactVersions []string
	CycleLayout      []string
	Refresh      []string
	Help         []string
	Quit         []string
	CommandMode  []string

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
		ToggleSidebar: []string{"\\"},

		ScrollDown:  []string{"J"},
		ScrollUp:    []string{"K"},
		ScrollLeft:  []string{"H"},
		ScrollRight: []string{"L"},
		ScrollHome:      []string{"0"},
		ScrollFirstChar: []string{"^"},
		ScrollEnd:       []string{"$"},
		Wrap:        []string{"w"},
		ToggleDiff:  []string{"t"},

		TreeMode:       []string{"f"},
		CollapseAll:    []string{"z"},
		ExpandAll:      []string{"e"},
		PrevSection:    []string{"{"},
		NextSection:    []string{"}"},
		FilterReviewed: []string{"/"},

		Comment:         []string{"c"},
		FileComment:     []string{"C"},
		Suggest:         []string{"s"},
		Visual:          []string{"v"},
		Reviewed:        []string{"r"},
		Submit:          []string{"S"},
		Pause:           []string{"P"},
		ClearReview:     []string{"D"},
		ToggleFocusMode: []string{"F"},

		OpenInEditor: []string{"ctrl+g"},
		BaseRef:          []string{"b"},
		ArtifactVersions: []string{"B"},
		CycleLayout:      []string{"T"},
		Refresh:     []string{"R"},
		Help:        []string{"?"},
		Quit:        []string{"q"},
		CommandMode: []string{":"},

		WizardAdvance: []string{"enter"},
		WizardBack:    []string{"shift+tab", "backspace"},
		WizardToggle:  []string{" ", "space"},
	}
}

// actionBindings maps config action names to pointers into the KeyMap.
// This is used by ApplyOverrides to find which field to update.
var actionNames = []string{
	"up", "down", "top", "bottom", "half_up", "half_down",
	"prev_file", "next_file", "select",
	"focus_swap", "toggle_sidebar",
	"scroll_down", "scroll_up", "scroll_left", "scroll_right", "scroll_home", "scroll_first_char", "scroll_end",
	"wrap", "toggle_diff",
	"tree_mode", "collapse_all", "expand_all", "prev_section", "next_section", "filter_reviewed",
	"comment", "file_comment", "suggest", "visual", "reviewed",
	"submit", "pause", "clear_review", "dismiss_outdated", "toggle_focus_mode",
	"open_in_editor", "base_ref", "artifact_versions", "cycle_layout", "refresh", "help", "quit", "command_mode",
	"wizard_advance", "wizard_back", "wizard_toggle",
}

// ApplyOverrides merges user-configured keybinding overrides into the keymap.
// Each key in overrides is an action name (e.g. "quit"), value is the key string.
func (km KeyMap) ApplyOverrides(overrides map[string]string) KeyMap {
	for action, key := range overrides {
		switch action {
		case "up":
			km.Up = []string{key}
		case "down":
			km.Down = []string{key}
		case "top":
			km.Top = []string{key}
		case "bottom":
			km.Bottom = []string{key}
		case "half_up":
			km.HalfUp = []string{key}
		case "half_down":
			km.HalfDown = []string{key}
		case "prev_file":
			km.PrevFile = []string{key}
		case "next_file":
			km.NextFile = []string{key}
		case "select":
			km.Select = []string{key}
		case "focus_swap":
			km.FocusSwap = []string{key}
		case "toggle_sidebar":
			km.ToggleSidebar = []string{key}
		case "scroll_down":
			km.ScrollDown = []string{key}
		case "scroll_up":
			km.ScrollUp = []string{key}
		case "scroll_left":
			km.ScrollLeft = []string{key}
		case "scroll_right":
			km.ScrollRight = []string{key}
		case "scroll_home":
			km.ScrollHome = []string{key}
		case "scroll_first_char":
			km.ScrollFirstChar = []string{key}
		case "scroll_end":
			km.ScrollEnd = []string{key}
		case "wrap":
			km.Wrap = []string{key}
		case "toggle_diff":
			km.ToggleDiff = []string{key}
		case "tree_mode":
			km.TreeMode = []string{key}
		case "collapse_all":
			km.CollapseAll = []string{key}
		case "expand_all":
			km.ExpandAll = []string{key}
		case "prev_section":
			km.PrevSection = []string{key}
		case "next_section":
			km.NextSection = []string{key}
		case "filter_reviewed":
			km.FilterReviewed = []string{key}
		case "comment":
			km.Comment = []string{key}
		case "file_comment":
			km.FileComment = []string{key}
		case "suggest":
			km.Suggest = []string{key}
		case "visual":
			km.Visual = []string{key}
		case "reviewed":
			km.Reviewed = []string{key}
		case "submit":
			km.Submit = []string{key}
		case "pause":
			km.Pause = []string{key}
		case "clear_review":
			km.ClearReview = []string{key}
		case "toggle_focus_mode":
			km.ToggleFocusMode = []string{key}
		case "open_in_editor":
			km.OpenInEditor = []string{key}
		case "base_ref":
			km.BaseRef = []string{key}
		case "artifact_versions":
			km.ArtifactVersions = []string{key}
		case "cycle_layout":
			km.CycleLayout = []string{key}
		case "refresh":
			km.Refresh = []string{key}
		case "help":
			km.Help = []string{key}
		case "quit":
			km.Quit = []string{key}
		case "command_mode":
			km.CommandMode = []string{key}
		case "wizard_advance":
			km.WizardAdvance = []string{key}
		case "wizard_back":
			km.WizardBack = []string{key}
		case "wizard_toggle":
			km.WizardToggle = []string{key}
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
