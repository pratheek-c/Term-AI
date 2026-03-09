// Package tools provides utilities for executing shell commands and an ADK
// function tool that exposes shell execution to the LLM agent.
package tools

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ShellInput is the argument schema for the execute_shell_command tool.
type ShellInput struct {
	Command string `json:"command" description:"The bash command to execute on the system"`
}

// ShellOutput is the result schema for the execute_shell_command tool.
type ShellOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// ExecCommand runs a bash command in the given directory (empty = inherit cwd)
// and returns stdout, stderr, and the exit code.
func ExecCommand(dir, command string) (stdout, stderr string, exitCode int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// NewShellTool creates an ADK FunctionTool that lets the LLM execute shell
// commands and inspect their output.
func NewShellTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "execute_shell_command",
		Description: `Execute a bash shell command on the user's system and return stdout, stderr, and exit code.
Use this to: run commands, inspect files, check system status, list directories, install packages, or perform any terminal operation.
Examples: "ls -la", "cat /etc/os-release", "df -h", "ps aux | grep python", "uname -a"`,
	}, func(_ tool.Context, input ShellInput) (ShellOutput, error) {
		stdout, stderr, exitCode := ExecCommand("", input.Command)
		return ShellOutput{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
		}, nil
	})
}
