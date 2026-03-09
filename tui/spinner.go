// Package tui contains shared TUI components for AI Shell.
// This file provides a simple frame-based spinner driven by tea.Tick.
package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// SpinTickMsg is sent on every spinner tick.
type SpinTickMsg struct{}

// SpinnerFPS is the refresh rate for the spinner animation.
const SpinnerFPS = 80 * time.Millisecond

// SpinFrames is the sequence of characters used for the spinner.
var SpinFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// SpinTick returns the command that fires the next tick.
func SpinTick() tea.Cmd {
	return tea.Tick(SpinnerFPS, func(time.Time) tea.Msg {
		return SpinTickMsg{}
	})
}

// Spinner holds the current frame index.
type Spinner struct {
	Frame int
}

// Tick advances the frame and returns the next tick command.
func (s *Spinner) Tick() tea.Cmd {
	s.Frame = (s.Frame + 1) % len(SpinFrames)
	return SpinTick()
}

// View returns the current spinner character.
func (s Spinner) View() string {
	return SpinFrames[s.Frame]
}
