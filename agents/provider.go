// Package agents — provider abstraction for LLM backends.
//
// Adding a new provider:
//  1. Implement the LLMClient interface.
//  2. Add a ProviderKind constant.
//  3. Register it in NewLLMClient.
package agents

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ─── ProviderKind ─────────────────────────────────────────────────────────────

// ProviderKind identifies which LLM backend to use.
type ProviderKind string

const (
	ProviderGoogle    ProviderKind = "google"
	ProviderOpenAI    ProviderKind = "openai"
	ProviderAnthropic ProviderKind = "anthropic"
)

// ─── ToolCall / ToolResult ────────────────────────────────────────────────────

// ToolCall is a request from the model to invoke a tool.
type ToolCall struct {
	// ID is an opaque correlation key the model may supply (used by OpenAI).
	ID string
	// Name is the tool/function name.
	Name string
	// Args is the JSON-decoded argument map.
	Args map[string]any
}

// ToolResult is the response to a ToolCall that is fed back to the model.
type ToolResult struct {
	// ID must match ToolCall.ID (used by OpenAI).
	ID string
	// Name is the tool/function name.
	Name string
	// Output is the string output produced by the tool.
	Output string
}

// ─── LLMClient ────────────────────────────────────────────────────────────────

// LLMClient is a provider-agnostic interface for LLM interactions.
// Both ShellAgent and AutoAgent use this interface so that the rest of the
// code is decoupled from the concrete backend.
type LLMClient interface {
	// Chat sends a single user message and returns the assistant text.
	// The client maintains its own conversation history.
	Chat(ctx context.Context, userMsg string) (string, error)

	// ChatWithTools sends a user message and executes the agentic tool-call
	// loop.  For each tool invocation, executor is called; its output is fed
	// back to the model.  onPlan / onOutput are optional hooks so the caller
	// can stream intermediate progress.
	//
	// Returns the final text response once the model stops calling tools.
	ChatWithTools(
		ctx context.Context,
		userMsg string,
		tools []ToolSpec,
		executor func(call ToolCall) (string, error),
		onPlan func(call ToolCall),
		onOutput func(output string),
	) (string, error)

	// Reset clears the conversation history, allowing reuse for a new task.
	Reset()
}

// ToolSpec describes a callable tool that the model can invoke.
type ToolSpec struct {
	Name        string
	Description string
	// Parameters is a JSON-Schema object describing the function arguments.
	Parameters map[string]any
}

// ─── Config ───────────────────────────────────────────────────────────────────

// ProviderConfig holds the configuration for a provider.
type ProviderConfig struct {
	Kind   ProviderKind
	APIKey string
	Model  string
}

// DefaultModel returns the sensible default model for each provider.
func (c ProviderConfig) DefaultModel() string {
	if c.Model != "" {
		return c.Model
	}
	switch c.Kind {
	case ProviderGoogle:
		return "gemini-2.5-flash"
	case ProviderOpenAI:
		return "gpt-4o"
	case ProviderAnthropic:
		return "claude-3-5-sonnet-20241022"
	default:
		return ""
	}
}

// ─── Factory ──────────────────────────────────────────────────────────────────

// NewLLMClient constructs the LLMClient for the given config.
func NewLLMClient(ctx context.Context, cfg ProviderConfig, systemPrompt string) (LLMClient, error) {
	switch cfg.Kind {
	case ProviderGoogle:
		return newGeminiClient(ctx, cfg, systemPrompt)
	case ProviderOpenAI:
		return newOpenAIClient(ctx, cfg, systemPrompt)
	case ProviderAnthropic:
		return newAnthropicClient(ctx, cfg, systemPrompt)
	default:
		return nil, fmt.Errorf("unknown provider: %q", cfg.Kind)
	}
}

// ─── Auto-detect ──────────────────────────────────────────────────────────────

// DetectProvider inspects environment variables and returns a ready-to-use
// ProviderConfig.  Priority: GOOGLE_API_KEY → OPENAI_API_KEY → ANTHROPIC_API_KEY.
// An explicit PROVIDER env var overrides the priority.
func DetectProvider() (ProviderConfig, error) {
	// Allow explicit override.
	if explicit := strings.ToLower(os.Getenv("PROVIDER")); explicit != "" {
		switch explicit {
		case "google", "gemini":
			key := os.Getenv("GOOGLE_API_KEY")
			if key == "" {
				return ProviderConfig{}, fmt.Errorf("PROVIDER=google but GOOGLE_API_KEY is not set")
			}
			return ProviderConfig{Kind: ProviderGoogle, APIKey: key, Model: os.Getenv("MODEL")}, nil
		case "openai":
			key := os.Getenv("OPENAI_API_KEY")
			if key == "" {
				return ProviderConfig{}, fmt.Errorf("PROVIDER=openai but OPENAI_API_KEY is not set")
			}
			return ProviderConfig{Kind: ProviderOpenAI, APIKey: key, Model: os.Getenv("MODEL")}, nil
		case "anthropic", "claude":
			key := os.Getenv("ANTHROPIC_API_KEY")
			if key == "" {
				return ProviderConfig{}, fmt.Errorf("PROVIDER=anthropic but ANTHROPIC_API_KEY is not set")
			}
			return ProviderConfig{Kind: ProviderAnthropic, APIKey: key, Model: os.Getenv("MODEL")}, nil
		default:
			return ProviderConfig{}, fmt.Errorf("unknown PROVIDER %q (valid: google, openai, anthropic)", explicit)
		}
	}

	// Auto-detect by available keys.
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		return ProviderConfig{Kind: ProviderGoogle, APIKey: key, Model: os.Getenv("MODEL")}, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return ProviderConfig{Kind: ProviderOpenAI, APIKey: key, Model: os.Getenv("MODEL")}, nil
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return ProviderConfig{Kind: ProviderAnthropic, APIKey: key, Model: os.Getenv("MODEL")}, nil
	}

	return ProviderConfig{}, fmt.Errorf(
		"no API key found — set one of: GOOGLE_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY\n" +
			"Optionally set PROVIDER=<google|openai|anthropic> to force a specific provider.\n" +
			"Optionally set MODEL=<model-name> to override the default model.")
}

// ProviderDisplay returns a short human-readable label for a provider.
func ProviderDisplay(kind ProviderKind) string {
	switch kind {
	case ProviderGoogle:
		return "Google Gemini"
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic Claude"
	default:
		return string(kind)
	}
}
