package tuistart_test

import (
	"testing"

	"tui-start/tui"
)

// ─── Spinner ──────────────────────────────────────────────────────────────────

func TestSpinFrames_Length(t *testing.T) {
	if len(tui.SpinFrames) == 0 {
		t.Fatal("SpinFrames is empty")
	}
}

func TestSpinner_InitialFrame(t *testing.T) {
	s := tui.Spinner{}
	if s.Frame != 0 {
		t.Errorf("initial Frame = %d, want 0", s.Frame)
	}
}

func TestSpinner_View_ReturnsCurrentFrame(t *testing.T) {
	s := tui.Spinner{}
	got := s.View()
	if got != tui.SpinFrames[0] {
		t.Errorf("View() = %q, want %q", got, tui.SpinFrames[0])
	}
}

func TestSpinner_Tick_AdvancesFrame(t *testing.T) {
	s := tui.Spinner{}
	s.Tick()
	if s.Frame != 1 {
		t.Errorf("after one Tick, Frame = %d, want 1", s.Frame)
	}
}

func TestSpinner_Tick_ViewMatchesFrame(t *testing.T) {
	s := tui.Spinner{}
	s.Tick()
	got := s.View()
	want := tui.SpinFrames[s.Frame]
	if got != want {
		t.Errorf("View() = %q, want %q (frame %d)", got, want, s.Frame)
	}
}

func TestSpinner_Tick_Wraps(t *testing.T) {
	s := tui.Spinner{}
	n := len(tui.SpinFrames)
	for i := 0; i < n; i++ {
		s.Tick()
	}
	if s.Frame != 0 {
		t.Errorf("after %d ticks, Frame = %d, want 0 (wrap-around)", n, s.Frame)
	}
}

func TestSpinner_Tick_CyclesAllFrames(t *testing.T) {
	s := tui.Spinner{}
	seen := make(map[string]bool)
	for i := 0; i < len(tui.SpinFrames); i++ {
		seen[s.View()] = true
		s.Tick()
	}
	for i, f := range tui.SpinFrames {
		if !seen[f] {
			t.Errorf("frame %d (%q) was never returned by View()", i, f)
		}
	}
}

// ─── Suggester.Push ───────────────────────────────────────────────────────────

func TestPush_EmptyString_Ignored(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("")
	s.Push("   ")
	if got := s.Match(""); got != nil {
		t.Errorf("expected nil matches for empty prefix, got %v", got)
	}
	matches := s.Match("l")
	for _, m := range matches {
		if m == "" || m == "   " {
			t.Errorf("empty/whitespace-only string was stored")
		}
	}
}

func TestPush_Deduplication(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("git status")
	s.Push("git diff")
	s.Push("git status")

	matches := s.Match("git")
	if len(matches) == 0 {
		t.Fatal("expected matches, got none")
	}
	if matches[0] != "git status" {
		t.Errorf("expected most-recently-pushed command first, got %q", matches[0])
	}
}

func TestPush_WhitespaceTrimmed(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("  echo hello  ")
	matches := s.Match("echo")
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0] != "echo hello" {
		t.Errorf("expected trimmed string %q, got %q", "echo hello", matches[0])
	}
}

// ─── Suggester.Match ──────────────────────────────────────────────────────────

func TestMatch_EmptyPrefix_ReturnsNil(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("ls")
	if got := s.Match(""); got != nil {
		t.Errorf("Match(\"\") = %v, want nil", got)
	}
}

func TestMatch_NoMatches_ReturnsNil(t *testing.T) {
	s := tui.NewSuggester()
	got := s.Match("zzz_no_such_command_xyz")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestMatch_HistoryPreferredOverStatic(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("lshistory")
	matches := s.Match("ls")
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}
	if matches[0] != "lshistory" {
		t.Errorf("expected history entry first, got %q", matches[0])
	}
}

func TestMatch_MaxSuggestionsRespected(t *testing.T) {
	s := tui.NewSuggester()
	got := s.Match("g")
	if len(got) > tui.MaxSuggestions {
		t.Errorf("Match returned %d results, want at most %d", len(got), tui.MaxSuggestions)
	}
}

func TestMatch_NoDuplicates(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("git status")
	matches := s.Match("git")
	seen := make(map[string]bool)
	for _, m := range matches {
		if seen[m] {
			t.Errorf("duplicate entry %q in Match results", m)
		}
		seen[m] = true
	}
}

func TestMatch_AllResultsHavePrefix(t *testing.T) {
	s := tui.NewSuggester()
	s.Push("ls -la")
	s.Push("ls -lh")
	matches := s.Match("ls")
	for _, m := range matches {
		if len(m) < 2 || m[:2] != "ls" {
			t.Errorf("result %q does not have prefix %q", m, "ls")
		}
	}
}

// ─── SuggestionState ─────────────────────────────────────────────────────────

func TestNewSuggestionState_InitialSelected(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b"})
	if ss.Selected != -1 {
		t.Errorf("initial Selected = %d, want -1", ss.Selected)
	}
}

func TestSuggestionState_Next_Advances(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b", "c"})
	ss.Next()
	if ss.Selected != 0 {
		t.Errorf("after 1 Next, Selected = %d, want 0", ss.Selected)
	}
	ss.Next()
	if ss.Selected != 1 {
		t.Errorf("after 2 Next, Selected = %d, want 1", ss.Selected)
	}
}

func TestSuggestionState_Next_Wraps(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b"})
	ss.Next()
	ss.Next()
	ss.Next()
	if ss.Selected != 0 {
		t.Errorf("after wrap, Selected = %d, want 0", ss.Selected)
	}
}

func TestSuggestionState_Next_EmptyMatches_NoOp(t *testing.T) {
	ss := tui.NewSuggestionState(nil)
	ss.Next()
	if ss.Selected != -1 {
		t.Errorf("Next on empty state: Selected = %d, want -1", ss.Selected)
	}
}

func TestSuggestionState_Prev_FromInitial_WrapsToLast(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b", "c"})
	ss.Prev()
	if ss.Selected != 2 {
		t.Errorf("Prev from -1: Selected = %d, want 2", ss.Selected)
	}
}

func TestSuggestionState_Prev_DecreasesIndex(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b", "c"})
	ss.Next()
	ss.Next()
	ss.Prev()
	if ss.Selected != 0 {
		t.Errorf("Prev from 1: Selected = %d, want 0", ss.Selected)
	}
}

func TestSuggestionState_Prev_FromZero_WrapsToLast(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b", "c"})
	ss.Next()
	ss.Prev()
	if ss.Selected != 2 {
		t.Errorf("Prev from 0: Selected = %d, want 2", ss.Selected)
	}
}

func TestSuggestionState_Prev_EmptyMatches_NoOp(t *testing.T) {
	ss := tui.NewSuggestionState(nil)
	ss.Prev()
	if ss.Selected != -1 {
		t.Errorf("Prev on empty state: Selected = %d, want -1", ss.Selected)
	}
}

func TestSuggestionState_Active_NoneSelected(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"a", "b"})
	if got := ss.Active(); got != "" {
		t.Errorf("Active() with no selection = %q, want \"\"", got)
	}
}

func TestSuggestionState_Active_ReturnsSelected(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"alpha", "beta"})
	ss.Next()
	if got := ss.Active(); got != "alpha" {
		t.Errorf("Active() = %q, want %q", got, "alpha")
	}
	ss.Next()
	if got := ss.Active(); got != "beta" {
		t.Errorf("Active() = %q, want %q", got, "beta")
	}
}

func TestSuggestionState_Ghost_ReturnsSuffix(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"git status", "git diff"})
	got := ss.Ghost("git ")
	if got != "status" {
		t.Errorf("Ghost(\"git \") = %q, want %q", got, "status")
	}
}

func TestSuggestionState_Ghost_EmptyMatches(t *testing.T) {
	ss := tui.NewSuggestionState(nil)
	if got := ss.Ghost("anything"); got != "" {
		t.Errorf("Ghost on empty state = %q, want \"\"", got)
	}
}

func TestSuggestionState_Ghost_NoPrefix_ReturnsEmpty(t *testing.T) {
	ss := tui.NewSuggestionState([]string{"ls -la"})
	if got := ss.Ghost("echo"); got != "" {
		t.Errorf("Ghost(\"echo\") with match \"ls -la\" = %q, want \"\"", got)
	}
}
