// Package tools provides utilities for executing shell commands.
package tools

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

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
