// Package agents provides an AI agent backed by Google Gemini (via the ADK)
// that can answer questions and execute shell commands as tools.
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

const (
	appName = "ai-shell"
	userID  = "local-user"

	systemInstruction = `You are an expert Linux system administrator and shell assistant embedded in an AI-powered terminal (AI Shell).

Your role:
- Suggest and explain shell commands concisely
- Execute commands using execute_shell_command when asked to run something
- Interpret command output and explain results clearly
- Help debug errors and system issues
- Warn about destructive operations (e.g. rm -rf) before executing them

Response style (this is a terminal UI, not a chat app):
- Keep replies short and to the point
- Put commands in backtick blocks: ` + "`command`" + `
- After executing a command, summarise the key result in one sentence
- If a command fails, explain why and suggest a fix`
)

// ShellAgent wraps a Google ADK runner with a Gemini-backed agent
// that has shell-execution capabilities.
type ShellAgent struct {
	r         *runner.Runner
	sessionID string
}

// New creates a ShellAgent using the provided Gemini API key.
func New(ctx context.Context, apiKey string) (*ShellAgent, error) {
	llm, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("creating gemini model: %w", err)
	}

	shellTool, err := tools.NewShellTool()
	if err != nil {
		return nil, fmt.Errorf("creating shell tool: %w", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "shell_assistant",
		Model:       llm,
		Description: "A Linux shell assistant that can suggest and execute commands.",
		Instruction: systemInstruction,
		Tools:       []tool.Tool{shellTool},
	})
	if err != nil {
		return nil, fmt.Errorf("creating llm agent: %w", err)
	}

	sessionSvc := session.InMemoryService()
	createResp, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          a,
		SessionService: sessionSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("creating runner: %w", err)
	}

	return &ShellAgent{
		r:         r,
		sessionID: createResp.Session.ID(),
	}, nil
}

// Ask sends a prompt to the AI agent and returns the final text response.
// It blocks until the agent completes (including any tool calls).
func (sa *ShellAgent) Ask(ctx context.Context, prompt string) (string, error) {
	content := genai.NewContentFromText(prompt, genai.RoleUser)

	var parts []string
	for event, err := range sa.r.Run(ctx, userID, sa.sessionID, content, agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}) {
		if err != nil {
			return "", fmt.Errorf("agent run error: %w", err)
		}
		if event == nil || !event.IsFinalResponse() {
			continue
		}
		if event.Content == nil {
			continue
		}
		for _, p := range event.Content.Parts {
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
	}

	if len(parts) == 0 {
		return "(no response)", nil
	}
	return strings.Join(parts, ""), nil
}
