package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

// geminiClient implements LLMClient using the google.golang.org/genai Chat API.
type geminiClient struct {
	client *genai.Client
	model  string
	sysMsg string

	// history is the accumulated conversation (excluding tool turns, which are
	// managed inside ChatWithTools).
	history []*genai.Content
}

func newGeminiClient(ctx context.Context, cfg ProviderConfig, systemPrompt string) (LLMClient, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: creating client: %w", err)
	}
	return &geminiClient{
		client: client,
		model:  cfg.DefaultModel(),
		sysMsg: systemPrompt,
	}, nil
}

// Chat sends a single user message maintaining multi-turn history.
func (g *geminiClient) Chat(ctx context.Context, userMsg string) (string, error) {
	chat, err := g.client.Chats.Create(ctx, g.model, &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(g.sysMsg, genai.RoleUser),
	}, g.history)
	if err != nil {
		return "", fmt.Errorf("gemini: creating chat: %w", err)
	}

	resp, err := chat.SendMessage(ctx, genai.Part{Text: userMsg})
	if err != nil {
		return "", fmt.Errorf("gemini: sending message: %w", err)
	}

	// Update history from the chat session so multi-turn context accumulates.
	g.history = chat.History(false)

	text := resp.Text()
	if text == "" {
		return "(no response)", nil
	}
	return text, nil
}

// ChatWithTools runs the tool-calling loop.
func (g *geminiClient) ChatWithTools(
	ctx context.Context,
	userMsg string,
	tools []ToolSpec,
	executor func(ToolCall) (string, error),
	onPlan func(ToolCall),
	onOutput func(string),
) (string, error) {
	// Build genai tool declarations.
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		schema, err := mapToSchema(t.Parameters)
		if err != nil {
			return "", fmt.Errorf("gemini: building schema for %s: %w", t.Name, err)
		}
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(g.sysMsg, genai.RoleUser),
		Tools: []*genai.Tool{
			{FunctionDeclarations: decls},
		},
	}

	chat, err := g.client.Chats.Create(ctx, g.model, cfg, nil)
	if err != nil {
		return "", fmt.Errorf("gemini: creating chat: %w", err)
	}

	// Prime with initial user message.
	resp, err := chat.SendMessage(ctx, genai.Part{Text: userMsg})
	if err != nil {
		return "", fmt.Errorf("gemini: sending message: %w", err)
	}

	for {
		calls := resp.FunctionCalls()
		if len(calls) == 0 {
			// No tool calls — we have the final text response.
			break
		}

		// Execute each tool call and collect responses.
		var responseParts []genai.Part
		for _, fc := range calls {
			tc := ToolCall{
				Name: fc.Name,
				Args: fc.Args,
			}
			if onPlan != nil {
				onPlan(tc)
			}

			output, err := executor(tc)
			if err != nil {
				output = fmt.Sprintf("error: %v", err)
			}
			if onOutput != nil && output != "" {
				onOutput(output)
			}

			// Build a map response that the genai SDK expects.
			responseParts = append(responseParts, genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: map[string]any{"output": output},
				},
			})
		}

		// Feed all tool responses back in one turn.
		resp, err = chat.SendMessage(ctx, responseParts...)
		if err != nil {
			return "", fmt.Errorf("gemini: sending tool response: %w", err)
		}
	}

	text := resp.Text()
	if text == "" {
		return "(no response)", nil
	}
	return text, nil
}

// Reset clears accumulated conversation history.
func (g *geminiClient) Reset() {
	g.history = nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// MapToSchema converts a JSON-Schema map (as produced by ToolSpec.Parameters)
// into a *genai.Schema.  We round-trip through JSON for simplicity.
//
// Exported so that tests outside the package can verify schema conversion.
func MapToSchema(m map[string]any) (*genai.Schema, error) {
	return mapToSchema(m)
}

func mapToSchema(m map[string]any) (*genai.Schema, error) {
	if m == nil {
		return nil, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var s genai.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
