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

// Complete implements ChatCompleter by delegating to the Anthropic client.
func (c *Completer) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if req.StreamCallback != nil {
		if len(req.Betas) > 0 || req.Container != nil || req.ThinkingConfig != nil {
			return c.betaStream(ctx, req)
		}

		return c.stream(ctx, req)
	}

	if len(req.Betas) > 0 || req.Container != nil || req.ThinkingConfig != nil {
		resp, err := c.client.Beta.Messages.New(ctx, toBetaAnthropicRequest(req))
		if err != nil {
			return nil, err
		}

		return fromBetaAnthropicResponse(resp), nil
	}

	resp, err := c.client.Messages.New(ctx, toAnthropicRequest(req))
	if err != nil {
		return nil, err
	}

	return fromAnthropicResponse(resp), nil
}

// stream handles streaming completion with callback support.
func (c *Completer) stream(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	stream := c.client.Messages.NewStreaming(ctx, toAnthropicRequest(req))
	defer stream.Close()

	resp := &agent.CompletionResponse{}
	blocks := make(map[int]*agent.AssistantMessageBlock)

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
			block := &agent.AssistantMessageBlock{}

			switch event.ContentBlock.Type {
			case "text":
			case "tool_use":
				block.Call = &agent.ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}

				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeToolCallStart,
					Index: index,
					Call:  block.Call,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			}

			blocks[index] = block

		case "content_block_delta":
			index := int(event.Index)
			block := blocks[index]

			switch event.Delta.Type {
			case "text_delta":
				block.Text += event.Delta.Text
				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeText,
					Index: index,
					Text:  event.Delta.Text,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}

			case "input_json_delta":
				block.Call.Arguments += event.Delta.PartialJSON
				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeToolCallDelta,
					Index: index,
					Call:  &agent.ToolCall{ID: block.Call.ID, Name: block.Call.Name, Arguments: event.Delta.PartialJSON},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			}

		case "content_block_stop":
		case "message_delta":
			resp.Usage.CompletionTokens = int(event.Usage.OutputTokens)
			resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens

			if event.Delta.StopReason != "" {
				resp.FinishReason = mapFinishReason(event.Delta.StopReason)
			}

			chunk := agent.StreamChunk{
				Type:  agent.StreamChunkTypeUsage,
				Usage: &resp.Usage,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}

		case "message_stop":
			reason := resp.FinishReason
			chunk := agent.StreamChunk{
				Type:         agent.StreamChunkTypeFinish,
				FinishReason: reason,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}
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

	resp.Content = make([]agent.AssistantMessageBlock, length+1)
	for idx, block := range blocks {
		resp.Content[idx] = *block
	}

	return resp, nil
}

func (c *Completer) betaStream(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	stream := c.client.Beta.Messages.NewStreaming(ctx, toBetaAnthropicRequest(req))
	defer stream.Close()

	resp := &agent.CompletionResponse{}
	blocks := make(map[int]*agent.AssistantMessageBlock)

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			resp.Model = string(event.Message.Model)
			resp.Usage.PromptTokens = int(event.Message.Usage.InputTokens)
			resp.Usage.CachedPromptTokens = int(event.Message.Usage.CacheReadInputTokens)

		case "content_block_start":
			index := int(event.Index)
			block := &agent.AssistantMessageBlock{}

			switch event.ContentBlock.Type {
			case "text":
			case "tool_use":
				block.Call = &agent.ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}

				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeToolCallStart,
					Index: index,
					Call:  block.Call,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			}

			blocks[index] = block

		case "content_block_delta":
			index := int(event.Index)
			block := blocks[index]

			switch event.Delta.Type {
			case "text_delta":
				block.Text += event.Delta.Text
				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeText,
					Index: index,
					Text:  event.Delta.Text,
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}

			case "input_json_delta":
				block.Call.Arguments += event.Delta.PartialJSON
				chunk := agent.StreamChunk{
					Type:  agent.StreamChunkTypeToolCallDelta,
					Index: index,
					Call:  &agent.ToolCall{ID: block.Call.ID, Name: block.Call.Name, Arguments: event.Delta.PartialJSON},
				}

				if err := req.StreamCallback(ctx, chunk); err != nil {
					return nil, err
				}
			}

		case "content_block_stop":
		case "message_delta":
			resp.Usage.CompletionTokens = int(event.Usage.OutputTokens)
			resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens

			if event.Delta.StopReason != "" {
				resp.FinishReason = mapBetaFinishReason(event.Delta.StopReason)
			}

			chunk := agent.StreamChunk{
				Type:  agent.StreamChunkTypeUsage,
				Usage: &resp.Usage,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}

		case "message_stop":
			reason := resp.FinishReason
			chunk := agent.StreamChunk{
				Type:         agent.StreamChunkTypeFinish,
				FinishReason: reason,
			}

			if err := req.StreamCallback(ctx, chunk); err != nil {
				return nil, err
			}
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

	resp.Content = make([]agent.AssistantMessageBlock, length+1)
	for idx, block := range blocks {
		resp.Content[idx] = *block
	}

	return resp, nil
}

// toAnthropicRequest converts a universal CompletionRequest to Anthropic-specific params.
func toAnthropicRequest(req agent.CompletionRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: 8192}

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
				case block.Call != nil:
					var input map[string]interface{}
					_ = json.Unmarshal([]byte(block.Call.Arguments), &input)
					content[i] = anthropic.NewToolUseBlock(block.Call.ID, input, block.Call.Name)
				case block.Text != "":
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
		}
	}

	return params
}

// fromAnthropicResponse converts an Anthropic response to a universal CompletionResponse.
func fromAnthropicResponse(resp *anthropic.Message) *agent.CompletionResponse {
	ar := &agent.CompletionResponse{
		Model:        string(resp.Model),
		Content:      make([]agent.AssistantMessageBlock, len(resp.Content)),
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
			ar.Content[i] = agent.AssistantMessageBlock{
				Text: b.Text,
			}
		case "tool_use":
			ar.Content[i] = agent.AssistantMessageBlock{
				Call: &agent.ToolCall{
					ID:        b.ID,
					Name:      b.Name,
					Arguments: string(b.Input),
				},
			}
		}
	}

	return ar
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
		params.MaxTokens = int64(*req.MaxTokens)
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	if req.TopP != nil {
		params.TopP = param.NewOpt(*req.TopP)
	}

	if len(req.Betas) > 0 {
		params.Betas = req.Betas
	}

	if req.Container != nil {
		skills := make([]anthropic.BetaSkillParams, len(req.Container.Skills))
		for i, skill := range req.Container.Skills {
			skills[i] = anthropic.BetaSkillParams{
				SkillID: skill.SkillID,
				Type:    anthropic.BetaSkillParamsType(skill.Type),
			}
			if skill.Version != "" {
				skills[i].Version = param.NewOpt(skill.Version)
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
				switch {
				case block.Call != nil:
					var input map[string]interface{}
					_ = json.Unmarshal([]byte(block.Call.Arguments), &input)
					content[i] = anthropic.NewBetaToolUseBlock(block.Call.ID, input, block.Call.Name)
				case block.Text != "":
					content[i] = anthropic.NewBetaTextBlock(block.Text)
				}
			}

			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role:    "assistant",
				Content: content,
			})

		case agent.ToolResult:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role: "user",
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.BetaContentBlockParamUnion{
						OfToolResult: &anthropic.BetaToolResultBlockParam{
							ToolUseID: m.CallID,
							Content: []anthropic.BetaToolResultBlockParamContentUnion{
								anthropic.BetaToolResultBlockParamContentUnion{
									OfText: &anthropic.BetaTextBlockParam{
										Type: "text",
										Text: m.String(),
									},
								},
							},
						},
					},
				},
			})

		case agent.ToolError:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role: "user",
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.BetaContentBlockParamUnion{
						OfToolResult: &anthropic.BetaToolResultBlockParam{
							ToolUseID: m.CallID,
							IsError:   param.NewOpt(true),
							Content: []anthropic.BetaToolResultBlockParamContentUnion{
								anthropic.BetaToolResultBlockParamContentUnion{
									OfText: &anthropic.BetaTextBlockParam{
										Type: "text",
										Text: m.String(),
									},
								},
							},
						},
					},
				},
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
		}
	}

	if req.ThinkingConfig != nil {
		if req.ThinkingConfig.Enabled {
			budget := int64(1024)
			if req.ThinkingConfig.Budget > 0 {
				budget = int64(req.ThinkingConfig.Budget)
			}
			params.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
		} else {
			disabled := anthropic.NewBetaThinkingConfigDisabledParam()
			params.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfDisabled: &disabled,
			}
		}
	}

	return params
}

func fromBetaAnthropicResponse(resp *anthropic.BetaMessage) *agent.CompletionResponse {
	ar := &agent.CompletionResponse{
		Model:        string(resp.Model),
		Content:      make([]agent.AssistantMessageBlock, len(resp.Content)),
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
			ar.Content[i] = agent.AssistantMessageBlock{
				Text: b.Text,
			}
		case "tool_use":
			ar.Content[i] = agent.AssistantMessageBlock{
				Call: &agent.ToolCall{
					ID:        b.ID,
					Name:      b.Name,
					Arguments: string(b.Input),
				},
			}
		}
	}

	return ar
}

func toBetaAnthropicTools(tools []agent.Tool) []anthropic.BetaToolUnionParam {
	result := make([]anthropic.BetaToolUnionParam, len(tools))

	for i, tool := range tools {
		t := &anthropic.BetaToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(tool.Description),
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
