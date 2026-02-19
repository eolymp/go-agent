package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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

// Complete implements ChatCompleter by delegating to the Anthropic client.
func (c *Completer) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if req.StreamCallback != nil {
		if len(req.Betas) > 0 || req.Container != nil || req.Reasoning != nil {
			return c.betaStream(ctx, req)
		}

		return c.stream(ctx, req)
	}

	if len(req.Betas) > 0 || req.Container != nil || req.Reasoning != nil {
		resp, err := c.client.Beta.Messages.New(ctx, toBetaAnthropicRequest(req))
		if err != nil {
			return nil, err
		}

		return fromBetaAnthropicResponse(ctx, resp), nil
	}

	resp, err := c.client.Messages.New(ctx, toAnthropicRequest(req))
	if err != nil {
		return nil, err
	}

	return fromAnthropicResponse(ctx, resp), nil
}

// stream handles streaming completion with callback support.
func (c *Completer) stream(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	stream := c.client.Messages.NewStreaming(ctx, toAnthropicRequest(req))
	defer stream.Close()

	resp := &agent.CompletionResponse{}
	blocks := make(map[int]*agent.MessageBlock)

	// Process stream events
	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			resp.Model = string(event.Message.Model)
			resp.Usage.PromptTokens = int(event.Message.Usage.InputTokens)
			resp.Usage.CachedPromptTokens = int(event.Message.Usage.CacheReadInputTokens)
		case "content_block_start":
			index := int(event.Index)
			block := &agent.MessageBlock{}

			switch event.ContentBlock.Type {
			case "text":
				block.Type = agent.MessageBlockTypeText
			case "tool_use":
				block.Type = agent.MessageBlockTypeToolCall
				block.ToolCall = &agent.ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}

				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeToolCallStart,
					Index: index,
					Call:  block.ToolCall,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			default:
				slog.WarnContext(ctx, "Unknown content block type", "channel", "llm", "type", event.ContentBlock.Type)
			}

			blocks[index] = block
		case "content_block_delta":
			index := int(event.Index)

			block, ok := blocks[index]
			if !ok {
				continue
			}

			switch event.Delta.Type {
			case "text_delta":
				if block.Type != agent.MessageBlockTypeText {
					continue
				}

				block.Text += event.Delta.Text
				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeText,
					Index: index,
					Text:  event.Delta.Text,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			case "input_json_delta":
				if block.Type != agent.MessageBlockTypeToolCall {
					continue
				}

				block.ToolCall.Arguments += event.Delta.PartialJSON
				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeToolCallDelta,
					Index: index,
					Call:  &agent.ToolCall{ID: block.ToolCall.ID, Name: block.ToolCall.Name, Arguments: event.Delta.PartialJSON},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			default:
				slog.WarnContext(ctx, "Unknown event delta type", "channel", "llm", "type", event.Delta.Type)
			}
		case "content_block_stop":
		case "message_delta":
			resp.Usage.CompletionTokens = int(event.Usage.OutputTokens)
			resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens

			if event.Delta.StopReason != "" {
				resp.FinishReason = mapFinishReason(event.Delta.StopReason)
			}

			chunk := agent.Chunk{
				Type:  agent.StreamChunkTypeUsage,
				Usage: &resp.Usage,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}
		case "message_stop":
			reason := resp.FinishReason
			chunk := agent.Chunk{
				Type:         agent.StreamChunkTypeFinish,
				FinishReason: reason,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}
		default:
			slog.WarnContext(ctx, "Unknown event type", "channel", "llm", "type", event.Type)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	length := -1
	for idx := range blocks {
		if idx > length {
			length = idx
		}
	}

	resp.Content = make([]agent.MessageBlock, length+1)
	for idx, block := range blocks {
		resp.Content[idx] = *block
	}

	return resp, nil
}

func (c *Completer) betaStream(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	stream := c.client.Beta.Messages.NewStreaming(ctx, toBetaAnthropicRequest(req))
	defer stream.Close()

	resp := &agent.CompletionResponse{}
	blocks := make(map[int]*agent.MessageBlock)

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			resp.Model = string(event.Message.Model)
			resp.Usage.PromptTokens = int(event.Message.Usage.InputTokens)
			resp.Usage.CachedPromptTokens = int(event.Message.Usage.CacheReadInputTokens)

		case "content_block_start":
			index := int(event.Index)
			block := &agent.MessageBlock{}

			switch event.ContentBlock.Type {
			case "text":
				block.Type = agent.MessageBlockTypeText
			case "thinking":
				block.Type = agent.MessageBlockTypeReasoning
			case "tool_use":
				block.Type = agent.MessageBlockTypeToolCall
				block.ToolCall = &agent.ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}

				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeToolCallStart,
					Index: index,
					Call:  block.ToolCall,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			case "server_tool_use":
				block.Type = agent.MessageBlockTypeServerToolCall
				block.ToolCall = &agent.ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}

				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeServerToolCallStart,
					Index: index,
					Call:  &agent.ToolCall{ID: event.ContentBlock.ID, Name: event.ContentBlock.Name},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			case "web_search_tool_result":
				block.Type = agent.MessageBlockTypeToolResult
				block.ToolResult = &agent.ToolResult{CallID: event.ContentBlock.ToolUseID}

				chunk := agent.Chunk{
					Type:   agent.StreamChunkTypeToolResult,
					Index:  index,
					Result: &agent.ToolResult{CallID: event.ContentBlock.ToolUseID},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			case "text_editor_code_execution_tool_result", "bash_code_execution_tool_result":
				block.Type = agent.MessageBlockTypeToolResult
				block.ToolResult = &agent.ToolResult{CallID: event.ContentBlock.ToolUseID}

				chunk := agent.Chunk{
					Type:   agent.StreamChunkTypeToolResult,
					Index:  index,
					Result: &agent.ToolResult{CallID: event.ContentBlock.ToolUseID},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			default:
				slog.WarnContext(ctx, "Unknown content block type in block start event", "channel", "llm", "type", event.ContentBlock.Type)
			}

			blocks[index] = block

		case "content_block_delta":
			index := int(event.Index)

			block, ok := blocks[index]
			if !ok {
				continue
			}

			switch event.Delta.Type {
			case "text_delta":
				if block.Type != agent.MessageBlockTypeText {
					continue
				}

				block.Text += event.Delta.Text
				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeText,
					Index: index,
					Text:  event.Delta.Text,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}

			case "thinking_delta":
				if block.Type != agent.MessageBlockTypeReasoning {
					continue
				}

				block.Text += event.Delta.Text
				chunk := agent.Chunk{
					Type:  agent.StreamChunkTypeReasoning,
					Index: index,
					Text:  event.Delta.Text,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}

			case "signature_delta":
				if block.Type != agent.MessageBlockTypeSignature {
					continue
				}

				block.Signature = event.Delta.Signature
				chunk := agent.Chunk{
					Type:      agent.StreamChunkTypeSignature,
					Index:     index,
					Signature: event.Delta.Signature,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}

			case "input_json_delta":
				switch block.Type {
				case agent.MessageBlockTypeToolCall:
					block.ToolCall.Arguments += event.Delta.PartialJSON
					chunk := agent.Chunk{
						Type:  agent.StreamChunkTypeToolCallDelta,
						Index: index,
						Call:  &agent.ToolCall{ID: block.ToolCall.ID, Name: block.ToolCall.Name, Arguments: event.Delta.PartialJSON},
					}

					if err := req.StreamCallback(ctx, chunk); err != nil {
						return nil, err
					}

				case agent.MessageBlockTypeServerToolCall:
					block.ToolCall.Arguments += event.Delta.PartialJSON
					chunk := agent.Chunk{
						Type:  agent.StreamChunkTypeServerToolCallDelta,
						Index: index,
						Call:  &agent.ToolCall{ID: block.ToolCall.ID, Name: block.ToolCall.Name, Arguments: event.Delta.PartialJSON},
					}

					if err := req.StreamCallback(ctx, chunk); err != nil {
						return nil, err
					}
				}

			default:
				slog.WarnContext(ctx, "Unknown event delta type", "channel", "llm", "type", event.Delta.Type)
			}

		case "content_block_stop":

		case "message_delta":
			resp.Usage.CompletionTokens = int(event.Usage.OutputTokens)
			resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens

			if event.Delta.StopReason != "" {
				resp.FinishReason = mapBetaFinishReason(event.Delta.StopReason)
			}

			chunk := agent.Chunk{
				Type:  agent.StreamChunkTypeUsage,
				Usage: &resp.Usage,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}

		case "message_stop":
			reason := resp.FinishReason
			chunk := agent.Chunk{
				Type:         agent.StreamChunkTypeFinish,
				FinishReason: reason,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}

		default:
			slog.WarnContext(ctx, "Unknown event type", "channel", "llm", "type", event.ContentBlock.Type)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	length := -1
	for idx := range blocks {
		if idx > length {
			length = idx
		}
	}

	resp.Content = make([]agent.MessageBlock, length+1)
	for idx, block := range blocks {
		resp.Content[idx] = *block
	}

	return resp, nil
}

// toAnthropicRequest converts a universal CompletionRequest to Anthropic-specific params.
func toAnthropicRequest(req agent.CompletionRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: 8192}

	if req.MaxTokens != nil {
		params.MaxTokens = *req.MaxTokens
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(float64(*req.Temperature))
	}

	if req.TopP != nil {
		params.TopP = param.NewOpt(float64(*req.TopP))
	}

	if req.TopK != nil {
		params.TopK = param.NewOpt(int64(*req.TopK))
	}

	// Convert messages - separate system messages from conversation messages
	for _, msg := range req.Messages {
		switch m := msg.(type) {
		case agent.SystemMessage:
			params.System = append(params.System, anthropic.TextBlockParam{
				Type: "text",
				Text: m.Content,
			})

		case agent.UserMessage:
			params.Messages = append(params.Messages, anthropic.MessageParam{
				Role:    "user",
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(m.Content)},
			})

		case agent.AssistantMessage:
			content := make([]anthropic.ContentBlockParamUnion, len(m.Content))
			for i, block := range m.Content {
				switch {
				case block.Type == agent.MessageBlockTypeToolCall:
					var input map[string]interface{}
					if block.ToolCall.Arguments != "" {
						_ = json.Unmarshal([]byte(block.ToolCall.Arguments), &input)
					}

					content[i] = anthropic.NewToolUseBlock(block.ToolCall.ID, input, block.ToolCall.Name)
				case block.Type == agent.MessageBlockTypeText:
					content[i] = anthropic.NewTextBlock(block.Text)
				}
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
		default:
		}
	}

	return params
}

// fromAnthropicResponse converts an Anthropic response to a universal CompletionResponse.
func fromAnthropicResponse(ctx context.Context, resp *anthropic.Message) *agent.CompletionResponse {
	ar := &agent.CompletionResponse{
		Model:        string(resp.Model),
		Content:      make([]agent.MessageBlock, len(resp.Content)),
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
			ar.Content[i] = agent.MessageBlock{
				Type: agent.MessageBlockTypeText,
				Text: b.Text,
			}
		case "tool_use":
			ar.Content[i] = agent.MessageBlock{
				Type: agent.MessageBlockTypeToolCall,
				ToolCall: &agent.ToolCall{
					ID:        b.ID,
					Name:      b.Name,
					Arguments: string(b.Input),
				},
			}
		default:
			slog.WarnContext(ctx, "Unknown content block type", "channel", "llm", "type", b.Type)
		}
	}

	return ar
}

// toAnthropicTools converts internal tools to Anthropic tool params.
func toAnthropicTools(tools []agent.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))

	for i, tool := range tools {
		switch tool.Type {
		case "bash_20250124":
			result[i] = anthropic.ToolUnionParam{OfBashTool20250124: &anthropic.ToolBash20250124Param{Name: "bash", Type: "bash_20250124"}}
			continue
		case "web_search_20250305":
			result[i] = anthropic.ToolUnionParam{OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{Name: "web_search", Type: "web_search_20250305"}}
			continue
		}

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

func toBetaAnthropicRequest(req agent.CompletionRequest) anthropic.BetaMessageNewParams {
	params := anthropic.BetaMessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: 8192}

	if req.MaxTokens != nil {
		params.MaxTokens = *req.MaxTokens
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(float64(*req.Temperature))
	}

	if req.TopP != nil {
		params.TopP = param.NewOpt(float64(*req.TopP))
	}

	if req.TopK != nil {
		params.TopK = param.NewOpt(int64(*req.TopK))
	}

	if len(req.Betas) > 0 {
		params.Betas = req.Betas
	}

	if req.Container != nil {
		skills := make([]anthropic.BetaSkillParams, len(req.Container.Skills))
		for i, s := range req.Container.Skills {
			skills[i] = anthropic.BetaSkillParams{
				SkillID: s.SkillID,
				Type:    anthropic.BetaSkillParamsType(s.Type),
			}

			if s.Version != "" {
				skills[i].Version = param.NewOpt(s.Version)
			}
		}

		params.Container = anthropic.BetaMessageNewParamsContainerUnion{
			OfContainers: &anthropic.BetaContainerParams{
				Skills: skills,
			},
		}

		if req.Container.ID != "" {
			params.Container.OfContainers.ID = param.NewOpt(req.Container.ID)
		}
	}

	for _, msg := range req.Messages {
		switch m := msg.(type) {
		case agent.SystemMessage:
			params.System = append(params.System, anthropic.BetaTextBlockParam{
				Type: "text",
				Text: m.Content,
			})

		case agent.UserMessage:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role:    "user",
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(m.Content)},
			})

		case agent.AssistantMessage:
			content := make([]anthropic.BetaContentBlockParamUnion, len(m.Content))
			for i, block := range m.Content {
				switch block.Type {
				case agent.MessageBlockTypeToolCall:
					var input map[string]interface{}
					if block.ToolCall.Arguments != "" {
						_ = json.Unmarshal([]byte(block.ToolCall.Arguments), &input)
					}

					content[i] = anthropic.NewBetaToolUseBlock(block.ToolCall.ID, input, block.ToolCall.Name)
				case agent.MessageBlockTypeText:
					content[i] = anthropic.NewBetaTextBlock(block.Text)
				case agent.MessageBlockTypeReasoning:
					content[i] = anthropic.BetaContentBlockParamUnion{
						OfThinking: &anthropic.BetaThinkingBlockParam{Type: "thinking", Thinking: block.Text},
					}
				case agent.MessageBlockTypeServerToolCall:
					var input map[string]interface{}
					if block.ToolCall.Arguments != "" {
						_ = json.Unmarshal([]byte(block.ToolCall.Arguments), &input)
					}

					content[i] = anthropic.BetaContentBlockParamUnion{
						OfServerToolUse: &anthropic.BetaServerToolUseBlockParam{
							Type:  "server_tool_use",
							ID:    block.ToolCall.ID,
							Name:  anthropic.BetaServerToolUseBlockParamName(block.ToolCall.Name),
							Input: input,
						},
					}
				case agent.MessageBlockTypeToolResult:
					content[i] = anthropic.BetaContentBlockParamUnion{
						OfWebSearchToolResult: &anthropic.BetaWebSearchToolResultBlockParam{
							Type:      "web_search_tool_result",
							ToolUseID: block.ToolResult.CallID,
							// TODO: Parse and set Content properly when SDK supports it
						},
					}
				}
			}

			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role:    "assistant",
				Content: content,
			})

		case agent.ToolResult:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role: "user",
				Content: []anthropic.BetaContentBlockParamUnion{{
					OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: m.CallID,
						Content: []anthropic.BetaToolResultBlockParamContentUnion{{
							OfText: &anthropic.BetaTextBlockParam{
								Type: "text",
								Text: m.String(),
							},
						}},
					},
				}},
			})

		case agent.ToolError:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role: "user",
				Content: []anthropic.BetaContentBlockParamUnion{{
					OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: m.CallID,
						IsError:   param.NewOpt(true),
						Content: []anthropic.BetaToolResultBlockParamContentUnion{{
							OfText: &anthropic.BetaTextBlockParam{
								Type: "text",
								Text: m.String(),
							},
						}},
					},
				}},
			})
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = toBetaAnthropicTools(req.Tools)

		switch req.ToolChoice {
		case agent.ToolChoiceAuto:
			params.ToolChoice = anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{
					Type: "auto",
				},
			}
		case agent.ToolChoiceRequired:
			params.ToolChoice = anthropic.BetaToolChoiceUnionParam{
				OfAny: &anthropic.BetaToolChoiceAnyParam{
					Type: "any",
				},
			}
		default:
		}
	}

	if req.Reasoning != nil {
		if req.Reasoning.Enabled {
			budget := int64(1024)
			if req.Reasoning.Budget > 0 {
				budget = int64(req.Reasoning.Budget)
			}

			params.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
		} else {
			disabled := anthropic.NewBetaThinkingConfigDisabledParam()
			params.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &disabled}
		}
	}

	return params
}

func fromBetaAnthropicResponse(ctx context.Context, resp *anthropic.BetaMessage) *agent.CompletionResponse {
	ar := &agent.CompletionResponse{
		Model:        string(resp.Model),
		Content:      make([]agent.MessageBlock, len(resp.Content)),
		FinishReason: mapBetaFinishReason(resp.StopReason),
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
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeText, Text: b.Text}
		case "thinking":
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeReasoning, Text: b.Text}
		case "tool_use":
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeToolCall, ToolCall: &agent.ToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Input)}}
		case "server_tool_use":
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeServerToolCall, ToolCall: &agent.ToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Input)}}
		case "web_search_tool_result":
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeToolResult, ToolResult: &agent.ToolResult{CallID: b.ToolUseID, Result: fmt.Sprintf("%v", b.Content)}}
		case "text_editor_code_execution_tool_result", "bash_code_execution_tool_result":
			ar.Content[i] = agent.MessageBlock{Type: agent.MessageBlockTypeToolResult, ToolResult: &agent.ToolResult{CallID: b.ToolUseID, Result: fmt.Sprintf("%v", b.Content)}}
		default:
			slog.WarnContext(ctx, "Unknown content block type", "channel", "llm", "type", b.Type)
		}
	}

	return ar
}

func toBetaAnthropicTools(tools []agent.Tool) []anthropic.BetaToolUnionParam {
	result := make([]anthropic.BetaToolUnionParam, len(tools))

	for i, tool := range tools {
		var dl param.Opt[bool]
		if tool.DeferLoading {
			dl = param.NewOpt(true)
		}

		switch tool.Type {
		case "bash_20250124":
			result[i] = anthropic.BetaToolUnionParam{OfBashTool20250124: &anthropic.BetaToolBash20250124Param{Name: "bash", Type: "bash_20250124", DeferLoading: dl}}
			continue
		case "bash_20241022":
			result[i] = anthropic.BetaToolUnionParam{OfBashTool20241022: &anthropic.BetaToolBash20241022Param{Name: "bash", Type: "bash_20241022", DeferLoading: dl}}
			continue
		case "code_execution_20250825":
			result[i] = anthropic.BetaToolUnionParam{OfCodeExecutionTool20250825: &anthropic.BetaCodeExecutionTool20250825Param{Name: "code_execution", Type: "code_execution_20250825", DeferLoading: dl}}
			continue
		case "web_search_20250305":
			result[i] = anthropic.BetaToolUnionParam{OfWebSearchTool20250305: &anthropic.BetaWebSearchTool20250305Param{Name: "web_search", Type: "web_search_20250305", DeferLoading: dl}}
			continue
		case "tool_search_tool_regex_20251119":
			result[i] = anthropic.BetaToolUnionParam{OfToolSearchToolRegex20251119: &anthropic.BetaToolSearchToolRegex20251119Param{Name: "tool_search_tool_regex", Type: "tool_search_tool_regex_20251119", DeferLoading: dl}}
			continue
		case "tool_search_tool_bm25_20251119":
			result[i] = anthropic.BetaToolUnionParam{OfToolSearchToolBm25_20251119: &anthropic.BetaToolSearchToolBm25_20251119Param{Name: "tool_search_tool_bm25", Type: "tool_search_tool_bm25_20251119", DeferLoading: dl}}
			continue
		}

		t := &anthropic.BetaToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(tool.Description),
		}

		if tool.DeferLoading {
			t.DeferLoading = param.NewOpt(true)
		}

		if tool.InputSchema != nil && tool.InputSchema.Type != "" {
			if tool.InputSchema.Type != "object" {
				panic(fmt.Errorf("tool %q input schema must be object", tool.Name))
			}

			t.InputSchema = anthropic.BetaToolInputSchemaParam{
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			}
		}

		result[i] = anthropic.BetaToolUnionParam{OfTool: t}
	}

	return result
}

func mapBetaFinishReason(reason anthropic.BetaStopReason) agent.FinishReason {
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
