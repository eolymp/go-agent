package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/eolymp/go-agent"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

type Completer struct {
	client openai.Client
}

// New creates a new OpenAI-based chat completer with the given options.
// It accepts the same options as openai.NewClient, such as:
//   - option.WithAPIKey(apiKey)
//   - option.WithBaseURL(baseURL)
//   - option.WithHeader(key, value)
//   - etc.
func New(opts ...option.RequestOption) *Completer {
	return &Completer{client: openai.NewClient(opts...)}
}

// NewWithClient creates a new OpenAI-based chat completer with an existing client.
func NewWithClient(client openai.Client) *Completer {
	return &Completer{client: client}
}

// Complete implements agent.ChatCompleter by delegating to the OpenAI client.
func (c *Completer) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if req.StreamCallback != nil {
		return c.stream(ctx, req)
	}

	resp, err := c.client.Chat.Completions.New(ctx, toOpenAIRequest(req))
	if err != nil {
		return nil, err
	}

	return fromOpenAIResponse(resp), nil
}

// stream handles streaming completion with callback support.
func (c *Completer) stream(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	stream := c.client.Chat.Completions.NewStreaming(ctx, toOpenAIRequest(req))

	resp := &agent.CompletionResponse{}
	calls := make(map[int]*agent.ToolCall)
	var text strings.Builder

	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		event := stream.Current()
		acc.AddChunk(event)

		if event.Model != "" && resp.Model == "" {
			resp.Model = event.Model
		}

		if len(event.Choices) > 0 {
			delta := event.Choices[0].Delta

			if delta.Content != "" {
				text.WriteString(delta.Content)

				chunk := agent.Chunk{
					Type: agent.StreamChunkTypeText,
					Text: delta.Content,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			}

			// Handle tool call deltas
			for _, tc := range delta.ToolCalls {
				index := int(tc.Index)

				if tc.Function.Name != "" {
					calls[index] = &agent.ToolCall{
						ID:   tc.ID,
						Name: tc.Function.Name,
					}

					chunk := agent.Chunk{
						Type:  agent.StreamChunkTypeToolCallStart,
						Index: index + 1,
						Call:  calls[index],
					}

					if err := req.StreamCallback(ctx, chunk); err != nil {
						return nil, err
					}
				}

				if tc.Function.Arguments != "" {
					if c, ok := calls[index]; ok {
						c.Arguments += tc.Function.Arguments
						chunk := agent.Chunk{
							Type:  agent.StreamChunkTypeToolCallDelta,
							Index: index + 1,
							Call: &agent.ToolCall{
								ID:        c.ID,
								Name:      c.Name,
								Arguments: tc.Function.Arguments,
							},
						}

						if err := req.StreamCallback(ctx, chunk); err != nil {
							return nil, err
						}
					}
				}
			}

			if event.Choices[0].FinishReason != "" {
				resp.FinishReason = mapFinishReason(event.Choices[0].FinishReason)
			}
		}

		resp.Usage.PromptTokens = int(event.Usage.PromptTokens)
		resp.Usage.CompletionTokens = int(event.Usage.CompletionTokens)
		resp.Usage.TotalTokens = int(event.Usage.TotalTokens)
		resp.Usage.CachedPromptTokens = int(event.Usage.PromptTokensDetails.CachedTokens)

		chunk := agent.Chunk{
			Type:  agent.StreamChunkTypeUsage,
			Usage: &resp.Usage,
		}

		if err := req.StreamCallback(ctx, chunk); err != nil {
			return nil, err
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	reason := resp.FinishReason
	chunk := agent.Chunk{
		Type:         agent.StreamChunkTypeFinish,
		FinishReason: reason,
	}

	if err := req.StreamCallback(ctx, chunk); err != nil {
		return nil, err
	}

	if text.Len() > 0 {
		resp.Content = append(resp.Content, agent.AssistantMessageBlock{
			Text: text.String(),
		})
	}

	length := -1
	for idx := range calls {
		if idx > length {
			length = idx
		}
	}

	for i := 0; i <= length; i++ {
		if tc := calls[i]; tc != nil {
			resp.Content = append(resp.Content, agent.AssistantMessageBlock{Call: tc})
		}
	}

	return resp, nil
}

// toOpenAIRequest converts a universal CompletionRequest to OpenAI-specific params.
func toOpenAIRequest(req agent.CompletionRequest) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: make([]openai.ChatCompletionMessageParamUnion, len(req.Messages)),
	}

	// Convert messages
	for i, msg := range req.Messages {
		params.Messages[i] = messageToOpenAI(msg)
	}

	// Convert tools if present
	if len(req.Tools) > 0 {
		params.Tools = toOpenAITools(req.Tools)
		params.ParallelToolCalls = openai.Bool(req.ParallelToolCalls)

		// Convert tool choice (currently only "auto" is supported by OpenAI SDK)
		switch req.ToolChoice {
		case agent.ToolChoiceAuto:
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt(string(openai.AssistantToolChoiceOptionAutoAuto)),
			}
		case agent.ToolChoiceRequired, agent.ToolChoiceNone:
			// Note: "required" and "none" tool choices are not currently supported
			// by the OpenAI Go SDK and will default to auto behavior
		}
	}

	// Optional parameters
	if req.MaxTokens != nil {
		params.MaxTokens = openai.Int(int64(*req.MaxTokens))
	}

	if req.Temperature != nil {
		params.Temperature = openai.Float(float64(*req.Temperature))
	}

	if req.TopP != nil {
		params.TopP = openai.Float(float64(*req.TopP))
	}

	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		params.ReasoningEffort = openai.ReasoningEffort(req.Reasoning.Effort)
	}

	return params
}

// fromOpenAIResponse converts an OpenAI response to a universal CompletionResponse.
func fromOpenAIResponse(resp *openai.ChatCompletion) *agent.CompletionResponse {
	// Pick the first choice (typically OpenAI only returns one choice anyway)
	if len(resp.Choices) == 0 {
		panic("OpenAI response has no choices")
	}

	choice := resp.Choices[0]

	return &agent.CompletionResponse{
		Model:        resp.Model,
		Content:      fromOpenAIContent(choice.Message.Content, choice.Message.ToolCalls),
		FinishReason: mapFinishReason(choice.FinishReason),
		Usage: agent.CompletionUsage{
			PromptTokens:       int(resp.Usage.PromptTokens),
			CompletionTokens:   int(resp.Usage.CompletionTokens),
			TotalTokens:        int(resp.Usage.TotalTokens),
			CachedPromptTokens: int(resp.Usage.PromptTokensDetails.CachedTokens),
		},
	}
}

// mapFinishReason converts OpenAI's string finish reason to the universal FinishReason type.
func mapFinishReason(reason string) agent.FinishReason {
	switch reason {
	case "stop":
		return agent.FinishReasonStop
	case "length":
		return agent.FinishReasonLength
	case "tool_calls":
		return agent.FinishReasonToolCalls
	case "content_filter":
		return agent.FinishReasonContentFilter
	default:
		return agent.FinishReasonStop // Default to stop for unknown reasons
	}
}

// fromOpenAIContent converts OpenAI content and tool calls to content blocks.
func fromOpenAIContent(content string, toolCalls []openai.ChatCompletionMessageToolCall) []agent.AssistantMessageBlock {
	var blocks []agent.AssistantMessageBlock

	// Add text block if content is not empty
	if content != "" {
		blocks = append(blocks, agent.AssistantMessageBlock{
			Text: content,
		})
	}

	// Add tool use blocks for each tool call
	for _, call := range toolCalls {
		blocks = append(blocks, agent.AssistantMessageBlock{
			Call: &agent.ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}

	return blocks
}

// messageToOpenAI converts a universal Message to OpenAI-specific message format.
func messageToOpenAI(msg agent.Message) openai.ChatCompletionMessageParamUnion {
	switch m := msg.(type) {
	case agent.SystemMessage:
		return systemMessageToOpenAI(m)
	case agent.UserMessage:
		return userMessageToOpenAI(m)
	case agent.AssistantMessage:
		return assistantMessageToOpenAI(m)
	case agent.ToolResult:
		return toolResultToOpenAI(m)
	case agent.ToolError:
		return toolErrorToOpenAI(m)
	default:
		panic(fmt.Sprintf("unknown message type: %T", msg))
	}
}

// systemMessageToOpenAI converts a SystemMessage to OpenAI format.
func systemMessageToOpenAI(m agent.SystemMessage) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{OfSystem: &openai.ChatCompletionSystemMessageParam{
		Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: param.NewOpt(m.Content)},
	}}
}

// userMessageToOpenAI converts a UserMessage to OpenAI format.
func userMessageToOpenAI(m agent.UserMessage) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{OfUser: &openai.ChatCompletionUserMessageParam{
		Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: param.NewOpt(m.Content)},
	}}
}

// assistantMessageToOpenAI converts an AssistantMessage to OpenAI format.
func assistantMessageToOpenAI(m agent.AssistantMessage) openai.ChatCompletionMessageParamUnion {
	var msg openai.ChatCompletionAssistantMessageParam

	var texts []string
	var calls []openai.ChatCompletionMessageToolCallParam

	// Extract text and tool use blocks
	for _, block := range m.Content {
		switch {
		case block.Call != nil:
			calls = append(calls, openai.ChatCompletionMessageToolCallParam{
				ID: block.Call.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      block.Call.Name,
					Arguments: block.Call.Arguments,
				},
			})
		case block.Text != "":
			texts = append(texts, block.Text)
		}
	}

	// Set content if there's any text
	if len(texts) > 0 {
		msg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{OfString: param.NewOpt(strings.Join(texts, ""))}
	}

	// Set tool calls if there are any
	if len(calls) > 0 {
		msg.ToolCalls = calls
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &msg}
}

// toolResultToOpenAI converts a ToolResult to OpenAI format.
func toolResultToOpenAI(c agent.ToolResult) openai.ChatCompletionMessageParamUnion {
	return openai.ToolMessage(c.String(), c.CallID)
}

// toolErrorToOpenAI converts a ToolError to OpenAI format.
func toolErrorToOpenAI(c agent.ToolError) openai.ChatCompletionMessageParamUnion {
	return openai.ToolMessage(c.String(), c.CallID)
}

// toOpenAITools converts internal tools to OpenAI tool params.
func toOpenAITools(tools []agent.Tool) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, len(tools))

	for i, tool := range tools {
		fn := openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
		}

		if tool.InputSchema != nil && tool.InputSchema.Type != "" {
			if tool.InputSchema.Type != "object" {
				panic(fmt.Errorf("tool %q input schema must be object", tool.Name))
			}

			fn.Parameters = openai.FunctionParameters{
				"type":                 "object",
				"properties":           tool.InputSchema.Properties,
				"required":             tool.InputSchema.Required,
				"additionalProperties": false,
			}
		}

		result[i] = openai.ChatCompletionToolParam{Function: fn}
	}

	return result
}
