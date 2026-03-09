package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"tui-start/agents"
	"tui-start/models"
	"tui-start/storage"
	"tui-start/themes"
	"tui-start/tools"
	"tui-start/tui"
)

// ─── Tea messages ─────────────────────────────────────────────────────────────

// aiResponseMsg is returned by the async AI goroutine.
type aiResponseMsg struct {
	text string
	err  error
}

// autoStepMsg carries a single step from the AutoAgent progress channel.
type autoStepMsg struct {
	step agents.AutoStep
}

// autoNextMsg triggers reading the next step from the auto-agent channel.
type autoNextMsg struct {
	ch <-chan agents.AutoStep
}

// ─── Mode ─────────────────────────────────────────────────────────────────────

type inputMode int

const (
	modeShell inputMode = iota
	modeAI
	modeAuto // autonomous task-execution mode
)

// ─── styledLine ───────────────────────────────────────────────────────────────

type styledLine struct{ text string }

// ─── appModel ─────────────────────────────────────────────────────────────────

type appModel struct {
	// Terminal dimensions
	width, height int

	// Conversation history (raw) + rendered viewport buffer
	messages []models.Message
	lines    []styledLine

	// Viewport scroll (lines from bottom)
	scrollOffset int

	// Text input
	input  []rune
	cursor int

	// Mode & loading state
	mode    inputMode
	loading bool
	spinner tui.Spinner

	// Working directory
	cwd string

	// AI agent
	agent *agents.ShellAgent

	// Auto-execute agent
	autoAgent *agents.AutoAgent

	// Theme
	themeIdx int
	theme    themes.Theme

	// Inline suggestions
	suggester *tui.Suggester
	suggest   tui.SuggestionState

	// History navigation
	history    []string
	histCursor int // -1 = not navigating
	histDraft  []rune

	// BoltDB store (may be nil if open failed)
	store *storage.Store

	// providerCfg is kept for AutoAgent construction.
	providerCfg agents.ProviderConfig

	// ── Session / sidebar ─────────────────────────────────────────────────
	sessions      []storage.SessionMeta // all named sessions
	activeSession string                // ID of the currently viewed session
	sessionCursor int                   // highlighted row in sidebar (0-based)
	sidebarFocus  bool                  // true when sidebar has keyboard focus
	sidebarScroll int                   // scroll offset for the session list
}

func initialModel(ag *agents.ShellAgent, cwd string, providerCfg agents.ProviderConfig, st *storage.Store) appModel {
	m := appModel{
		agent:       ag,
		cwd:         cwd,
		providerCfg: providerCfg,
		store:       st,
		width:       80,
		height:      24,
		mode:        modeShell,
		themeIdx:    0,
		theme:       themes.All[0],
		suggester:   tui.NewSuggester(),
		histCursor:  -1,
		autoAgent:   agents.NewAutoAgent(providerCfg, cwd),
	}

	// ── Restore scalar preferences ────────────────────────────────────────
	if st != nil {
		if idx, err := st.LoadThemeIdx(); err == nil && idx >= 0 && idx < len(themes.All) {
			m.themeIdx = idx
			m.theme = themes.All[idx]
		}
		if savedCwd, err := st.LoadCwd(); err == nil && savedCwd != "" {
			if _, statErr := os.Stat(savedCwd); statErr == nil {
				m.cwd = savedCwd
				_ = os.Chdir(savedCwd)
				m.autoAgent.SetCwd(savedCwd)
			}
		}
		if history, err := st.LoadHistory(); err == nil {
			m.history = history
			for _, h := range history {
				m.suggester.Push(h)
			}
		}

		// ── Load / create sessions ────────────────────────────────────────
		sessions, err := st.ListSessions()
		if err != nil || len(sessions) == 0 {
			// First run: create the default "New Chat" session.
			sess, err := st.CreateSession("New Chat")
			if err == nil {
				sessions = []storage.SessionMeta{sess}
				_ = st.SaveActiveSession(sess.ID)
			}
		}
		m.sessions = sessions

		// Active session: prefer saved, fall back to newest.
		activeID, _ := st.LoadActiveSession()
		if activeID == "" && len(sessions) > 0 {
			activeID = sessions[len(sessions)-1].ID
		}
		m.activeSession = activeID

		// Point sidebar cursor at the active session.
		for i, s := range m.sessions {
			if s.ID == activeID {
				m.sessionCursor = i
				break
			}
		}

		// Load messages for the active session.
		if activeID != "" {
			if msgs, err := st.LoadSessionMessages(activeID); err == nil {
				m.messages = msgs
			}
		}
	} else {
		// No store: create an ephemeral in-memory session.
		sess := storage.SessionMeta{
			ID:        fmt.Sprintf("local-%d", time.Now().UnixNano()),
			Name:      "New Chat",
			CreatedAt: time.Now(),
		}
		m.sessions = []storage.SessionMeta{sess}
		m.activeSession = sess.ID
	}

	// Welcome / restored banner
	msgCount := len(m.messages)
	if msgCount > 0 {
		m.pushMsg(models.New(models.RoleSystem, fmt.Sprintf(
			"🚀 AI-TERM — session restored · %d messages · theme: %s",
			msgCount, m.theme.P.Name)))
	} else {
		m.pushMsg(models.New(models.RoleSystem,
			"🚀 AI-TERM ready  ·  Tab: mode  ·  Ctrl+B: sessions  ·  Ctrl+N: new chat  ·  Ctrl+T: theme  ·  Ctrl+C: quit"))
	}
	return m
}

// ─── BubbleTea interface ──────────────────────────────────────────────────────

func (m appModel) Init() tea.Cmd { return nil }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildLines()
		return m, nil

	case tui.SpinTickMsg:
		if m.loading {
			return m, m.spinner.Tick()
		}
		return m, nil

	case aiResponseMsg:
		m.loading = false
		if msg.err != nil {
			newMsg := models.New(models.RoleError, fmt.Sprintf("AI error: %v", msg.err))
			m.pushMsg(newMsg)
			m.persistMessage(newMsg)
		} else {
			newMsg := models.New(models.RoleAI, msg.text)
			m.pushMsg(newMsg)
			m.persistMessage(newMsg)
		}
		m.scrollOffset = 0
		return m, nil

	// ── Auto-agent step streaming ─────────────────────────────────────────
	case autoNextMsg:
		step, ok := <-msg.ch
		if !ok {
			// Channel closed — task finished
			m.loading = false
			return m, nil
		}
		newMsg := m.autoStepToMessage(step)
		m.pushMsg(newMsg)
		m.persistMessage(newMsg)
		m.scrollOffset = 0
		// Stop spinner on terminal steps
		if step.Kind == agents.AutoStepDone || step.Kind == agents.AutoStepError {
			m.loading = false
			return m, nil
		}
		// Schedule reading the next step
		ch := msg.ch
		return m, func() tea.Msg { return autoNextMsg{ch: ch} }

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m appModel) View() tea.View {
	return tea.NewView(m.render())
}

// ─── Key handling ─────────────────────────────────────────────────────────────

func (m appModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Always honour quit — save state first
	if msg.String() == "ctrl+c" {
		m.saveState()
		return m, tea.Quit
	}

	// ── Sidebar focus intercept ─────────────────────────────────────────
	if m.sidebarFocus {
		return m.handleSidebarKey(msg)
	}

	// ── Global shortcuts (available any time sidebar is not focused) ──────
	switch msg.String() {
	case "ctrl+b":
		if !m.loading {
			m.sidebarFocus = true
			for i, s := range m.sessions {
				if s.ID == m.activeSession {
					m.sessionCursor = i
					m.clampSidebarScroll()
					break
				}
			}
		}
		return m, nil
	case "ctrl+n":
		if !m.loading {
			return m.createNewSession()
		}
		return m, nil
	}

	// Printable characters → insert, then update suggestions
	if msg.Text != "" {
		m = m.insertText(msg.Text)
		m.histCursor = -1
		m.updateSuggest()
		return m, nil
	}

	switch msg.String() {

	// ── Theme cycling ────────────────────────────────────────────────────
	case "ctrl+t":
		m.themeIdx = (m.themeIdx + 1) % len(themes.All)
		m.theme = themes.All[m.themeIdx]
		m.rebuildLines()
		m.pushMsg(models.New(models.RoleSystem,
			fmt.Sprintf("Theme → %s", m.theme.P.Name)))
		if m.store != nil {
			_ = m.store.SaveThemeIdx(m.themeIdx)
		}

	// ── Auto-execute mode toggle ─────────────────────────────────────────
	case "ctrl+x":
		if m.loading {
			return m, nil
		}
		switch m.mode {
		case modeAuto:
			m.mode = modeShell
		default:
			m.mode = modeAuto
			m.suggest = tui.NewSuggestionState(nil)
		}
	// ── Clear screen ─────────────────────────────────────────────────────
	case "ctrl+l":
		m.messages = nil
		m.lines = nil
		m.pushMsg(models.New(models.RoleSystem, "Screen cleared."))
		if m.store != nil {
			_ = m.store.ClearSessionMessages(m.activeSession)
		}
	// ── Tab: accept suggestion, or cycle mode ─────────────────────────────
	case "tab":
		typed := string(m.input)
		if m.mode == modeShell && len(m.suggest.Matches) > 0 {
			// Accept the first (or selected) suggestion
			var accepted string
			if m.suggest.Selected >= 0 {
				accepted = m.suggest.Active()
			} else {
				accepted = m.suggest.Matches[0]
			}
			m.input = []rune(accepted)
			m.cursor = len(m.input)
			m.suggest = tui.NewSuggestionState(nil)
		} else if m.mode == modeShell && len(typed) == 0 {
			m.mode = modeAI
			m.suggest = tui.NewSuggestionState(nil)
		} else if m.mode == modeAI && len(typed) == 0 {
			m.mode = modeShell
		}

	// ── Execute ──────────────────────────────────────────────────────────
	case "enter":
		if m.loading {
			return m, nil
		}
		input := strings.TrimSpace(string(m.input))
		if input == "" {
			return m, nil
		}
		m.input = nil
		m.cursor = 0
		m.scrollOffset = 0
		m.suggest = tui.NewSuggestionState(nil)
		m.histCursor = -1
		m.histDraft = nil
		switch m.mode {
		case modeShell:
			m.suggester.Push(input)
			m.history = append(m.history, input)
			if m.store != nil {
				_ = m.store.AppendHistory(input)
			}
			return m.execShell(input)
		case modeAI:
			return m.askAI(input)
		case modeAuto:
			return m.runAutoTask(input)
		}

	// ── Suggestion navigation (Alt+↑/↓ or Ctrl+P/N in shell mode) ────────
	case "alt+down":
		if m.mode == modeShell && len(m.suggest.Matches) > 0 {
			m.suggest.Next()
			if sel := m.suggest.Active(); sel != "" {
				m.input = []rune(sel)
				m.cursor = len(m.input)
			}
			return m, nil
		}
		m.scrollDown(1)
	case "alt+up", "ctrl+p":
		if m.mode == modeShell && len(m.suggest.Matches) > 0 {
			m.suggest.Prev()
			if sel := m.suggest.Active(); sel != "" {
				m.input = []rune(sel)
				m.cursor = len(m.input)
			}
			return m, nil
		}
		m.scrollUp(1)

	// ── History navigation (↑/↓ when no suggestions) ─────────────────────
	case "up":
		if m.mode == modeShell && len(m.suggest.Matches) == 0 {
			m.navigateHistory(-1)
		} else {
			m.scrollUp(1)
		}
	case "down":
		if m.mode == modeShell && len(m.suggest.Matches) == 0 {
			m.navigateHistory(1)
		} else {
			m.scrollDown(1)
		}

	// ── Scrolling ────────────────────────────────────────────────────────
	case "pgup":
		m.scrollUp(m.viewportHeight() / 2)
	case "pgdown":
		m.scrollDown(m.viewportHeight() / 2)

	// ── Right-arrow: accept inline ghost ────────────────────────────────
	case "right":
		// If cursor is at end and there's a ghost, accept it
		ghost := m.suggest.Ghost(string(m.input))
		if m.cursor == len(m.input) && ghost != "" {
			m.input = append(m.input, []rune(ghost)...)
			m.cursor = len(m.input)
			m.updateSuggest()
		} else if m.cursor < len(m.input) {
			m.cursor++
		}

	// ── Editing ──────────────────────────────────────────────────────────
	case "backspace":
		if m.cursor > 0 {
			m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
			m.cursor--
			m.histCursor = -1
			m.updateSuggest()
		}
	case "delete":
		if m.cursor < len(m.input) {
			m.input = append(m.input[:m.cursor], m.input[m.cursor+1:]...)
			m.updateSuggest()
		}
	case "left":
		if m.cursor > 0 {
			m.cursor--
		}
	case "home":
		m.cursor = 0
	case "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = len(m.input)
	case "ctrl+k":
		m.input = m.input[:m.cursor]
		m.updateSuggest()
	case "ctrl+u":
		m.input = m.input[m.cursor:]
		m.cursor = 0
		m.updateSuggest()
	case "ctrl+w":
		m.input, m.cursor = deleteWordBefore(m.input, m.cursor)
		m.updateSuggest()
	case "esc":
		m.suggest = tui.NewSuggestionState(nil)
	}

	return m, nil
}

// ─── Text insertion ──────────────────────────────────────────────────────────

func (m appModel) insertText(text string) appModel {
	runes := []rune(text)
	n := len(runes)
	grown := make([]rune, len(m.input)+n)
	copy(grown, m.input[:m.cursor])
	copy(grown[m.cursor:], runes)
	copy(grown[m.cursor+n:], m.input[m.cursor:])
	m.input = grown
	m.cursor += n
	return m
}

func deleteWordBefore(input []rune, cursor int) ([]rune, int) {
	if cursor == 0 {
		return input, cursor
	}
	i := cursor - 1
	for i > 0 && input[i] == ' ' {
		i--
	}
	for i > 0 && input[i-1] != ' ' {
		i--
	}
	return append(input[:i], input[cursor:]...), i
}

// ─── Suggestions ─────────────────────────────────────────────────────────────

func (m *appModel) updateSuggest() {
	if m.mode != modeShell {
		m.suggest = tui.NewSuggestionState(nil)
		return
	}
	typed := string(m.input)
	if strings.TrimSpace(typed) == "" {
		m.suggest = tui.NewSuggestionState(nil)
		return
	}
	matches := m.suggester.Match(typed)
	// Filter out the exact match (already fully typed)
	if len(matches) == 1 && matches[0] == typed {
		m.suggest = tui.NewSuggestionState(nil)
		return
	}
	m.suggest = tui.NewSuggestionState(matches)
}

// ─── History navigation ───────────────────────────────────────────────────────

func (m *appModel) navigateHistory(dir int) {
	if len(m.history) == 0 {
		return
	}
	if m.histCursor == -1 {
		m.histDraft = append([]rune(nil), m.input...)
		m.histCursor = len(m.history)
	}
	next := m.histCursor + dir
	if next < 0 {
		return
	}
	if next >= len(m.history) {
		// Restore draft
		m.histCursor = -1
		m.input = append([]rune(nil), m.histDraft...)
		m.cursor = len(m.input)
		return
	}
	m.histCursor = next
	m.input = []rune(m.history[len(m.history)-1-m.histCursor])
	m.cursor = len(m.input)
}

// ─── Shell execution ──────────────────────────────────────────────────────────

func (m appModel) execShell(command string) (appModel, tea.Cmd) {
	m.maybeRenameSession(command)
	cmdMsg := models.New(models.RoleShellCmd,
		fmt.Sprintf("[%s] $ %s", shortPath(m.cwd), command))
	m.pushMsg(cmdMsg)
	m.persistMessage(cmdMsg)

	if isCd(command) {
		return m.handleCd(command), nil
	}

	stdout, stderr, exitCode := tools.ExecCommand(m.cwd, command)

	if out := strings.TrimRight(stdout, "\n"); out != "" {
		outMsg := models.New(models.RoleShellOut, out)
		m.pushMsg(outMsg)
		m.persistMessage(outMsg)
	}
	if errOut := strings.TrimRight(stderr, "\n"); errOut != "" {
		errMsg := models.New(models.RoleError, errOut)
		m.pushMsg(errMsg)
		m.persistMessage(errMsg)
	}
	if stdout == "" && stderr == "" {
		sysMsg := models.New(models.RoleSystem, fmt.Sprintf("(exit %d)", exitCode))
		m.pushMsg(sysMsg)
		m.persistMessage(sysMsg)
	}
	return m, nil
}

func isCd(command string) bool {
	s := strings.TrimSpace(command)
	return s == "cd" || strings.HasPrefix(s, "cd ") || strings.HasPrefix(s, "cd\t")
}

func (m appModel) handleCd(command string) appModel {
	parts := strings.Fields(command)
	var target string
	if len(parts) < 2 || parts[1] == "~" {
		target = os.Getenv("HOME")
	} else {
		target = parts[1]
	}
	if err := os.Chdir(target); err != nil {
		errMsg := models.New(models.RoleError, fmt.Sprintf("cd: %v", err))
		m.pushMsg(errMsg)
		m.persistMessage(errMsg)
	} else {
		newCwd, _ := os.Getwd()
		m.cwd = newCwd
		m.autoAgent.SetCwd(newCwd)
		sysMsg := models.New(models.RoleSystem, fmt.Sprintf("→ %s", m.cwd))
		m.pushMsg(sysMsg)
		m.persistMessage(sysMsg)
		if m.store != nil {
			_ = m.store.SaveCwd(m.cwd)
		}
	}
	return m
}

// ─── AI query ─────────────────────────────────────────────────────────────────

func (m appModel) askAI(prompt string) (appModel, tea.Cmd) {
	m.maybeRenameSession(prompt)
	userMsg := models.New(models.RoleUser, prompt)
	m.pushMsg(userMsg)
	m.persistMessage(userMsg)
	m.loading = true
	ag := m.agent
	spinCmd := tui.SpinTick()
	return m, tea.Batch(
		spinCmd,
		func() tea.Msg {
			resp, err := ag.Ask(context.Background(), prompt)
			return aiResponseMsg{text: resp, err: err}
		},
	)
}

// ─── Auto-execute task ─────────────────────────────────────────────────────────────

func (m appModel) runAutoTask(task string) (appModel, tea.Cmd) {
	taskMsg := models.New(models.RoleAutoTask, "⚡ AUTO: "+task)
	m.pushMsg(taskMsg)
	m.persistMessage(taskMsg)
	m.loading = true

	aa := m.autoAgent
	ctx := context.Background()
	spinCmd := tui.SpinTick()

	// Start the auto agent; it writes progress to the returned channel.
	ch := aa.Run(ctx, task)

	// Batch: start spinner + immediately schedule first step read.
	return m, tea.Batch(spinCmd, func() tea.Msg {
		return autoNextMsg{ch: ch}
	})
}

// autoStepToMessage converts an AutoStep into a models.Message for display.
func (m *appModel) autoStepToMessage(step agents.AutoStep) models.Message {
	switch step.Kind {
	case agents.AutoStepPlan:
		return models.New(models.RoleAutoStep, "▶ "+step.Text)
	case agents.AutoStepOutput:
		return models.New(models.RoleAutoOut, step.Text)
	case agents.AutoStepInfo:
		return models.New(models.RoleSystem, step.Text)
	case agents.AutoStepDone:
		return models.New(models.RoleAutoDone, "✔ "+step.Text)
	case agents.AutoStepError:
		text := step.Text
		if step.Err != nil {
			text = step.Err.Error()
		}
		return models.New(models.RoleError, "✗ AUTO ERROR: "+text)
	default:
		return models.New(models.RoleSystem, step.Text)
	}
}

// ─── Persistence helpers ─────────────────────────────────────────────────────────────

func (m *appModel) persistMessage(msg models.Message) {
	if m.store == nil || m.activeSession == "" {
		return
	}
	_ = m.store.AppendSessionMessage(m.activeSession, msg)
}

// saveState flushes all mutable state to BoltDB.
func (m *appModel) saveState() {
	if m.store == nil {
		return
	}
	_ = m.store.SaveHistory(m.history)
	_ = m.store.SaveThemeIdx(m.themeIdx)
	_ = m.store.SaveCwd(m.cwd)
	_ = m.store.SaveActiveSession(m.activeSession)
}

// ─── Scroll helpers ───────────────────────────────────────────────────────────

func (m *appModel) scrollUp(n int) {
	maxOff := len(m.lines) - m.viewportHeight()
	if maxOff < 0 {
		maxOff = 0
	}
	m.scrollOffset += n
	if m.scrollOffset > maxOff {
		m.scrollOffset = maxOff
	}
}

func (m *appModel) scrollDown(n int) {
	m.scrollOffset -= n
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// ─── Rendering ────────────────────────────────────────────────────────────────

func (m appModel) render() string {
	if m.width == 0 {
		return "initialising…"
	}
	header := m.renderHeader()
	mainArea := m.renderMainArea()
	status := m.renderStatus()

	if m.sidebarWidth() > 0 {
		sidebar := m.renderSidebar()
		middle := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainArea)
		return header + "\n" + middle + "\n" + status
	}
	return header + "\n" + mainArea + "\n" + status
}

// renderMainArea stacks the viewport, suggestion bar, and input panel.
func (m appModel) renderMainArea() string {
	return m.renderViewport() + "\n" + m.renderSuggestionBar() + "\n" + m.renderInputPanel()
}

// renderHeader builds the full-width top bar: 🚀 AI-TERM title + badges.
func (m appModel) renderHeader() string {
	th := m.theme
	title := "  🚀  AI-TERM"

	var modeBadge string
	switch {
	case m.loading && m.mode == modeAuto:
		modeBadge = th.BadgeThink.Render(" " + m.spinner.View() + "  RUNNING ")
	case m.loading:
		modeBadge = th.BadgeThink.Render(" " + m.spinner.View() + "  THINKING ")
	case m.mode == modeShell:
		modeBadge = th.BadgeShell.Render("  SHELL ")
	case m.mode == modeAI:
		modeBadge = th.BadgeAI.Render("  AI ")
	default:
		modeBadge = th.BadgeAuto.Render("  AUTO ")
	}

	themeBadge := th.BadgeName.Render("◈ " + th.P.Name)

	// Active session name badge
	var sessBadge string
	for _, s := range m.sessions {
		if s.ID == m.activeSession {
			name := s.Name
			if len([]rune(name)) > 18 {
				name = string([]rune(name)[:17]) + "…"
			}
			sessBadge = "  " + th.BadgeName.Render("💬 "+name)
			break
		}
	}

	// Storage indicator
	var storeBadge string
	if m.store != nil {
		storeBadge = " " + th.SuggestionDim.Render("💾")
	}

	rightSide := storeBadge + sessBadge + "  " + themeBadge + "  " + modeBadge + " "
	innerW := m.width - 2
	gap := innerW - lipgloss.Width(title) - lipgloss.Width(rightSide)
	if gap < 0 {
		gap = 0
	}
	content := title + strings.Repeat(" ", gap) + rightSide
	return th.Header.Width(m.width).Render(content)
}

// renderViewport builds the scrollable message panel with a rounded border.
func (m appModel) renderViewport() string {
	th := m.theme
	panelW := m.mainWidth()
	innerW := panelW - 2
	if innerW < 4 {
		innerW = 4
	}
	vpH := m.viewportHeight()

	visible := m.visibleLines(vpH)
	var rows []string
	for _, line := range visible {
		padded := line + strings.Repeat(" ", max(0, innerW-lipgloss.Width(line)))
		rows = append(rows, padded)
	}

	if m.scrollOffset > 0 {
		indicator := th.BadgeScrolled.Render(
			fmt.Sprintf(" ↑ scrolled +%d lines ", m.scrollOffset))
		rows = append(rows, indicator)
		if len(rows) > vpH {
			rows = rows[1:]
		}
	}

	content := strings.Join(rows, "\n")
	return th.Panel.Width(innerW).Render(content)
}

// renderSuggestionBar renders the inline suggestion row beneath the viewport.
func (m appModel) renderSuggestionBar() string {
	th := m.theme
	typed := string(m.input)

	// Ghost text (dim suffix preview for the first match)
	ghost := m.suggest.Ghost(typed)
	if ghost == "" && len(m.suggest.Matches) == 0 {
		// Show a placeholder hint instead
		var hint string
		if m.mode == modeShell {
			hint = "  type to see completions  ·  ↑↓ history  ·  Tab complete"
		} else {
			hint = "  ask the AI anything — Tab switches back to shell mode"
		}
		return th.SuggestionDim.Render(hint)
	}

	var b strings.Builder

	if ghost != "" && m.suggest.Selected < 0 {
		// Inline ghost: show typed + dim suffix
		b.WriteString(th.SuggestionDim.Render("  ➜  "))
		b.WriteString(typed)
		b.WriteString(th.SuggestionHint.Render(ghost))
		b.WriteString(th.SuggestionDim.Render("  (→ accept)"))
	}

	// Chip list of matches
	if len(m.suggest.Matches) > 0 {
		b.WriteString(th.SuggestionDim.Render("  "))
		for i, match := range m.suggest.Matches {
			if i == m.suggest.Selected {
				b.WriteString(th.SuggestionSelectedSt.Render(match))
			} else {
				// Dim everything after the typed prefix
				typed := string(m.input)
				if strings.HasPrefix(match, typed) {
					suffix := match[len(typed):]
					b.WriteString(th.SuggestionDim.Render(typed))
					b.WriteString(th.SuggestionHint.Render(suffix))
				} else {
					b.WriteString(th.SuggestionDim.Render(match))
				}
			}
			if i < len(m.suggest.Matches)-1 {
				b.WriteString(th.SuggestionDim.Render("  ·  "))
			}
		}
	}

	return b.String()
}

// renderInputPanel builds the prompt + input box.
func (m appModel) renderInputPanel() string {
	th := m.theme
	innerW := m.mainWidth() - 2
	if innerW < 4 {
		innerW = 4
	}

	var promptStr string
	switch m.mode {
	case modeAI:
		promptStr = th.PromptAI.Render("? ")
	case modeAuto:
		promptStr = th.BadgeAuto.Render(" ⚡ ") + th.PromptShell.Render(" task> ")
	default:
		cwdPart := th.SuggestionDim.Render(shortPath(m.cwd))
		promptStr = cwdPart + th.PromptShell.Render(" $ ")
	}

	inputStr := m.renderInputRunes()
	content := promptStr + inputStr
	return th.InputLine.Width(innerW).Render(content)
}

// renderInputRunes renders the input rune slice with a block cursor.
func (m appModel) renderInputRunes() string {
	th := m.theme
	if len(m.input) == 0 {
		return th.Cursor.Render(" ")
	}
	var b strings.Builder
	for i, r := range m.input {
		ch := string(r)
		if i == m.cursor {
			b.WriteString(th.Cursor.Render(ch))
		} else {
			b.WriteString(ch)
		}
	}
	if m.cursor == len(m.input) {
		b.WriteString(th.Cursor.Render(" "))
	}
	return b.String()
}

// renderStatus renders the bottom hint / key-binding bar.
func (m appModel) renderStatus() string {
	th := m.theme
	parts := []string{
		"Tab: mode",
		"Ctrl+B: sessions",
		"Ctrl+N: new chat",
		"Ctrl+T: theme",
		"Ctrl+X: auto",
		"PgUp/Dn: scroll",
		"Ctrl+L: clear",
		"Ctrl+C: quit",
	}
	return th.StatusBar.Width(m.width).Render("  " + strings.Join(parts, "  │  "))
}

// ─── Layout helpers ───────────────────────────────────────────────────────────

// viewportHeight: total rows = header(1) + viewport-panel + suggest(1) + input-panel + status(1)
// viewport-panel height (with borders) = viewportHeight + 2
// input-panel height (with borders) = 1 + 2 = 3
// total = 1 + (vpH+2) + 1 + 3 + 1 = vpH + 8
func (m appModel) viewportHeight() int {
	h := m.height - 8
	if h < 2 {
		return 2
	}
	return h
}

func (m appModel) innerWidth() int {
	w := m.mainWidth() - 2
	if w < 10 {
		return 10
	}
	return w
}

// sidebarWidth returns the outer width of the sessions sidebar (0 = hidden).
func (m appModel) sidebarWidth() int {
	if m.width < 60 {
		return 0
	}
	return 26
}

// mainWidth returns the width available to the main chat area.
func (m appModel) mainWidth() int {
	return m.width - m.sidebarWidth()
}

// visibleLines returns the viewport-height slice of rendered lines.
func (m appModel) visibleLines(height int) []string {
	total := len(m.lines)
	maxOff := total - height
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.scrollOffset
	if off > maxOff {
		off = maxOff
	}
	end := total - off
	start := end - height
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}

	result := make([]string, height)
	src := m.lines[start:end]
	topPad := height - len(src)
	for i, l := range src {
		result[topPad+i] = l.text
	}
	return result
}

// ─── Line buffer ──────────────────────────────────────────────────────────────

func (m *appModel) pushMsg(msg models.Message) {
	m.messages = append(m.messages, msg)
	for _, l := range m.renderMsg(msg, m.innerWidth()) {
		m.lines = append(m.lines, l)
	}
}

func (m *appModel) rebuildLines() {
	m.lines = nil
	w := m.innerWidth()
	for _, msg := range m.messages {
		for _, l := range m.renderMsg(msg, w) {
			m.lines = append(m.lines, l)
		}
	}
}

func (m appModel) renderMsg(msg models.Message, width int) []styledLine {
	th := m.theme
	var prefix string
	var st lipgloss.Style

	switch msg.Role {
	case models.RoleUser:
		prefix, st = "? ", th.MsgUser
	case models.RoleAI:
		prefix, st = "✦ ", th.MsgAI
	case models.RoleShellCmd:
		prefix, st = "", th.MsgShellCmd
	case models.RoleShellOut:
		prefix, st = "  ", th.MsgShellOut
	case models.RoleError:
		prefix, st = "✗ ", th.MsgError
	case models.RoleAutoTask:
		prefix, st = "", th.MsgAutoTask
	case models.RoleAutoStep:
		prefix, st = "", th.MsgAutoStep
	case models.RoleAutoOut:
		prefix, st = "  ", th.MsgAutoOut
	case models.RoleAutoDone:
		prefix, st = "", th.MsgAutoDone
	default:
		prefix, st = "· ", th.MsgSystem
	}

	indent := strings.Repeat(" ", len([]rune(prefix)))
	rawLines := wrapText(msg.Content, width-len([]rune(prefix)))

	out := make([]styledLine, 0, len(rawLines)+1)
	for i, line := range rawLines {
		pfx := prefix
		if i > 0 {
			pfx = indent
		}
		out = append(out, styledLine{text: st.Render(pfx + line)})
	}
	if msg.Role == models.RoleAI || msg.Role == models.RoleAutoDone {
		out = append(out, styledLine{})
	}
	return out
}

// ─── Text utilities ───────────────────────────────────────────────────────────

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		out = append(out, wordWrap(raw, width)...)
	}
	return out
}

func wordWrap(line string, width int) []string {
	runes := []rune(line)
	if len(runes) <= width {
		if len(runes) == 0 {
			return []string{""}
		}
		return []string{line}
	}
	var out []string
	for len(runes) > width {
		bp := -1
		for i := width - 1; i >= width/2; i-- {
			if runes[i] == ' ' {
				bp = i
				break
			}
		}
		if bp < 0 {
			bp = width
		}
		out = append(out, string(runes[:bp]))
		runes = runes[bp:]
		for len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}
	if len(runes) > 0 {
		out = append(out, string(runes))
	}
	return out
}

func shortPath(path string) string {
	home := os.Getenv("HOME")
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─── Sidebar rendering ────────────────────────────────────────────────────────

// renderSidebar renders the sessions panel on the left side.
func (m appModel) renderSidebar() string {
	th := m.theme
	sW := m.sidebarWidth()
	sH := m.height - 2 // middle section height (minus header + status)
	innerW := sW - 2
	innerH := sH - 2

	var rows []string

	// Title row
	rows = append(rows, th.MsgAutoTask.Render("  🚀 Sessions"))
	rows = append(rows, th.SuggestionDim.Render(strings.Repeat("─", innerW)))

	// Session list (reserve 2 rows at bottom for footer hints)
	listH := innerH - 4 // title + separator = 2, footer = 2
	if listH < 1 {
		listH = 1
	}
	visStart := m.sidebarScroll
	visEnd := visStart + listH
	if visEnd > len(m.sessions) {
		visEnd = len(m.sessions)
	}
	for i := visStart; i < visEnd; i++ {
		sess := m.sessions[i]
		name := sess.Name
		maxLen := innerW - 5 // leave room for prefix + emoji
		if len([]rune(name)) > maxLen {
			runes := []rune(name)
			name = string(runes[:maxLen-1]) + "…"
		}
		isActive := sess.ID == m.activeSession
		isCursor := m.sidebarFocus && i == m.sessionCursor

		prefix := "  "
		if isActive {
			prefix = "▶ "
		}
		content := prefix + "💬 " + name

		var line string
		switch {
		case isCursor:
			line = th.SuggestionSelectedSt.Width(innerW).Render(content)
		case isActive:
			line = th.MsgAutoTask.Render(content)
		default:
			line = th.SuggestionDim.Render(content)
		}
		rows = append(rows, line)
	}

	// Fill empty rows to push footer to bottom
	for len(rows) < innerH-2 {
		rows = append(rows, "")
	}

	// Footer hints (change based on focus state)
	if m.sidebarFocus {
		rows = append(rows, th.SuggestionDim.Render("  ↑↓: nav  Enter: open"))
		rows = append(rows, th.SuggestionDim.Render("  Del: delete  Esc: back"))
	} else {
		rows = append(rows, th.SuggestionDim.Render("  Ctrl+B: focus"))
		rows = append(rows, th.SuggestionDim.Render("  Ctrl+N: new chat"))
	}

	// Pad each row to fill the inner width cleanly
	for i, row := range rows {
		w := lipgloss.Width(row)
		if w < innerW {
			rows[i] = row + strings.Repeat(" ", innerW-w)
		}
	}

	content := strings.Join(rows, "\n")

	var borderColor lipgloss.Color
	if m.sidebarFocus {
		borderColor = th.P.BorderFocused
	} else {
		borderColor = th.P.BorderNormal
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(content)
}

// ─── Session management ───────────────────────────────────────────────────────

// handleSidebarKey handles key events when the sidebar has focus.
func (m appModel) handleSidebarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.sessionCursor > 0 {
			m.sessionCursor--
			m.clampSidebarScroll()
		}
	case "down":
		if m.sessionCursor < len(m.sessions)-1 {
			m.sessionCursor++
			m.clampSidebarScroll()
		}
	case "enter":
		if m.sessionCursor >= 0 && m.sessionCursor < len(m.sessions) {
			return m.switchSession(m.sessions[m.sessionCursor].ID)
		}
	case "delete", "ctrl+d":
		if len(m.sessions) > 1 && m.sessionCursor < len(m.sessions) {
			return m.deleteSession(m.sessions[m.sessionCursor].ID)
		}
	case "ctrl+n":
		return m.createNewSession()
	case "esc", "ctrl+b":
		m.sidebarFocus = false
	}
	return m, nil
}

// switchSession loads and activates a named chat session.
func (m appModel) switchSession(id string) (appModel, tea.Cmd) {
	if id == m.activeSession {
		m.sidebarFocus = false
		return m, nil
	}
	m.activeSession = id
	m.sidebarFocus = false
	m.scrollOffset = 0

	if m.store != nil {
		_ = m.store.SaveActiveSession(id)
		msgs, err := m.store.LoadSessionMessages(id)
		if err == nil {
			m.messages = msgs
		} else {
			m.messages = nil
		}
	} else {
		m.messages = nil
	}

	m.rebuildLines()
	// Show which session we switched to
	for _, s := range m.sessions {
		if s.ID == id {
			m.pushMsg(models.New(models.RoleSystem,
				fmt.Sprintf("💬 Switched to: %s", s.Name)))
			break
		}
	}
	return m, nil
}

// deleteSession removes a session and switches away if it was the active one.
func (m appModel) deleteSession(id string) (appModel, tea.Cmd) {
	if m.store != nil {
		_ = m.store.DeleteSession(id)
	}
	for i, s := range m.sessions {
		if s.ID == id {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			break
		}
	}
	if m.sessionCursor >= len(m.sessions) {
		m.sessionCursor = len(m.sessions) - 1
	}
	if m.sessionCursor < 0 {
		m.sessionCursor = 0
	}
	if id == m.activeSession && len(m.sessions) > 0 {
		return m.switchSession(m.sessions[m.sessionCursor].ID)
	}
	return m, nil
}

// createNewSession creates a timestamped session and switches to it.
func (m appModel) createNewSession() (appModel, tea.Cmd) {
	name := fmt.Sprintf("Chat %s", time.Now().Format("Jan 02 15:04"))
	var sess storage.SessionMeta
	if m.store != nil {
		var err error
		sess, err = m.store.CreateSession(name)
		if err != nil {
			m.pushMsg(models.New(models.RoleError, "create session: "+err.Error()))
			return m, nil
		}
	} else {
		sess = storage.SessionMeta{
			ID:        fmt.Sprintf("local-%d", time.Now().UnixNano()),
			Name:      name,
			CreatedAt: time.Now(),
		}
	}
	m.sessions = append(m.sessions, sess)
	m.sessionCursor = len(m.sessions) - 1
	m.sidebarFocus = false
	return m.switchSession(sess.ID)
}

// maybeRenameSession auto-renames the active session based on its first input.
func (m *appModel) maybeRenameSession(input string) {
	if m.activeSession == "" {
		return
	}
	for i, s := range m.sessions {
		if s.ID != m.activeSession {
			continue
		}
		// Only rename if session has no non-system messages yet
		nonSystem := 0
		for _, msg := range m.messages {
			if msg.Role != models.RoleSystem {
				nonSystem++
			}
		}
		if nonSystem > 0 {
			return
		}
		name := []rune(strings.TrimSpace(input))
		if len(name) > 22 {
			name = append(name[:21], '…')
		}
		m.sessions[i].Name = string(name)
		if m.store != nil {
			_ = m.store.RenameSession(m.activeSession, m.sessions[i].Name)
		}
		return
	}
}

// clampSidebarScroll keeps sessionCursor visible in the session list.
func (m *appModel) clampSidebarScroll() {
	sH := m.height - 2
	innerH := sH - 2
	listH := innerH - 4
	if listH < 1 {
		listH = 1
	}
	if m.sessionCursor < m.sidebarScroll {
		m.sidebarScroll = m.sessionCursor
	}
	if m.sessionCursor >= m.sidebarScroll+listH {
		m.sidebarScroll = m.sessionCursor - listH + 1
	}
	if m.sidebarScroll < 0 {
		m.sidebarScroll = 0
	}
}

// ─── Entry point ──────────────────────────────────────────────────────────────

func main() {
	// Detect which provider to use based on available environment variables.
	providerCfg, err := agents.DetectProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		fmt.Fprintln(os.Stderr, "\nSupported providers (set any one API key):")
		fmt.Fprintln(os.Stderr, "  Google Gemini  — export GOOGLE_API_KEY=<key>")
		fmt.Fprintln(os.Stderr, "  OpenAI         — export OPENAI_API_KEY=<key>")
		fmt.Fprintln(os.Stderr, "  Anthropic      — export ANTHROPIC_API_KEY=<key>")
		fmt.Fprintln(os.Stderr, "\nOptional overrides:")
		fmt.Fprintln(os.Stderr, "  export PROVIDER=<google|openai|anthropic>")
		fmt.Fprintln(os.Stderr, "  export MODEL=<model-name>")
		os.Exit(1)
	}

	ctx := context.Background()
	ag, err := agents.New(ctx, providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise AI agent (%s): %v\n",
			agents.ProviderDisplay(providerCfg.Kind), err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Using provider: %s (model: %s)\n",
		agents.ProviderDisplay(providerCfg.Kind), providerCfg.DefaultModel())

	// Open (or create) the BoltDB session store — non-fatal if it fails.
	st, storeErr := storage.Open()
	if storeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: session store unavailable: %v\n", storeErr)
	}
	if st != nil {
		defer st.Close()
	}

	cwd, _ := os.Getwd()
	p := tea.NewProgram(initialModel(ag, cwd, providerCfg, st))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
