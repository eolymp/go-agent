package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/eolymp/go-agent"
)

// Completer implements agent.ChatCompleter using Anthropic's SDK.
type Completer struct {
	client anthropic.Client
}

// New creates a new Anthropic-based chat completer with the given options.
// It accepts the same options as anthropic.NewClient, such as:
//   - option.WithAPIKey(apiKey)
//   - option.WithBaseURL(baseURL)
//   - option.WithHeader(key, value)
//   - etc.
func New(opts ...option.RequestOption) *Completer {
	return &Completer{client: anthropic.NewClient(opts...)}
}

// NewWithClient creates a new Anthropic-based chat completer with an existing client.
func NewWithClient(client anthropic.Client) *Completer {
	return &Completer{client: client}
}

// Complete implements agent.ChatCompleter by delegating to the Anthropic client.
func (c *Completer) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	resp, err := c.client.Messages.New(ctx, toAnthropicRequest(req))
	if err != nil {
		return nil, err
	}

	return fromAnthropicResponse(resp), nil
}

// toAnthropicRequest converts a universal CompletionRequest to Anthropic-specific params.
func toAnthropicRequest(req agent.CompletionRequest) anthropic.MessageNewParams {
	tokens := int64(4096) // Default
	if req.MaxTokens != nil {
		tokens = int64(*req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: tokens,
		Messages:  []anthropic.MessageParam{},
		System:    []anthropic.TextBlockParam{},
	}

	// Convert messages - separate system messages from conversation messages
	for _, msg := range req.Messages {
		switch m := msg.(type) {
		case agent.SystemMessage:
			content := m.Content
			if m.Name != "" {
				content = fmt.Sprintf("[%s] %s", agent.NormalizeName(m.Name), m.Content)
			}
			params.System = append(params.System, anthropic.TextBlockParam{
				Type: "text",
				Text: content,
			})
		case agent.UserMessage, agent.AssistantMessage, agent.AssistantToolCall:
			if msg := messageToAnthropic(m); msg != nil {
				params.Messages = append(params.Messages, *msg)
			}
		case agent.ToolResult:
			if msg := toolResultToAnthropic(m); msg != nil {
				params.Messages = append(params.Messages, *msg)
			}
		case agent.ToolError:
			if msg := toolErrorToAnthropic(m); msg != nil {
				params.Messages = append(params.Messages, *msg)
			}
		}
	}

	// Convert tools if present
	if len(req.Tools) > 0 {
		params.Tools = toAnthropicTools(req.Tools)

		// Convert tool choice
		switch req.ToolChoice {
		case agent.ToolChoiceAuto:
			params.ToolChoice = anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{
					Type: "auto",
				},
			}
		case agent.ToolChoiceRequired:
			params.ToolChoice = anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{
					Type: "any",
				},
			}
		}
	}

	// Optional parameters
	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	if req.TopP != nil {
		params.TopP = param.NewOpt(*req.TopP)
	}

	return params
}

// fromAnthropicResponse converts an Anthropic response to a universal CompletionResponse.
func fromAnthropicResponse(resp *anthropic.Message) *agent.CompletionResponse {
	return &agent.CompletionResponse{
		Model: string(resp.Model),
		Usage: agent.CompletionUsage{
			PromptTokens:       int(resp.Usage.InputTokens),
			CompletionTokens:   int(resp.Usage.OutputTokens),
			TotalTokens:        int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
			CachedPromptTokens: int(resp.Usage.CacheReadInputTokens),
		},
		Choices: []agent.CompletionChoice{{
			Index:        0,
			FinishReason: mapFinishReason(resp.StopReason),
			Message: agent.CompletionMessage{
				Content:   extractContent(resp.Content),
				ToolCalls: extractToolCalls(resp.Content),
			},
		}},
	}
}

// mapFinishReason converts Anthropic's stop reason to the universal FinishReason type.
func mapFinishReason(reason anthropic.StopReason) agent.FinishReason {
	switch reason {
	case "end_turn":
		return agent.FinishReasonStop
	case "max_tokens":
		return agent.FinishReasonLength
	case "tool_use":
		return agent.FinishReasonToolCalls
	case "stop_sequence":
		return agent.FinishReasonStop
	default:
		return agent.FinishReasonStop
	}
}

// extractContent extracts text content from Anthropic's content blocks.
func extractContent(content []anthropic.ContentBlockUnion) string {
	for _, block := range content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// extractToolCalls extracts tool calls from Anthropic's content blocks.
func extractToolCalls(content []anthropic.ContentBlockUnion) []agent.CompletionToolCall {
	var calls []agent.CompletionToolCall
	for _, block := range content {
		if block.Type == "tool_use" {
			calls = append(calls, agent.CompletionToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		}
	}
	return calls
}

// messageToAnthropic converts a universal Message to Anthropic-specific message format.
// Since Anthropic doesn't support message names natively, names are embedded as [name] prefix.
func messageToAnthropic(msg agent.Message) *anthropic.MessageParam {
	switch m := msg.(type) {
	case agent.UserMessage:
		content := m.Content
		if m.Name != "" {
			content = fmt.Sprintf("[%s] %s", agent.NormalizeName(m.Name), m.Content)
		}
		return &anthropic.MessageParam{
			Role:    "user",
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(content)},
		}
	case agent.AssistantMessage:
		content := m.Content
		if m.Name != "" {
			content = fmt.Sprintf("[%s] %s", agent.NormalizeName(m.Name), m.Content)
		}
		return &anthropic.MessageParam{
			Role:    "assistant",
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(content)},
		}
	case agent.AssistantToolCall:
		content := make([]anthropic.ContentBlockParamUnion, len(m.Calls))
		for i, call := range m.Calls {
			// Parse arguments from JSON string to map
			var input map[string]interface{}
			json.Unmarshal(call.Arguments, &input)
			content[i] = anthropic.NewToolUseBlock(call.CallID, input, call.Name)
		}
		return &anthropic.MessageParam{
			Role:    "assistant",
			Content: content,
		}
	default:
		return nil
	}
}

// toolResultToAnthropic converts a ToolResult to Anthropic format.
func toolResultToAnthropic(result agent.ToolResult) *anthropic.MessageParam {
	return &anthropic.MessageParam{
		Role:    "user",
		Content: []anthropic.ContentBlockParamUnion{anthropic.NewToolResultBlock(result.CallID, result.String(), false)},
	}
}

// toolErrorToAnthropic converts a ToolError to Anthropic format.
func toolErrorToAnthropic(err agent.ToolError) *anthropic.MessageParam {
	return &anthropic.MessageParam{
		Role:    "user",
		Content: []anthropic.ContentBlockParamUnion{anthropic.NewToolResultBlock(err.CallID, err.String(), true)},
	}
}

// toAnthropicTools converts internal tools to Anthropic tool params.
func toAnthropicTools(tools []agent.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))

	for i, tool := range tools {
		t := &anthropic.ToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(tool.Description),
		}

		if tool.InputSchema != nil && tool.InputSchema.Type != "" {
			if tool.InputSchema.Type != "object" {
				panic(fmt.Errorf("tool %q input schema must be object", tool.Name))
			}

			t.InputSchema = anthropic.ToolInputSchemaParam{
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			}
		}

		result[i] = anthropic.ToolUnionParam{OfTool: t}
	}

	return result
}
