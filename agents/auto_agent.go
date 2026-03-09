package agents

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"tui-start/tools"
)

// ─── Auto-execute step types ──────────────────────────────────────────────────

// AutoStep is one step emitted by the AutoAgent while executing a task.
// Steps are sent over a channel so the TUI can stream progress in real-time.
type AutoStep struct {
	// Kind is the category of the step.
	Kind AutoStepKind
	// Text carries the human-readable content of the step.
	Text string
	// Err is non-nil when Kind == AutoStepError.
	Err error
}

// AutoStepKind classifies an AutoStep.
type AutoStepKind int

const (
	AutoStepPlan   AutoStepKind = iota // agent announces the next command to run
	AutoStepOutput                     // stdout/stderr captured from a command
	AutoStepInfo                       // informational message from the agent
	AutoStepDone                       // task complete, Text is the final summary
	AutoStepError                      // unrecoverable error
)

// ─── Auto agent ───────────────────────────────────────────────────────────────

const autoSystemInstruction = `You are an autonomous Linux task executor embedded in an AI Shell TUI.

The user gives you a high-level task (e.g. "find all .log files larger than 1 MB and compress them").
You must decompose the task into concrete shell commands and execute them one at a time using the
execute_shell_command tool.

Rules:
1. Think step-by-step. Before each command, state PLAN: <what you are about to do>.
2. After each command, examine the output and decide if the task is done or if another step is needed.
3. When the task is fully complete, state DONE: <short summary of what was accomplished>.
4. If you encounter an unrecoverable error, state ERROR: <explanation>.
5. Never execute destructive commands (rm -rf /, format disks, etc.) without making them obviously safe.
6. Stay focused on the task — do not execute unrelated commands.
7. Keep every PLAN/DONE/ERROR statement on its own line.
8. After outputting DONE or ERROR, stop — do not send any further messages.`

const (
	autoAppName = "ai-shell-auto"
	maxSteps    = 30 // safety ceiling — stop after this many tool calls
)

// AutoAgent runs a sequence of shell commands to complete a task autonomously.
// It sends progress updates over the Steps channel until the task is done or
// an error occurs; then it closes the channel.
type AutoAgent struct {
	apiKey string
	cwd    string // working directory for all commands
}

// NewAutoAgent creates an AutoAgent for the given working directory.
func NewAutoAgent(apiKey, cwd string) *AutoAgent {
	return &AutoAgent{apiKey: apiKey, cwd: cwd}
}

// SetCwd updates the working directory used for command execution.
func (a *AutoAgent) SetCwd(cwd string) { a.cwd = cwd }

// Run executes the given task description asynchronously.
// It returns a read-only channel; callers should range over it until it is
// closed.  The channel is always closed by the time Run returns.
func (a *AutoAgent) Run(ctx context.Context, task string) <-chan AutoStep {
	ch := make(chan AutoStep, 32)
	go func() {
		defer close(ch)
		if err := a.run(ctx, task, ch); err != nil {
			ch <- AutoStep{Kind: AutoStepError, Err: err}
		}
	}()
	return ch
}

// run contains the main loop. It creates a fresh ADK session per task so
// previous context does not bleed across tasks.
func (a *AutoAgent) run(ctx context.Context, task string, ch chan<- AutoStep) error {
	// ── Build model ───────────────────────────────────────────────────────
	llm, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: a.apiKey,
	})
	if err != nil {
		return fmt.Errorf("creating gemini model: %w", err)
	}

	// ── Build a shell tool bound to the current working directory ─────────
	shellTool, err := tools.NewShellTool()
	if err != nil {
		return fmt.Errorf("creating shell tool: %w", err)
	}

	// ── Build the agent ───────────────────────────────────────────────────
	ag, err := llmagent.New(llmagent.Config{
		Name:        "auto_executor",
		Model:       llm,
		Description: "Autonomous shell task executor.",
		Instruction: autoSystemInstruction,
		Tools:       []tool.Tool{shellTool},
	})
	if err != nil {
		return fmt.Errorf("creating llm agent: %w", err)
	}

	// ── Fresh session ─────────────────────────────────────────────────────
	sessionSvc := session.InMemoryService()
	createResp, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: autoAppName,
		UserID:  userID,
	})
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        autoAppName,
		Agent:          ag,
		SessionService: sessionSvc,
	})
	if err != nil {
		return fmt.Errorf("creating runner: %w", err)
	}

	sessionID := createResp.Session.ID()

	// ── Send the task with CWD context ────────────────────────────────────
	initialPrompt := fmt.Sprintf(
		"Working directory: %s\n\nTask: %s", a.cwd, task)
	content := genai.NewContentFromText(initialPrompt, genai.RoleUser)

	steps := 0
	currentPrompt := content

	for {
		if steps >= maxSteps {
			ch <- AutoStep{
				Kind: AutoStepError,
				Err:  fmt.Errorf("safety limit reached: %d steps executed without DONE", maxSteps),
			}
			return nil
		}

		var responseLines []string
		isDone := false
		isError := false

		for event, err := range r.Run(ctx, userID, sessionID, currentPrompt, agent.RunConfig{
			StreamingMode: agent.StreamingModeNone,
		}) {
			if err != nil {
				return fmt.Errorf("agent run error: %w", err)
			}
			if event == nil {
				continue
			}

			// Capture tool call descriptions (the commands being run)
			if event.Content != nil {
				for _, p := range event.Content.Parts {
					if p.FunctionCall != nil {
						ch <- AutoStep{
							Kind: AutoStepPlan,
							Text: fmt.Sprintf("$ %s", extractCommand(p.FunctionCall.Args)),
						}
						steps++
					}
					if p.FunctionResponse != nil {
						out := extractToolOutput(p.FunctionResponse.Response)
						if out != "" {
							ch <- AutoStep{Kind: AutoStepOutput, Text: out}
						}
					}
				}
			}

			if !event.IsFinalResponse() {
				continue
			}
			if event.Content == nil {
				continue
			}
			for _, p := range event.Content.Parts {
				if p.Text != "" {
					responseLines = append(responseLines, p.Text)
				}
			}
		}

		fullResponse := strings.Join(responseLines, "")

		// Parse PLAN / DONE / ERROR directives
		for _, line := range strings.Split(fullResponse, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "PLAN:"):
				ch <- AutoStep{Kind: AutoStepPlan, Text: strings.TrimSpace(line[5:])}
			case strings.HasPrefix(line, "DONE:"):
				ch <- AutoStep{Kind: AutoStepDone, Text: strings.TrimSpace(line[5:])}
				isDone = true
			case strings.HasPrefix(line, "ERROR:"):
				ch <- AutoStep{
					Kind: AutoStepError,
					Err:  fmt.Errorf("%s", strings.TrimSpace(line[6:])),
				}
				isError = true
			default:
				if line != "" {
					ch <- AutoStep{Kind: AutoStepInfo, Text: line}
				}
			}
		}

		if isDone || isError {
			return nil
		}

		// If the agent finished but hasn't said DONE, nudge it to continue or wrap up
		if fullResponse == "" {
			return nil
		}
		currentPrompt = genai.NewContentFromText("Continue with the next step, or say DONE if the task is complete.", genai.RoleUser)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// extractCommand pulls the command string out of a FunctionCall args map.
func extractCommand(args map[string]any) string {
	if v, ok := args["command"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	// Fallback: marshal all args
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}

// extractToolOutput converts a function-response map to a readable string.
func extractToolOutput(resp map[string]any) string {
	var lines []string
	if stdout, ok := resp["stdout"].(string); ok && strings.TrimSpace(stdout) != "" {
		lines = append(lines, strings.TrimRight(stdout, "\n"))
	}
	if stderr, ok := resp["stderr"].(string); ok && strings.TrimSpace(stderr) != "" {
		lines = append(lines, "stderr: "+strings.TrimRight(stderr, "\n"))
	}
	if code, ok := resp["exit_code"].(float64); ok && int(code) != 0 {
		lines = append(lines, fmt.Sprintf("exit: %d", int(code)))
	}
	return strings.Join(lines, "\n")
}
