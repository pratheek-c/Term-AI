package agents

import (
	"context"
	"fmt"
	"strings"

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

const maxSteps = 30 // safety ceiling — stop after this many tool calls

// AutoAgent runs a sequence of shell commands to complete a task autonomously.
// It sends progress updates over the Steps channel until the task is done or
// an error occurs; then it closes the channel.
type AutoAgent struct {
	cfg ProviderConfig
	cwd string // working directory for all commands
}

// NewAutoAgent creates an AutoAgent for the given provider config and working directory.
func NewAutoAgent(cfg ProviderConfig, cwd string) *AutoAgent {
	return &AutoAgent{cfg: cfg, cwd: cwd}
}

// SetCwd updates the working directory used for command execution.
func (a *AutoAgent) SetCwd(cwd string) { a.cwd = cwd }

// Cfg returns the ProviderConfig for this agent.
func (a *AutoAgent) Cfg() ProviderConfig { return a.cfg }

// Cwd returns the working directory for this agent.
func (a *AutoAgent) Cwd() string { return a.cwd }

// Run executes the given task description asynchronously.
// It returns a read-only channel; callers should range over it until it is closed.
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

// run contains the main loop.
func (a *AutoAgent) run(ctx context.Context, task string, ch chan<- AutoStep) error {
	// Create a fresh LLM client for this task so context doesn't bleed across tasks.
	llm, err := NewLLMClient(ctx, a.cfg, autoSystemInstruction)
	if err != nil {
		return fmt.Errorf("creating LLM client: %w", err)
	}

	cwd := a.cwd
	steps := 0

	// onPlan streams a plan step to the TUI.
	onPlan := func(call ToolCall) {
		cmd, _ := call.Args["command"].(string)
		ch <- AutoStep{Kind: AutoStepPlan, Text: "$ " + cmd}
		steps++
	}

	// onOutput streams tool output to the TUI.
	onOutput := func(output string) {
		if output != "" {
			ch <- AutoStep{Kind: AutoStepOutput, Text: output}
		}
	}

	// executor runs the shell command.
	executor := func(call ToolCall) (string, error) {
		if steps > maxSteps {
			return "", fmt.Errorf("safety limit reached: %d steps executed", maxSteps)
		}
		cmd, _ := call.Args["command"].(string)
		stdout, stderr, exitCode := tools.ExecCommand(cwd, cmd)
		var parts []string
		if strings.TrimSpace(stdout) != "" {
			parts = append(parts, strings.TrimRight(stdout, "\n"))
		}
		if strings.TrimSpace(stderr) != "" {
			parts = append(parts, "stderr: "+strings.TrimRight(stderr, "\n"))
		}
		if exitCode != 0 {
			parts = append(parts, fmt.Sprintf("exit: %d", exitCode))
		}
		return strings.Join(parts, "\n"), nil
	}

	initialPrompt := fmt.Sprintf("Working directory: %s\n\nTask: %s", cwd, task)

	finalText, err := llm.ChatWithTools(
		ctx,
		initialPrompt,
		[]ToolSpec{shellToolSpec},
		executor,
		onPlan,
		onOutput,
	)
	if err != nil {
		return fmt.Errorf("agent run error: %w", err)
	}

	// Parse PLAN / DONE / ERROR directives from the final response.
	isDone := false
	for _, line := range strings.Split(finalText, "\n") {
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
			return nil
		default:
			if line != "" {
				ch <- AutoStep{Kind: AutoStepInfo, Text: line}
			}
		}
	}

	if !isDone {
		// The model finished without an explicit DONE — treat it as done.
		ch <- AutoStep{Kind: AutoStepDone, Text: "Task complete."}
	}

	return nil
}
