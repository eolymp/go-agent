package agent

import (
	"encoding/json"
	"errors"
	"strings"
)

type AssistantMessage struct {
	Content []MessageBlock `json:"content"`
}

func NewAssistantMessage(text ...string) AssistantMessage {
	content := make([]MessageBlock, len(text))
	for i, t := range text {
		content[i] = MessageBlock{Type: MessageBlockTypeText, Text: t}
	}

	return AssistantMessage{Content: content}
}

func (m AssistantMessage) isMessage() {}

func (m AssistantMessage) Text() string {
	var result strings.Builder
	for _, block := range m.Content {
		if block.Type == MessageBlockTypeText {
			result.WriteString(block.Text)
		}
	}

	return result.String()
}

func (m AssistantMessage) Reasoning() string {
	var result strings.Builder
	for _, block := range m.Content {
		if block.Type == MessageBlockTypeReasoning {
			result.WriteString(block.Text)
		}
	}

	return result.String()
}

func (m AssistantMessage) Unmarshal(v any) error {
	for _, b := range m.Content {
		if b.Type == MessageBlockTypeToolCall {
			return errors.New("assistant message contains tool usage")
		}
	}

	return json.Unmarshal([]byte(strings.TrimPrefix(strings.Trim(m.Text(), "`"), "json")), v)
}

type MessageBlock struct {
	Type       MessageBlockType `json:"type"`
	Text       string           `json:"text,omitempty"`
	Signature  string           `json:"signature,omitempty"`
	ToolCall   *ToolCall        `json:"toolcall,omitempty"`
	ToolResult *ToolResult      `json:"tool_result,omitempty"`
}

type MessageBlockType string

const (
	MessageBlockTypeText           MessageBlockType = "text"
	MessageBlockTypeToolCall       MessageBlockType = "tool_call"
	MessageBlockTypeReasoning      MessageBlockType = "reasoning"
	MessageBlockTypeSignature      MessageBlockType = "signature"
	MessageBlockTypeServerToolCall MessageBlockType = "server_tool_call"
	MessageBlockTypeToolResult     MessageBlockType = "tool_result"
)

type ToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ToolCallApproval int

const (
	ToolCallUndecided ToolCallApproval = iota
	ToolCallApproved
	ToolCallRejected
)

type ToolApprovalRequest struct {
	Calls []ToolCall `json:"calls,omitempty"`
}

func (r ToolApprovalRequest) Error() string {
	var names []string
	for _, call := range r.Calls {
		names = append(names, call.Name)
	}

	return "tool approval is required: " + strings.Join(names, ", ")
}
