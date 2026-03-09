// Package themes defines named colour themes for the AI Shell TUI.
// Each theme exposes a complete set of lipgloss styles so the rest of the
// application never needs to know which colours are active.
package themes

import "github.com/charmbracelet/lipgloss"

// ─── Palette ─────────────────────────────────────────────────────────────────

// Palette is the raw colour values for a theme.
type Palette struct {
	Name string

	// Base surfaces
	Bg      lipgloss.Color
	BgPanel lipgloss.Color
	BgAlt   lipgloss.Color

	// Foregrounds
	Fg       lipgloss.Color
	FgDim    lipgloss.Color
	FgSubtle lipgloss.Color

	// Accents
	Primary   lipgloss.Color // main accent (mode badge, AI text)
	Secondary lipgloss.Color // shell / green accent
	Warning   lipgloss.Color // thinking badge / yellow
	Danger    lipgloss.Color // error
	Info      lipgloss.Color // user prompt blue
	Success   lipgloss.Color // shell output

	// Border colours
	BorderNormal  lipgloss.Color
	BorderFocused lipgloss.Color

	// Suggestion colours
	SuggestionText      lipgloss.Color
	SuggestionHighlight lipgloss.Color
	SuggestionSelected  lipgloss.Color
}

// ─── Theme (derived styles) ───────────────────────────────────────────────────

// Theme contains all pre-built lipgloss styles derived from a Palette.
type Theme struct {
	P Palette

	// Layout
	Header    lipgloss.Style
	Panel     lipgloss.Style // viewport border
	InputLine lipgloss.Style // row that holds the prompt + text
	StatusBar lipgloss.Style
	Separator lipgloss.Style

	// Mode badges
	BadgeShell    lipgloss.Style
	BadgeAI       lipgloss.Style
	BadgeThink    lipgloss.Style
	BadgeAuto     lipgloss.Style // auto-execute mode
	BadgeName     lipgloss.Style // theme name chip
	BadgeScrolled lipgloss.Style

	// Message styles
	MsgSystem   lipgloss.Style
	MsgUser     lipgloss.Style
	MsgAI       lipgloss.Style
	MsgShellCmd lipgloss.Style
	MsgShellOut lipgloss.Style
	MsgError    lipgloss.Style
	MsgAutoTask lipgloss.Style // auto-execute: task heading
	MsgAutoStep lipgloss.Style // auto-execute: planned command
	MsgAutoOut  lipgloss.Style // auto-execute: command output
	MsgAutoDone lipgloss.Style // auto-execute: completion summary

	// Input
	PromptShell lipgloss.Style
	PromptAI    lipgloss.Style
	Cursor      lipgloss.Style

	// Suggestions
	SuggestionDim        lipgloss.Style
	SuggestionHint       lipgloss.Style
	SuggestionSelectedSt lipgloss.Style
	SuggestionBox        lipgloss.Style
}

func build(p Palette) Theme {
	badge := func(bg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Bold(true).
			Foreground(p.Bg).Background(bg).Padding(0, 1)
	}
	return Theme{
		P: p,

		// Layout ─────────────────────────────────────────────────────────
		Header: lipgloss.NewStyle().Bold(true).
			Foreground(p.Fg).Background(p.BgPanel).
			PaddingLeft(1).PaddingRight(1),

		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.BorderNormal),

		InputLine: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.BorderFocused),

		StatusBar: lipgloss.NewStyle().
			Foreground(p.FgSubtle),

		Separator: lipgloss.NewStyle().
			Foreground(p.FgSubtle),

		// Badges ─────────────────────────────────────────────────────────
		BadgeShell:    badge(p.Secondary),
		BadgeAI:       badge(p.Primary),
		BadgeThink:    badge(p.Warning),
		BadgeAuto:     badge(p.Info),
		BadgeName:     lipgloss.NewStyle().Bold(true).Foreground(p.FgDim).Padding(0, 1),
		BadgeScrolled: lipgloss.NewStyle().Bold(true).Foreground(p.Warning),

		// Messages ───────────────────────────────────────────────────────
		MsgSystem:   lipgloss.NewStyle().Foreground(p.FgSubtle).Italic(true),
		MsgUser:     lipgloss.NewStyle().Foreground(p.Info).Bold(true),
		MsgAI:       lipgloss.NewStyle().Foreground(p.Primary),
		MsgShellCmd: lipgloss.NewStyle().Foreground(p.Secondary).Bold(true),
		MsgShellOut: lipgloss.NewStyle().Foreground(p.Fg),
		MsgError:    lipgloss.NewStyle().Foreground(p.Danger),
		MsgAutoTask: lipgloss.NewStyle().Foreground(p.Info).Bold(true),
		MsgAutoStep: lipgloss.NewStyle().Foreground(p.Warning).Bold(true),
		MsgAutoOut:  lipgloss.NewStyle().Foreground(p.Fg),
		MsgAutoDone: lipgloss.NewStyle().Foreground(p.Success).Bold(true),

		// Input ───────────────────────────────────────────────────────────
		PromptShell: lipgloss.NewStyle().Foreground(p.Secondary).Bold(true),
		PromptAI:    lipgloss.NewStyle().Foreground(p.Primary).Bold(true),
		Cursor:      lipgloss.NewStyle().Background(p.Fg).Foreground(p.Bg),

		// Suggestions ─────────────────────────────────────────────────────
		SuggestionDim: lipgloss.NewStyle().Foreground(p.SuggestionText),
		SuggestionHint: lipgloss.NewStyle().Foreground(p.SuggestionHighlight).
			Italic(true),
		SuggestionSelectedSt: lipgloss.NewStyle().
			Foreground(p.Bg).Background(p.SuggestionSelected).Bold(true).Padding(0, 1),
		SuggestionBox: lipgloss.NewStyle().
			Foreground(p.FgSubtle),
	}
}

// ─── Named palettes ───────────────────────────────────────────────────────────

// Dracula is the iconic dark purple theme.
var Dracula = build(Palette{
	Name:                "Dracula",
	Bg:                  "#282a36",
	BgPanel:             "#21222c",
	BgAlt:               "#343746",
	Fg:                  "#f8f8f2",
	FgDim:               "#6272a4",
	FgSubtle:            "#44475a",
	Primary:             "#bd93f9",
	Secondary:           "#50fa7b",
	Warning:             "#ffb86c",
	Danger:              "#ff5555",
	Info:                "#8be9fd",
	Success:             "#50fa7b",
	BorderNormal:        "#44475a",
	BorderFocused:       "#bd93f9",
	SuggestionText:      "#6272a4",
	SuggestionHighlight: "#ffb86c",
	SuggestionSelected:  "#bd93f9",
})

// TokyoNight is a cool blue/indigo dark theme.
var TokyoNight = build(Palette{
	Name:                "Tokyo Night",
	Bg:                  "#1a1b26",
	BgPanel:             "#16161e",
	BgAlt:               "#1f2335",
	Fg:                  "#c0caf5",
	FgDim:               "#565f89",
	FgSubtle:            "#3b4261",
	Primary:             "#7aa2f7",
	Secondary:           "#9ece6a",
	Warning:             "#e0af68",
	Danger:              "#f7768e",
	Info:                "#7dcfff",
	Success:             "#9ece6a",
	BorderNormal:        "#3b4261",
	BorderFocused:       "#7aa2f7",
	SuggestionText:      "#565f89",
	SuggestionHighlight: "#e0af68",
	SuggestionSelected:  "#7aa2f7",
})

// Gruvbox is the warm retro dark theme.
var Gruvbox = build(Palette{
	Name:                "Gruvbox",
	Bg:                  "#282828",
	BgPanel:             "#1d2021",
	BgAlt:               "#3c3836",
	Fg:                  "#ebdbb2",
	FgDim:               "#928374",
	FgSubtle:            "#504945",
	Primary:             "#d3869b",
	Secondary:           "#b8bb26",
	Warning:             "#fabd2f",
	Danger:              "#fb4934",
	Info:                "#83a598",
	Success:             "#b8bb26",
	BorderNormal:        "#504945",
	BorderFocused:       "#d3869b",
	SuggestionText:      "#928374",
	SuggestionHighlight: "#fabd2f",
	SuggestionSelected:  "#d3869b",
})

// Catppuccin (Mocha variant) — soft, pastel dark theme.
var Catppuccin = build(Palette{
	Name:                "Catppuccin",
	Bg:                  "#1e1e2e",
	BgPanel:             "#181825",
	BgAlt:               "#313244",
	Fg:                  "#cdd6f4",
	FgDim:               "#7f849c",
	FgSubtle:            "#45475a",
	Primary:             "#cba6f7",
	Secondary:           "#a6e3a1",
	Warning:             "#fab387",
	Danger:              "#f38ba8",
	Info:                "#89dceb",
	Success:             "#a6e3a1",
	BorderNormal:        "#45475a",
	BorderFocused:       "#cba6f7",
	SuggestionText:      "#7f849c",
	SuggestionHighlight: "#fab387",
	SuggestionSelected:  "#cba6f7",
})

// Nord is the icy arctic dark theme.
var Nord = build(Palette{
	Name:                "Nord",
	Bg:                  "#2e3440",
	BgPanel:             "#242933",
	BgAlt:               "#3b4252",
	Fg:                  "#eceff4",
	FgDim:               "#7b88a1",
	FgSubtle:            "#4c566a",
	Primary:             "#88c0d0",
	Secondary:           "#a3be8c",
	Warning:             "#ebcb8b",
	Danger:              "#bf616a",
	Info:                "#81a1c1",
	Success:             "#a3be8c",
	BorderNormal:        "#4c566a",
	BorderFocused:       "#88c0d0",
	SuggestionText:      "#7b88a1",
	SuggestionHighlight: "#ebcb8b",
	SuggestionSelected:  "#88c0d0",
})

// Solarized is the classic light/dark balanced theme (dark variant).
var Solarized = build(Palette{
	Name:                "Solarized",
	Bg:                  "#002b36",
	BgPanel:             "#073642",
	BgAlt:               "#083f4d",
	Fg:                  "#839496",
	FgDim:               "#586e75",
	FgSubtle:            "#073642",
	Primary:             "#268bd2",
	Secondary:           "#859900",
	Warning:             "#b58900",
	Danger:              "#dc322f",
	Info:                "#2aa198",
	Success:             "#859900",
	BorderNormal:        "#073642",
	BorderFocused:       "#268bd2",
	SuggestionText:      "#586e75",
	SuggestionHighlight: "#b58900",
	SuggestionSelected:  "#268bd2",
})

// All is the ordered list of available themes for cycling.
var All = []Theme{Dracula, TokyoNight, Gruvbox, Catppuccin, Nord, Solarized}
