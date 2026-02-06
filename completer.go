package agent

import "context"

var defaultCompleter ChatCompleter

func SetChatCompleter(c ChatCompleter) {
	defaultCompleter = c
}

// ChatCompleter defines the interface for chat completion operations.
// This abstraction allows for different LLM providers (OpenAI, Anthropic, Google, etc.)
type ChatCompleter interface {
	// Complete performs a chat completion request and returns the response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// ToolChoice represents how the model should use tools during completion.
type ToolChoice int

const (
	// ToolChoiceAuto allows the model to decide whether to use tools
	ToolChoiceAuto ToolChoice = iota
	// ToolChoiceRequired forces the model to use at least one tool
	ToolChoiceRequired
	// ToolChoiceNone prevents the model from using any tools
	ToolChoiceNone
)

// String returns the string representation of ToolChoice for debugging.
func (t ToolChoice) String() string {
	switch t {
	case ToolChoiceAuto:
		return "auto"
	case ToolChoiceRequired:
		return "required"
	case ToolChoiceNone:
		return "none"
	default:
		return "unknown"
	}
}

// FinishReason indicates why the model stopped generating.
type FinishReason int

const (
	// FinishReasonStop indicates natural stop point
	FinishReasonStop FinishReason = iota
	// FinishReasonLength indicates max tokens reached
	FinishReasonLength
	// FinishReasonToolCalls indicates model wants to call tools
	FinishReasonToolCalls
	// FinishReasonContentFilter indicates content was filtered
	FinishReasonContentFilter
)

// String returns the string representation of FinishReason for debugging.
func (f FinishReason) String() string {
	switch f {
	case FinishReasonStop:
		return "stop"
	case FinishReasonLength:
		return "length"
	case FinishReasonToolCalls:
		return "tool_calls"
	case FinishReasonContentFilter:
		return "content_filter"
	default:
		return "unknown"
	}
}

type StreamChunk struct {
	Type         StreamChunkType
	Index        int
	Text         string
	Call         *ToolCall
	Usage        *CompletionUsage
	FinishReason FinishReason
}

type StreamChunkType int

const (
	StreamChunkTypeText             StreamChunkType = iota // a text delta
	StreamChunkTypeToolCallStart                           // the start of a new tool call (just call id and tool name)
	StreamChunkTypeToolCallDelta                           // a delta in tool call arguments
	StreamChunkTypeToolCallExecute                         // a tool is being executed (comes from agent, not LLM)
	StreamChunkTypeToolCallComplete                        // a tool has finished (comes from agent, not LLM)
	StreamChunkTypeUsage                                   // usage statistics update
	StreamChunkTypeFinish                                  // the completion has finished
)

func (s StreamChunkType) String() string {
	switch s {
	case StreamChunkTypeText:
		return "text"
	case StreamChunkTypeToolCallStart:
		return "tool_call_start"
	case StreamChunkTypeToolCallDelta:
		return "tool_call_delta"
	case StreamChunkTypeUsage:
		return "usage"
	case StreamChunkTypeFinish:
		return "finish"
	default:
		return "unknown"
	}
}

type Container struct {
	ID     string
	Skills []Skill
}

type Skill struct {
	SkillID string
	Type    string
	Version string
}

type ThinkingConfig struct {
	Enabled bool
	Budget  int
}

type CompletionRequest struct {
	Model             string
	Messages          []Message
	Tools             []Tool
	ToolChoice        ToolChoice
	ParallelToolCalls bool
	MaxTokens         *int
	Temperature       *float64
	TopP              *float64
	Container         *Container
	Betas             []string
	ThinkingConfig    *ThinkingConfig
	StreamCallback    func(ctx context.Context, chunk StreamChunk) error
}

// CompletionResponse represents a provider-agnostic chat completion response.
type CompletionResponse struct {
	Content      []AssistantMessageBlock
	FinishReason FinishReason
	Usage        CompletionUsage
	Model        string
}

// CompletionUsage represents token usage information for a completion.
type CompletionUsage struct {
	PromptTokens       int `json:"prompt_tokens,omitempty"`
	CompletionTokens   int `json:"completion_tokens,omitempty"`
	ThinkingTokens     int `json:"thinking_tokens,omitempty"`
	TotalTokens        int `json:"total_tokens,omitempty"`
	CachedPromptTokens int `json:"cached_prompt_tokens,omitempty"`
}
