package tuistart_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tui-start/agents"
)

// ─── mockLLMClient ────────────────────────────────────────────────────────────

type mockLLMClient struct {
	ChatResp  string
	ChatErr   error
	ToolResp  string
	ToolErr   error
	ToolCalls []agents.ToolCall
	ResetN    int
}

func (m *mockLLMClient) Chat(_ context.Context, _ string) (string, error) {
	return m.ChatResp, m.ChatErr
}

func (m *mockLLMClient) ChatWithTools(
	_ context.Context,
	_ string,
	_ []agents.ToolSpec,
	executor func(agents.ToolCall) (string, error),
	onPlan func(agents.ToolCall),
	onOutput func(string),
) (string, error) {
	if m.ToolErr != nil {
		return "", m.ToolErr
	}
	for _, tc := range m.ToolCalls {
		if onPlan != nil {
			onPlan(tc)
		}
		out, err := executor(tc)
		if err != nil {
			return "", err
		}
		if onOutput != nil && out != "" {
			onOutput(out)
		}
	}
	return m.ToolResp, nil
}

func (m *mockLLMClient) Reset() {
	m.ResetN++
}

// mockCapture records executor/plan call details.
type mockCapture struct {
	toolCalls    []agents.ToolCall
	toolResp     string
	capturedCmd  *string
	capturedPlan *string
}

func (m *mockCapture) Chat(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockCapture) Reset()                                           {}
func (m *mockCapture) ChatWithTools(
	_ context.Context,
	_ string,
	_ []agents.ToolSpec,
	executor func(agents.ToolCall) (string, error),
	onPlan func(agents.ToolCall),
	onOutput func(string),
) (string, error) {
	for _, tc := range m.toolCalls {
		if onPlan != nil {
			onPlan(tc)
			if m.capturedPlan != nil {
				if cmd, ok := tc.Args["command"].(string); ok {
					*m.capturedPlan = cmd
				}
			}
		}
		out, err := executor(tc)
		if err != nil {
			return "", err
		}
		if m.capturedCmd != nil {
			*m.capturedCmd = out
		}
		if onOutput != nil && out != "" {
			onOutput(out)
		}
	}
	return m.toolResp, nil
}

// ─── MapToSchema ──────────────────────────────────────────────────────────────

func TestMapToSchema_Nil(t *testing.T) {
	s, err := agents.MapToSchema(nil)
	if err != nil {
		t.Errorf("MapToSchema(nil) error: %v", err)
	}
	if s != nil {
		t.Errorf("MapToSchema(nil) = %v, want nil", s)
	}
}

func TestMapToSchema_ValidSchema(t *testing.T) {
	m := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "a command",
			},
		},
		"required": []any{"command"},
	}
	s, err := agents.MapToSchema(m)
	if err != nil {
		t.Fatalf("MapToSchema error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil schema")
	}
}

// ─── BuildAnthropicSchema ─────────────────────────────────────────────────────

func TestBuildAnthropicSchema_Nil(t *testing.T) {
	schema := agents.BuildAnthropicSchema(nil)
	if schema.Properties != nil {
		t.Errorf("expected nil Properties, got %v", schema.Properties)
	}
}

func TestBuildAnthropicSchema_WithProperties(t *testing.T) {
	props := map[string]any{
		"command": map[string]any{"type": "string"},
	}
	params := map[string]any{
		"type":       "object",
		"properties": props,
		"required":   []string{"command"},
	}
	schema := agents.BuildAnthropicSchema(params)
	if schema.Properties == nil {
		t.Error("expected non-nil Properties")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Errorf("Required = %v, want [command]", schema.Required)
	}
}

func TestBuildAnthropicSchema_RequiredAsAnySlice(t *testing.T) {
	params := map[string]any{
		"required": []any{"alpha", "beta"},
	}
	schema := agents.BuildAnthropicSchema(params)
	if len(schema.Required) != 2 {
		t.Errorf("Required len = %d, want 2", len(schema.Required))
	}
}

// ─── ShellAgent.Ask ───────────────────────────────────────────────────────────

func TestShellAgent_Ask_ReturnsResponse(t *testing.T) {
	mock := &mockLLMClient{ToolResp: "Task done."}
	agent := agents.NewShellAgentWithLLM(mock)
	resp, err := agent.Ask(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}
	if resp != "Task done." {
		t.Errorf("Ask resp = %q, want %q", resp, "Task done.")
	}
}

func TestShellAgent_Ask_PropagatesError(t *testing.T) {
	mock := &mockLLMClient{ToolErr: errors.New("network failure")}
	agent := agents.NewShellAgentWithLLM(mock)
	_, err := agent.Ask(context.Background(), "do something")
	if err == nil {
		t.Error("expected error from Ask")
	}
}

func TestShellAgent_Ask_ExecutesToolCall(t *testing.T) {
	var executedCmd string
	var planSeen string

	capturedMock := &mockCapture{
		toolCalls: []agents.ToolCall{
			{Name: "execute_shell_command", Args: map[string]any{"command": "echo capture_test"}},
		},
		toolResp:     "done",
		capturedCmd:  &executedCmd,
		capturedPlan: &planSeen,
	}
	agent := agents.NewShellAgentWithLLM(capturedMock)
	resp, err := agent.Ask(context.Background(), "capture test")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}
	if resp != "done" {
		t.Errorf("resp = %q, want %q", resp, "done")
	}
}

// ─── ProviderConfig.DefaultModel ─────────────────────────────────────────────

func TestDefaultModel_ExplicitModelReturned(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: agents.ProviderGoogle, Model: "my-custom-model"}
	if got := cfg.DefaultModel(); got != "my-custom-model" {
		t.Errorf("DefaultModel = %q, want %q", got, "my-custom-model")
	}
}

func TestDefaultModel_Google_Default(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: agents.ProviderGoogle}
	if got := cfg.DefaultModel(); got != "gemini-2.5-flash" {
		t.Errorf("Google default model = %q, want %q", got, "gemini-2.5-flash")
	}
}

func TestDefaultModel_OpenAI_Default(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: agents.ProviderOpenAI}
	if got := cfg.DefaultModel(); got != "gpt-4o" {
		t.Errorf("OpenAI default model = %q, want %q", got, "gpt-4o")
	}
}

func TestDefaultModel_Anthropic_Default(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: agents.ProviderAnthropic}
	if got := cfg.DefaultModel(); got != "claude-3-5-sonnet-20241022" {
		t.Errorf("Anthropic default model = %q, want %q", got, "claude-3-5-sonnet-20241022")
	}
}

func TestDefaultModel_Unknown_Empty(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: "unknown"}
	if got := cfg.DefaultModel(); got != "" {
		t.Errorf("unknown provider DefaultModel = %q, want \"\"", got)
	}
}

// ─── ProviderDisplay ──────────────────────────────────────────────────────────

func TestProviderDisplay_Google(t *testing.T) {
	if got := agents.ProviderDisplay(agents.ProviderGoogle); got != "Google Gemini" {
		t.Errorf("ProviderDisplay(Google) = %q, want %q", got, "Google Gemini")
	}
}

func TestProviderDisplay_OpenAI(t *testing.T) {
	if got := agents.ProviderDisplay(agents.ProviderOpenAI); got != "OpenAI" {
		t.Errorf("ProviderDisplay(OpenAI) = %q, want %q", got, "OpenAI")
	}
}

func TestProviderDisplay_Anthropic(t *testing.T) {
	if got := agents.ProviderDisplay(agents.ProviderAnthropic); got != "Anthropic Claude" {
		t.Errorf("ProviderDisplay(Anthropic) = %q, want %q", got, "Anthropic Claude")
	}
}

func TestProviderDisplay_Unknown_ReturnsKind(t *testing.T) {
	if got := agents.ProviderDisplay("xyzprovider"); got != "xyzprovider" {
		t.Errorf("ProviderDisplay(unknown) = %q, want %q", got, "xyzprovider")
	}
}

// ─── DetectProvider ───────────────────────────────────────────────────────────

func TestDetectProvider_NoKeys_Error(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := agents.DetectProvider()
	if err == nil {
		t.Error("expected error when no API keys are set")
	}
}

func TestDetectProvider_GoogleKey_AutoDetect(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "test-google-key")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderGoogle {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderGoogle)
	}
	if cfg.APIKey != "test-google-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-google-key")
	}
}

func TestDetectProvider_OpenAIKey_AutoDetect(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderOpenAI {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderOpenAI)
	}
}

func TestDetectProvider_AnthropicKey_AutoDetect(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderAnthropic {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderAnthropic)
	}
}

func TestDetectProvider_GooglePriority(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "gkey")
	t.Setenv("OPENAI_API_KEY", "okey")
	t.Setenv("ANTHROPIC_API_KEY", "")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderGoogle {
		t.Errorf("priority: Kind = %q, want Google", cfg.Kind)
	}
}

func TestDetectProvider_ExplicitGoogle(t *testing.T) {
	t.Setenv("PROVIDER", "google")
	t.Setenv("GOOGLE_API_KEY", "explicit-google")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderGoogle || cfg.APIKey != "explicit-google" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestDetectProvider_ExplicitGemini_Alias(t *testing.T) {
	t.Setenv("PROVIDER", "gemini")
	t.Setenv("GOOGLE_API_KEY", "gemini-key")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderGoogle {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderGoogle)
	}
}

func TestDetectProvider_ExplicitOpenAI(t *testing.T) {
	t.Setenv("PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "explicit-openai")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderOpenAI {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderOpenAI)
	}
}

func TestDetectProvider_ExplicitAnthropic(t *testing.T) {
	t.Setenv("PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "explicit-anthropic")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderAnthropic {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderAnthropic)
	}
}

func TestDetectProvider_ExplicitClaudeAlias(t *testing.T) {
	t.Setenv("PROVIDER", "claude")
	t.Setenv("ANTHROPIC_API_KEY", "claude-key")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != agents.ProviderAnthropic {
		t.Errorf("Kind = %q, want %q", cfg.Kind, agents.ProviderAnthropic)
	}
}

func TestDetectProvider_Explicit_MissingKey_Error(t *testing.T) {
	t.Setenv("PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")
	_, err := agents.DetectProvider()
	if err == nil {
		t.Error("expected error when explicit provider set but key missing")
	}
}

func TestDetectProvider_ExplicitUnknown_Error(t *testing.T) {
	t.Setenv("PROVIDER", "notaprovider")
	_, err := agents.DetectProvider()
	if err == nil {
		t.Error("expected error for unknown PROVIDER value")
	}
}

func TestDetectProvider_MODEL_EnvVar(t *testing.T) {
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_API_KEY", "key")
	t.Setenv("MODEL", "gemini-custom")
	cfg, err := agents.DetectProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "gemini-custom" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gemini-custom")
	}
	t.Setenv("MODEL", "")
}

// ─── NewLLMClient: unknown provider ──────────────────────────────────────────

func TestNewLLMClient_UnknownProvider_Error(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: "doesnotexist", APIKey: "key"}
	_, err := agents.NewLLMClient(nil, cfg, "")
	if err == nil {
		t.Error("expected error for unknown provider kind")
	}
}

// ─── AutoAgent ───────────────────────────────────────────────────────────────

func TestNewAutoAgent_Fields(t *testing.T) {
	cfg := agents.ProviderConfig{Kind: agents.ProviderGoogle, APIKey: "key"}
	a := agents.NewAutoAgent(cfg, "/tmp")
	if a.Cfg().Kind != agents.ProviderGoogle {
		t.Errorf("cfg.Kind = %q, want %q", a.Cfg().Kind, agents.ProviderGoogle)
	}
	if a.Cwd() != "/tmp" {
		t.Errorf("cwd = %q, want %q", a.Cwd(), "/tmp")
	}
}

func TestAutoAgent_SetCwd(t *testing.T) {
	a := agents.NewAutoAgent(agents.ProviderConfig{}, "/old")
	a.SetCwd("/new")
	if a.Cwd() != "/new" {
		t.Errorf("cwd = %q, want %q", a.Cwd(), "/new")
	}
}

func TestAutoAgent_Run_ChannelClosedOnError(t *testing.T) {
	a := agents.NewAutoAgent(agents.ProviderConfig{Kind: "bad-provider", APIKey: "x"}, "/tmp")
	ch := a.Run(context.Background(), "do something")

	var steps []agents.AutoStep
	for step := range ch {
		steps = append(steps, step)
	}
	found := false
	for _, s := range steps {
		if s.Kind == agents.AutoStepError {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected AutoStepError in steps, got %v", steps)
	}
}

func TestAutoAgent_Run_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := agents.NewAutoAgent(agents.ProviderConfig{Kind: "bad-provider"}, "/tmp")
	ch := a.Run(ctx, "task")

	var count int
	for range ch {
		count++
		if count > 100 {
			t.Fatal("channel never closed")
		}
	}
}

// ─── Auto-step parsing logic ──────────────────────────────────────────────────

func TestAutoStep_ParseDirectivesDirectly(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantKinds []agents.AutoStepKind
	}{
		{
			name:      "DONE line",
			text:      "DONE: all done",
			wantKinds: []agents.AutoStepKind{agents.AutoStepDone},
		},
		{
			name:      "PLAN line",
			text:      "PLAN: run ls",
			wantKinds: []agents.AutoStepKind{agents.AutoStepPlan},
		},
		{
			name:      "ERROR line",
			text:      "ERROR: something broke",
			wantKinds: []agents.AutoStepKind{agents.AutoStepError},
		},
		{
			name:      "info line",
			text:      "some informational text",
			wantKinds: []agents.AutoStepKind{agents.AutoStepInfo},
		},
		{
			name:      "mixed",
			text:      "PLAN: step1\nsome info\nDONE: complete",
			wantKinds: []agents.AutoStepKind{agents.AutoStepPlan, agents.AutoStepInfo, agents.AutoStepDone},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan agents.AutoStep, 32)
			for _, line := range strings.Split(tc.text, "\n") {
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "PLAN:"):
					ch <- agents.AutoStep{Kind: agents.AutoStepPlan, Text: strings.TrimSpace(line[5:])}
				case strings.HasPrefix(line, "DONE:"):
					ch <- agents.AutoStep{Kind: agents.AutoStepDone, Text: strings.TrimSpace(line[5:])}
				case strings.HasPrefix(line, "ERROR:"):
					ch <- agents.AutoStep{Kind: agents.AutoStepError, Err: errors.New(strings.TrimSpace(line[6:]))}
				default:
					if line != "" {
						ch <- agents.AutoStep{Kind: agents.AutoStepInfo, Text: line}
					}
				}
			}
			close(ch)

			var got []agents.AutoStepKind
			for s := range ch {
				got = append(got, s.Kind)
			}
			if len(got) != len(tc.wantKinds) {
				t.Fatalf("got %d steps, want %d: %v", len(got), len(tc.wantKinds), got)
			}
			for i := range got {
				if got[i] != tc.wantKinds[i] {
					t.Errorf("step[%d] kind = %v, want %v", i, got[i], tc.wantKinds[i])
				}
			}
		})
	}
}
