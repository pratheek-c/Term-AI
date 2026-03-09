package tuistart_test

import (
	"os/exec"
	"strings"
	"testing"

	"tui-start/tools"
)

// requireBash skips the test if bash is not available on PATH.
func requireBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found on PATH; skipping shell tests")
	}
}

func TestExecCommand_SimpleEcho(t *testing.T) {
	requireBash(t)
	stdout, stderr, code := tools.ExecCommand("", "echo hello")
	if !strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, want to contain %q", stdout, "hello")
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestExecCommand_ExitCode_NonZero(t *testing.T) {
	requireBash(t)
	_, _, code := tools.ExecCommand("", "exit 42")
	if code != 42 {
		t.Errorf("exit code = %d, want 42", code)
	}
}

func TestExecCommand_Stderr(t *testing.T) {
	requireBash(t)
	_, stderr, code := tools.ExecCommand("", "echo errtext >&2; exit 1")
	if !strings.Contains(stderr, "errtext") {
		t.Errorf("stderr = %q, want to contain %q", stderr, "errtext")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestExecCommand_BothStreams(t *testing.T) {
	requireBash(t)
	stdout, stderr, code := tools.ExecCommand("", "echo out; echo err >&2")
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout = %q, want to contain %q", stdout, "out")
	}
	if !strings.Contains(stderr, "err") {
		t.Errorf("stderr = %q, want to contain %q", stderr, "err")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestExecCommand_WorkingDirectory(t *testing.T) {
	requireBash(t)
	dir := t.TempDir()
	stdout, _, code := tools.ExecCommand(dir, "pwd")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("expected non-empty stdout from pwd")
	}
}

func TestExecCommand_EmptyDir_InheritsProcess(t *testing.T) {
	requireBash(t)
	stdout, _, code := tools.ExecCommand("", "pwd")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("expected non-empty stdout")
	}
}

func TestExecCommand_MultiLineOutput(t *testing.T) {
	requireBash(t)
	stdout, _, code := tools.ExecCommand("", "printf 'a\nb\nc\n'")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}
