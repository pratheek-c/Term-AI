package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicClient implements LLMClient using the Anthropic Messages API.
type anthropicClient struct {
	client    anthropic.Client
	model     string
	sysPrompt string
	history   []anthropic.MessageParam
}

func newAnthropicClient(_ context.Context, cfg ProviderConfig, systemPrompt string) (LLMClient, error) {
	client := anthropic.NewClient(option.WithAPIKey(cfg.APIKey))
	return &anthropicClient{
		client:    client,
		model:     cfg.DefaultModel(),
		sysPrompt: systemPrompt,
	}, nil
}

// Chat sends a single user message maintaining multi-turn history.
func (a *anthropicClient) Chat(ctx context.Context, userMsg string) (string, error) {
	a.history = append(a.history, anthropic.NewUserMessage(
		anthropic.NewTextBlock(userMsg),
	))

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: a.sysPrompt},
		},
		Messages: a.history,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: message: %w", err)
	}

	text := extractAnthropicText(resp.Content)
	if text == "" {
		return "(no response)", nil
	}

	// Accumulate assistant turn.
	a.history = append(a.history, anthropic.NewAssistantMessage(
		anthropic.NewTextBlock(text),
	))
	return text, nil
}

// ChatWithTools runs the tool-calling loop.
func (a *anthropicClient) ChatWithTools(
	ctx context.Context,
	userMsg string,
	tools []ToolSpec,
	executor func(ToolCall) (string, error),
	onPlan func(ToolCall),
	onOutput func(string),
) (string, error) {
	// Build Anthropic tool definitions.
	var anthTools []anthropic.ToolUnionParam
	for _, t := range tools {
		schema := buildAnthropicSchema(t.Parameters)
		anthTools = append(anthTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: schema,
			},
		})
	}

	// Fresh message history per task.
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg)),
	}

	for {
		resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: a.sysPrompt},
			},
			Messages: messages,
			Tools:    anthTools,
		})
		if err != nil {
			return "", fmt.Errorf("anthropic: message: %w", err)
		}

		// Collect tool use blocks and text blocks.
		var toolUseBlocks []anthropic.ContentBlockUnion
		var textParts []string
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			} else if block.Type == "text" {
				textParts = append(textParts, block.Text)
			}
		}

		if resp.StopReason != "tool_use" || len(toolUseBlocks) == 0 {
			// Final response — no more tool calls.
			text := strings.Join(textParts, "")
			if text == "" {
				return "(no response)", nil
			}
			return text, nil
		}

		// Append the assistant turn (with tool_use blocks) to history.
		var assistantContent []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				assistantContent = append(assistantContent, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    block.ID,
						Name:  block.Name,
						Input: json.RawMessage(block.Input),
					},
				})
			} else if block.Type == "text" && block.Text != "" {
				assistantContent = append(assistantContent, anthropic.ContentBlockParamUnion{
					OfText: &anthropic.TextBlockParam{Text: block.Text},
				})
			}
		}
		messages = append(messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: assistantContent,
		})

		// Execute each tool and build the tool-result user turn.
		var resultContent []anthropic.ContentBlockParamUnion
		for _, block := range toolUseBlocks {
			var args map[string]any
			if err := json.Unmarshal(block.Input, &args); err != nil {
				args = map[string]any{"raw": string(block.Input)}
			}

			tc := ToolCall{
				ID:   block.ID,
				Name: block.Name,
				Args: args,
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

			resultContent = append(resultContent, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: block.ID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: output}},
					},
				},
			})
		}
		messages = append(messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: resultContent,
		})
	}
}

// Reset clears conversation history.
func (a *anthropicClient) Reset() {
	a.history = nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func extractAnthropicText(blocks []anthropic.ContentBlockUnion) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// BuildAnthropicSchema builds a ToolInputSchemaParam from a JSON-Schema map.
// We extract "properties" and "required" directly since that is what the
// Anthropic SDK needs.
//
// Exported so that tests outside the package can verify schema conversion.
func BuildAnthropicSchema(params map[string]any) anthropic.ToolInputSchemaParam {
	return buildAnthropicSchema(params)
}

// buildAnthropicSchema builds a ToolInputSchemaParam from a JSON-Schema map.
// We extract "properties" and "required" directly since that is what the
// Anthropic SDK needs.
func buildAnthropicSchema(params map[string]any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{}
	if params == nil {
		return schema
	}
	if props, ok := params["properties"]; ok {
		schema.Properties = props
	}
	if req, ok := params["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	if req, ok := params["required"].([]string); ok {
		schema.Required = req
	}
	return schema
}
