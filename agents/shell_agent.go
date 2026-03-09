// Package agents provides AI agents backed by configurable LLM providers.
//
// Supported providers: Google Gemini, OpenAI, Anthropic Claude.
// Provider selection is automatic based on available environment variables,
// or can be forced with the PROVIDER env var.
package agents

import (
	"context"
	"fmt"
	"strings"

	"tui-start/tools"
)

// shellToolSpec describes the execute_shell_command tool in terms of our
// provider-agnostic ToolSpec type.
var shellToolSpec = ToolSpec{
	Name: "execute_shell_command",
	Description: `Execute a bash shell command on the user's system and return stdout, stderr, and exit code.
Use this to: run commands, inspect files, check system status, list directories, install packages, or perform any terminal operation.
Examples: "ls -la", "cat /etc/os-release", "df -h", "ps aux | grep python", "uname -a"`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute on the system",
			},
		},
		"required": []string{"command"},
	},
}

const systemInstruction = `You are an expert Linux system administrator and shell assistant embedded in an AI-powered terminal (AI Shell).

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

// ShellAgent wraps an LLMClient and provides shell-execution capabilities.
type ShellAgent struct {
	llm LLMClient
}

// New creates a ShellAgent using the given ProviderConfig.
func New(ctx context.Context, cfg ProviderConfig) (*ShellAgent, error) {
	llm, err := NewLLMClient(ctx, cfg, systemInstruction)
	if err != nil {
		return nil, fmt.Errorf("creating LLM client (%s): %w", cfg.Kind, err)
	}
	return &ShellAgent{llm: llm}, nil
}

// NewShellAgentWithLLM creates a ShellAgent with a pre-built LLMClient.
// This is intended for testing, where a mock LLMClient can be injected.
func NewShellAgentWithLLM(llm LLMClient) *ShellAgent {
	return &ShellAgent{llm: llm}
}

// Ask sends a prompt to the AI agent and returns the final text response.
// The agent may execute shell commands as part of answering.
func (sa *ShellAgent) Ask(ctx context.Context, prompt string) (string, error) {
	executor := func(call ToolCall) (string, error) {
		cmd, _ := call.Args["command"].(string)
		stdout, stderr, exitCode := tools.ExecCommand("", cmd)
		parts := []string{}
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

	return sa.llm.ChatWithTools(ctx, prompt, []ToolSpec{shellToolSpec}, executor, nil, nil)
}
