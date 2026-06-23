package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds all styles for the TUI.
type Theme struct {
	// Layout
	SidebarBorder        lipgloss.Style
	SidebarBorderFocused lipgloss.Style
	MainPane             lipgloss.Style
	MainPaneFocused      lipgloss.Style

	// Diff colors
	Added          lipgloss.Style
	Removed        lipgloss.Style
	Context        lipgloss.Style
	HunkHeader     lipgloss.Style
	LineNumber     lipgloss.Style

	// Diff backgrounds (true color for syntax highlighting overlay)
	AddedBg         color.Color
	RemovedBg       color.Color
	AddedChangeBg   color.Color
	RemovedChangeBg color.Color

	// SyntaxStyle is the chroma style name used for code syntax highlighting
	// (e.g. "monokai", "dracula", "nord", "github"). Empty falls back to monokai.
	SyntaxStyle string

	// Comment styles
	CommentBorder  lipgloss.Style
	CommentIssue   lipgloss.Style
	CommentSuggest lipgloss.Style
	CommentNote    lipgloss.Style
	CommentPraise  lipgloss.Style

	// Status
	StatusBar lipgloss.Style

	// Modal
	ModalOverlay   lipgloss.Style
	ModalBorder    lipgloss.Style

	// Markdown
	MarkdownH1         lipgloss.Style
	MarkdownH2         lipgloss.Style
	MarkdownH3         lipgloss.Style
	MarkdownBlockquote lipgloss.Style
	MarkdownCode       lipgloss.Style
	MarkdownCodeBlock  lipgloss.Style
	MarkdownRule       lipgloss.Style
	MarkdownBullet     lipgloss.Style
}

// DefaultTheme returns a theme using 16-color ANSI for maximum compatibility.
func DefaultTheme() Theme {
	return Theme{
		SidebarBorder:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")),
		SidebarBorderFocused: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("4")),
		MainPane:             lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")),
		MainPaneFocused:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("4")),

		Added:          lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Removed:        lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		Context:        lipgloss.NewStyle(),
		HunkHeader:     lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Faint(true),
		LineNumber:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),

		CommentBorder:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		CommentIssue:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		CommentSuggest: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		CommentNote:    lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		CommentPraise:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("7")),

		ModalOverlay:   lipgloss.NewStyle().Background(lipgloss.Color("0")),
		ModalBorder:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("4")).Padding(1, 2),

		MarkdownH1:         lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		MarkdownH2:         lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		MarkdownH3:         lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true),
		MarkdownBlockquote: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
		MarkdownCode:       lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		MarkdownCodeBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Faint(true),
		MarkdownRule:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		MarkdownBullet:     lipgloss.NewStyle().Foreground(lipgloss.Color("5")),

		AddedBg:         lipgloss.Color("#132a13"),
		RemovedBg:       lipgloss.Color("#2a1313"),
		AddedChangeBg:   lipgloss.Color("#1f4a1f"),
		RemovedChangeBg: lipgloss.Color("#4a1f1f"),

		SyntaxStyle: "monokai",
	}
}

// LightTheme returns a theme tuned for terminals with a light background.
func LightTheme() Theme {
	return Theme{
		SidebarBorder:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("7")),
		SidebarBorderFocused: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("4")),
		MainPane:             lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("7")),
		MainPaneFocused:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("4")),

		Added:      lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Removed:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		Context:    lipgloss.NewStyle(),
		HunkHeader: lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Faint(true),
		LineNumber: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),

		CommentBorder:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		CommentIssue:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		CommentSuggest: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		CommentNote:    lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		CommentPraise:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color("7")).Foreground(lipgloss.Color("0")),

		ModalOverlay: lipgloss.NewStyle().Background(lipgloss.Color("7")),
		ModalBorder:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("4")).Padding(1, 2),

		MarkdownH1:         lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		MarkdownH2:         lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		MarkdownH3:         lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true),
		MarkdownBlockquote: lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Italic(true),
		MarkdownCode:       lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		MarkdownCodeBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color("0")),
		MarkdownRule:       lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		MarkdownBullet:     lipgloss.NewStyle().Foreground(lipgloss.Color("5")),

		AddedBg:         lipgloss.Color("#d4f4d4"),
		RemovedBg:       lipgloss.Color("#f4d4d4"),
		AddedChangeBg:   lipgloss.Color("#a8e6a8"),
		RemovedChangeBg: lipgloss.Color("#e6a8a8"),

		SyntaxStyle: "github",
	}
}

// MolokaiTheme is a true-color theme based on the molokai vim colorscheme
// (a Monokai variant). Palette: bg #1B1D1E, fg #F8F8F2, green #A6E22E,
// pink #F92672, yellow #E6DB74, blue #66D9EF, purple #AE81FF, orange #FD971F.
func MolokaiTheme() Theme {
	const (
		fg      = "#F8F8F2"
		bg      = "#1B1D1E"
		panel   = "#232526"
		border  = "#465457"
		green   = "#A6E22E"
		pink    = "#F92672"
		yellow  = "#E6DB74"
		blue    = "#66D9EF"
		orange  = "#FD971F"
		comment = "#7E8E91"
		lineNr  = "#6E7B7C"
	)
	return Theme{
		SidebarBorder:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		SidebarBorderFocused: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(blue)),
		MainPane:             lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		MainPaneFocused:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(blue)),

		Added:      lipgloss.NewStyle().Foreground(lipgloss.Color(green)),
		Removed:    lipgloss.NewStyle().Foreground(lipgloss.Color(pink)),
		Context:    lipgloss.NewStyle().Foreground(lipgloss.Color(fg)),
		HunkHeader: lipgloss.NewStyle().Foreground(lipgloss.Color(blue)).Faint(true),
		LineNumber: lipgloss.NewStyle().Foreground(lipgloss.Color(lineNr)),

		CommentBorder:  lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		CommentIssue:   lipgloss.NewStyle().Foreground(lipgloss.Color(pink)).Bold(true),
		CommentSuggest: lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)).Bold(true),
		CommentNote:    lipgloss.NewStyle().Foreground(lipgloss.Color(blue)).Bold(true),
		CommentPraise:  lipgloss.NewStyle().Foreground(lipgloss.Color(green)).Bold(true),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color(panel)).Foreground(lipgloss.Color(fg)),

		ModalOverlay: lipgloss.NewStyle().Background(lipgloss.Color(bg)),
		ModalBorder:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(blue)).Padding(1, 2),

		MarkdownH1:         lipgloss.NewStyle().Foreground(lipgloss.Color(pink)).Bold(true),
		MarkdownH2:         lipgloss.NewStyle().Foreground(lipgloss.Color(orange)).Bold(true),
		MarkdownH3:         lipgloss.NewStyle().Foreground(lipgloss.Color(blue)).Bold(true),
		MarkdownBlockquote: lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)).Italic(true),
		MarkdownCode:       lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		MarkdownCodeBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Faint(true),
		MarkdownRule:       lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		MarkdownBullet:     lipgloss.NewStyle().Foreground(lipgloss.Color(pink)),

		AddedBg:         lipgloss.Color("#2B3D2B"),
		RemovedBg:       lipgloss.Color("#3C1F26"),
		AddedChangeBg:   lipgloss.Color("#3F5F2F"),
		RemovedChangeBg: lipgloss.Color("#5C2733"),

		SyntaxStyle: "monokai",
	}
}

// DraculaTheme is a true-color theme based on the Dracula palette.
func DraculaTheme() Theme {
	const (
		fg      = "#F8F8F2"
		bg      = "#282A36"
		panel   = "#44475A"
		border  = "#44475A"
		green   = "#50FA7B"
		pink    = "#FF79C6"
		red     = "#FF5555"
		yellow  = "#F1FA8C"
		cyan    = "#8BE9FD"
		purple  = "#BD93F9"
		comment = "#6272A4"
	)
	return Theme{
		SidebarBorder:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		SidebarBorderFocused: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(purple)),
		MainPane:             lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		MainPaneFocused:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(purple)),

		Added:      lipgloss.NewStyle().Foreground(lipgloss.Color(green)),
		Removed:    lipgloss.NewStyle().Foreground(lipgloss.Color(red)),
		Context:    lipgloss.NewStyle().Foreground(lipgloss.Color(fg)),
		HunkHeader: lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Faint(true),
		LineNumber: lipgloss.NewStyle().Foreground(lipgloss.Color(comment)),

		CommentBorder:  lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		CommentIssue:   lipgloss.NewStyle().Foreground(lipgloss.Color(red)).Bold(true),
		CommentSuggest: lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)).Bold(true),
		CommentNote:    lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Bold(true),
		CommentPraise:  lipgloss.NewStyle().Foreground(lipgloss.Color(green)).Bold(true),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color(panel)).Foreground(lipgloss.Color(fg)),

		ModalOverlay: lipgloss.NewStyle().Background(lipgloss.Color(bg)),
		ModalBorder:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(purple)).Padding(1, 2),

		MarkdownH1:         lipgloss.NewStyle().Foreground(lipgloss.Color(pink)).Bold(true),
		MarkdownH2:         lipgloss.NewStyle().Foreground(lipgloss.Color(purple)).Bold(true),
		MarkdownH3:         lipgloss.NewStyle().Foreground(lipgloss.Color(cyan)).Bold(true),
		MarkdownBlockquote: lipgloss.NewStyle().Foreground(lipgloss.Color(comment)).Italic(true),
		MarkdownCode:       lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		MarkdownCodeBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Faint(true),
		MarkdownRule:       lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		MarkdownBullet:     lipgloss.NewStyle().Foreground(lipgloss.Color(pink)),

		AddedBg:         lipgloss.Color("#1E3A2A"),
		RemovedBg:       lipgloss.Color("#3A1E22"),
		AddedChangeBg:   lipgloss.Color("#2E5740"),
		RemovedChangeBg: lipgloss.Color("#572E33"),

		SyntaxStyle: "dracula",
	}
}

// NordTheme is a true-color theme based on the Nord palette.
func NordTheme() Theme {
	const (
		fg     = "#D8DEE9"
		bg     = "#2E3440"
		panel  = "#3B4252"
		border = "#4C566A"
		green  = "#A3BE8C"
		red    = "#BF616A"
		yellow = "#EBCB8B"
		orange = "#D08770"
		frost1 = "#88C0D0"
		frost2 = "#81A1C1"
		purple = "#B48EAD"
	)
	return Theme{
		SidebarBorder:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		SidebarBorderFocused: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(frost1)),
		MainPane:             lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(border)),
		MainPaneFocused:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(frost1)),

		Added:      lipgloss.NewStyle().Foreground(lipgloss.Color(green)),
		Removed:    lipgloss.NewStyle().Foreground(lipgloss.Color(red)),
		Context:    lipgloss.NewStyle().Foreground(lipgloss.Color(fg)),
		HunkHeader: lipgloss.NewStyle().Foreground(lipgloss.Color(frost2)).Faint(true),
		LineNumber: lipgloss.NewStyle().Foreground(lipgloss.Color(border)),

		CommentBorder:  lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		CommentIssue:   lipgloss.NewStyle().Foreground(lipgloss.Color(red)).Bold(true),
		CommentSuggest: lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)).Bold(true),
		CommentNote:    lipgloss.NewStyle().Foreground(lipgloss.Color(frost2)).Bold(true),
		CommentPraise:  lipgloss.NewStyle().Foreground(lipgloss.Color(green)).Bold(true),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color(panel)).Foreground(lipgloss.Color(fg)),

		ModalOverlay: lipgloss.NewStyle().Background(lipgloss.Color(bg)),
		ModalBorder:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(frost1)).Padding(1, 2),

		MarkdownH1:         lipgloss.NewStyle().Foreground(lipgloss.Color(frost1)).Bold(true),
		MarkdownH2:         lipgloss.NewStyle().Foreground(lipgloss.Color(frost2)).Bold(true),
		MarkdownH3:         lipgloss.NewStyle().Foreground(lipgloss.Color(purple)).Bold(true),
		MarkdownBlockquote: lipgloss.NewStyle().Foreground(lipgloss.Color(border)).Italic(true),
		MarkdownCode:       lipgloss.NewStyle().Foreground(lipgloss.Color(yellow)),
		MarkdownCodeBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Faint(true),
		MarkdownRule:       lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		MarkdownBullet:     lipgloss.NewStyle().Foreground(lipgloss.Color(orange)),

		AddedBg:         lipgloss.Color("#3B4A3B"),
		RemovedBg:       lipgloss.Color("#4A3536"),
		AddedChangeBg:   lipgloss.Color("#4E6A4E"),
		RemovedChangeBg: lipgloss.Color("#6A4B4D"),

		SyntaxStyle: "nord",
	}
}

// themeNames lists the available theme names in cycle order. The first entry
// is the default when an unknown/empty name is requested.
var themeNames = []string{"dark", "light", "molokai", "dracula", "nord"}

// ThemeNames returns the available theme names in cycle order.
func ThemeNames() []string {
	return append([]string(nil), themeNames...)
}

// validThemeName reports whether name is a known theme.
func validThemeName(name string) bool {
	for _, n := range themeNames {
		if n == name {
			return true
		}
	}
	return false
}

// ThemeByName returns the theme for the given name, falling back to the dark
// (default) theme for an empty or unknown name.
func ThemeByName(name string) Theme {
	switch name {
	case "light":
		return LightTheme()
	case "molokai":
		return MolokaiTheme()
	case "dracula":
		return DraculaTheme()
	case "nord":
		return NordTheme()
	default:
		return DefaultTheme()
	}
}

// NextThemeName returns the name following current in cycle order, wrapping
// around. An unknown current name starts the cycle at the first theme.
func NextThemeName(current string) string {
	for i, n := range themeNames {
		if n == current {
			return themeNames[(i+1)%len(themeNames)]
		}
	}
	return themeNames[0]
}
