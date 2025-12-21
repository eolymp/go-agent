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
	params := anthropic.MessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: 64000}

	if req.MaxTokens != nil {
		params.MaxTokens = int64(*req.MaxTokens)
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	if req.TopP != nil {
		params.TopP = param.NewOpt(*req.TopP)
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

		case agent.UserMessage:
			content := m.Content
			if m.Name != "" {
				content = fmt.Sprintf("[%s] %s", agent.NormalizeName(m.Name), m.Content)
			}

			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "user",
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(content)},
			})

		case agent.AssistantMessage:
			content := m.Content
			if m.Name != "" {
				content = fmt.Sprintf("[%s] %s", agent.NormalizeName(m.Name), m.Content)
			}

			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "assistant",
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(content)},
			})

		case agent.AssistantToolCall:
			content := make([]anthropic.ContentBlockParamUnion, len(m.Calls))
			for i, call := range m.Calls {
				var input map[string]interface{}
				_ = json.Unmarshal(call.Arguments, &input)

				content[i] = anthropic.NewToolUseBlock(call.CallID, input, call.Name)
			}

			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "assistant",
				Content: content,
			})

		case agent.ToolResult:
			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "user",
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewToolResultBlock(m.CallID, m.String(), false)},
			})

		case agent.ToolError:
			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "user",
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewToolResultBlock(m.CallID, m.String(), true)},
			})
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

	return params
}

// fromAnthropicResponse converts an Anthropic response to a universal CompletionResponse.
func fromAnthropicResponse(resp *anthropic.Message) *agent.CompletionResponse {
	ar := &agent.CompletionResponse{
		Model:        string(resp.Model),
		Content:      make([]agent.ContentBlock, len(resp.Content)),
		FinishReason: mapFinishReason(resp.StopReason),
		Usage: agent.CompletionUsage{
			PromptTokens:       int(resp.Usage.InputTokens),
			CompletionTokens:   int(resp.Usage.OutputTokens),
			TotalTokens:        int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
			CachedPromptTokens: int(resp.Usage.CacheReadInputTokens),
		},
	}

	for i, b := range resp.Content {
		switch b.Type {
		case "text":
			ar.Content[i] = agent.ContentBlock{
				Type: agent.ContentBlockTypeText,
				Text: b.Text,
			}
		case "tool_use":
			ar.Content[i] = agent.ContentBlock{
				Type:      agent.ContentBlockTypeToolUse,
				ID:        b.ID,
				Name:      b.Name,
				Arguments: string(b.Input),
			}
		}
	}

	return ar
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
