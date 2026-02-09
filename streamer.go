package agent

import "context"

type Streamer interface {
	Stream(ctx context.Context, chunk Chunk) error
}

type Chunk struct {
	Type         StreamChunkType
	Index        int
	Text         string      // For text and thinking deltas
	Call         *ToolCall   // For tool calls (both user and server tools)
	Signature    string      // For thinking signature
	Result       *ToolResult // For inline tool results
	Usage        *CompletionUsage
	FinishReason FinishReason
}

type StreamChunkType int

const (
	StreamChunkTypeText                StreamChunkType = iota // a text delta
	StreamChunkTypeToolCallStart                              // the start of a new tool call (just call id and tool name)
	StreamChunkTypeToolCallDelta                              // a delta in tool call arguments
	StreamChunkTypeToolCallExecute                            // a tool is being executed (comes from agent, not LLM)
	StreamChunkTypeToolCallComplete                           // a tool has finished (comes from agent, not LLM)
	StreamChunkTypeReasoning                                  // thinking content delta (extended reasoning)
	StreamChunkTypeSignature                                  // thinking signature for verification
	StreamChunkTypeServerToolCallStart                        // built-in tool call start (web_search, bash, etc.)
	StreamChunkTypeServerToolCallDelta                        // built-in tool call arguments delta
	StreamChunkTypeToolResult                                 // inline tool result from server
	StreamChunkTypeUsage                                      // usage statistics update
	StreamChunkTypeFinish                                     // the completion has finished
)

func (s StreamChunkType) String() string {
	switch s {
	case StreamChunkTypeText:
		return "text"
	case StreamChunkTypeToolCallStart:
		return "tool_call_start"
	case StreamChunkTypeToolCallDelta:
		return "tool_call_delta"
	case StreamChunkTypeToolCallExecute:
		return "tool_call_execute"
	case StreamChunkTypeToolCallComplete:
		return "tool_call_complete"
	case StreamChunkTypeReasoning:
		return "thinking_delta"
	case StreamChunkTypeSignature:
		return "thinking_signature"
	case StreamChunkTypeServerToolCallStart:
		return "server_tool_call_start"
	case StreamChunkTypeServerToolCallDelta:
		return "server_tool_call_delta"
	case StreamChunkTypeToolResult:
		return "tool_result"
	case StreamChunkTypeUsage:
		return "usage"
	case StreamChunkTypeFinish:
		return "finish"
	default:
		return "unknown"
	}
}
