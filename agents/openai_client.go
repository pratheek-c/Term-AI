package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// openaiClient implements LLMClient using the OpenAI Chat Completions API.
type openaiClient struct {
	client  openai.Client
	model   string
	history []openai.ChatCompletionMessageParamUnion
}

func newOpenAIClient(_ context.Context, cfg ProviderConfig, systemPrompt string) (LLMClient, error) {
	client := openai.NewClient(option.WithAPIKey(cfg.APIKey))
	history := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}
	return &openaiClient{
		client:  client,
		model:   cfg.DefaultModel(),
		history: history,
	}, nil
}

// Chat sends a single user message maintaining multi-turn history.
func (o *openaiClient) Chat(ctx context.Context, userMsg string) (string, error) {
	o.history = append(o.history, openai.UserMessage(userMsg))

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(o.model),
		Messages: o.history,
	})
	if err != nil {
		return "", fmt.Errorf("openai: chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "(no response)", nil
	}

	text := resp.Choices[0].Message.Content
	// Accumulate assistant reply in history.
	o.history = append(o.history, openai.AssistantMessage(text))
	return text, nil
}

// ChatWithTools runs the tool-calling loop.
func (o *openaiClient) ChatWithTools(
	ctx context.Context,
	userMsg string,
	tools []ToolSpec,
	executor func(ToolCall) (string, error),
	onPlan func(ToolCall),
	onOutput func(string),
) (string, error) {
	// Build OpenAI tool definitions.
	var oaiTools []openai.ChatCompletionToolParam
	for _, t := range tools {
		params, err := json.Marshal(t.Parameters)
		if err != nil {
			return "", fmt.Errorf("openai: marshalling params for %s: %w", t.Name, err)
		}
		var fp shared.FunctionParameters
		if err := json.Unmarshal(params, &fp); err != nil {
			return "", fmt.Errorf("openai: unmarshalling params for %s: %w", t.Name, err)
		}
		oaiTools = append(oaiTools, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  fp,
			},
		})
	}

	// Build the message history for this task (fresh, no cross-task bleed).
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(userMsg),
	}

	for {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(o.model),
			Messages: messages,
			Tools:    oaiTools,
		})
		if err != nil {
			return "", fmt.Errorf("openai: chat completion: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "(no response)", nil
		}

		choice := resp.Choices[0]

		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			// No tool calls — final text response.
			return choice.Message.Content, nil
		}

		// Append assistant message (with tool_calls) to history.
		messages = append(messages, openai.AssistantMessage(choice.Message.Content))

		// Execute each tool call.
		for _, tc := range choice.Message.ToolCalls {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{"raw": tc.Function.Arguments}
			}

			call := ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			}
			if onPlan != nil {
				onPlan(call)
			}

			output, err := executor(call)
			if err != nil {
				output = fmt.Sprintf("error: %v", err)
			}
			if onOutput != nil && output != "" {
				onOutput(output)
			}

			messages = append(messages, openai.ToolMessage(output, tc.ID))
		}
	}
}

// Reset clears conversation history (keeps the system prompt).
func (o *openaiClient) Reset() {
	// Keep the system message at index 0.
	if len(o.history) > 0 {
		o.history = o.history[:1]
	}
}
