package tuistart_test

import (
	"testing"

	"tui-start/themes"
)

func TestAll_Length(t *testing.T) {
	if len(themes.All) != 6 {
		t.Errorf("len(All) = %d, want 6", len(themes.All))
	}
}

func TestAll_ExpectedNames(t *testing.T) {
	want := []string{"Dracula", "Tokyo Night", "Gruvbox", "Catppuccin", "Nord", "Solarized"}
	for i, theme := range themes.All {
		if theme.P.Name != want[i] {
			t.Errorf("All[%d].P.Name = %q, want %q", i, theme.P.Name, want[i])
		}
	}
}

func TestAll_NamesAreNonEmpty(t *testing.T) {
	for i, theme := range themes.All {
		if theme.P.Name == "" {
			t.Errorf("All[%d].P.Name is empty", i)
		}
	}
}

func TestAll_ColorsAreNonEmpty(t *testing.T) {
	for _, theme := range themes.All {
		p := theme.P
		checks := []struct {
			name  string
			value string
		}{
			{"Bg", string(p.Bg)},
			{"Fg", string(p.Fg)},
			{"Primary", string(p.Primary)},
			{"Secondary", string(p.Secondary)},
			{"Warning", string(p.Warning)},
			{"Danger", string(p.Danger)},
			{"Info", string(p.Info)},
			{"Success", string(p.Success)},
			{"BorderNormal", string(p.BorderNormal)},
			{"BorderFocused", string(p.BorderFocused)},
		}
		for _, c := range checks {
			if c.value == "" {
				t.Errorf("theme %q: %s is empty", p.Name, c.name)
			}
		}
	}
}

func TestNamedThemes_MatchAll(t *testing.T) {
	named := []themes.Theme{themes.Dracula, themes.TokyoNight, themes.Gruvbox, themes.Catppuccin, themes.Nord, themes.Solarized}
	for i, n := range named {
		if n.P.Name != themes.All[i].P.Name {
			t.Errorf("named[%d].P.Name = %q, All[%d].P.Name = %q", i, n.P.Name, i, themes.All[i].P.Name)
		}
	}
}

func TestDracula_Name(t *testing.T) {
	if themes.Dracula.P.Name != "Dracula" {
		t.Errorf("Dracula.P.Name = %q, want %q", themes.Dracula.P.Name, "Dracula")
	}
}

func TestTokyoNight_Name(t *testing.T) {
	if themes.TokyoNight.P.Name != "Tokyo Night" {
		t.Errorf("TokyoNight.P.Name = %q, want %q", themes.TokyoNight.P.Name, "Tokyo Night")
	}
}

func TestTheme_StyleFields_NonZero(t *testing.T) {
	for _, theme := range themes.All {
		if theme.MsgUser.Render("x") == "" {
			t.Errorf("theme %q: MsgUser.Render empty", theme.P.Name)
		}
		if theme.MsgAI.Render("x") == "" {
			t.Errorf("theme %q: MsgAI.Render empty", theme.P.Name)
		}
		if theme.MsgError.Render("x") == "" {
			t.Errorf("theme %q: MsgError.Render empty", theme.P.Name)
		}
		if theme.PromptShell.Render("x") == "" {
			t.Errorf("theme %q: PromptShell.Render empty", theme.P.Name)
		}
	}
}
