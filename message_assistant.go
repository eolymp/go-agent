package agent

import (
	"encoding/json"
	"errors"
	"strings"
)

type AssistantMessage struct {
	Content []AssistantMessageBlock `json:"content"`
}

func NewAssistantMessage(text ...string) AssistantMessage {
	content := make([]AssistantMessageBlock, len(text))
	for i, t := range text {
		content[i] = AssistantMessageBlock{Text: t}
	}

	return AssistantMessage{Content: content}
}

func (m AssistantMessage) isMessage() {}

func (m AssistantMessage) Text() string {
	var result strings.Builder
	for _, block := range m.Content {
		result.WriteString(block.Text)
	}

	return result.String()
}

func (m AssistantMessage) Unmarshal(v any) error {
	for _, b := range m.Content {
		if b.Call != nil {
			return errors.New("assistant message contains tool usage")
		}
	}

	return json.Unmarshal([]byte(strings.TrimPrefix(strings.Trim(m.Text(), "`"), "json")), v)
}

type AssistantMessageBlock struct {
	Text      string          `json:"text,omitempty"`
	Call      *ToolCall       `json:"call,omitempty"`
	Reasoning *ReasoningBlock `json:"reasoning,omitempty"`
}

// ReasoningBlock represents extended thinking content from models with reasoning capabilities.
// This includes thinking text, built-in tool usage (server_tool_use), and inline tool results.
type ReasoningBlock struct {
	Content   string      `json:"content,omitempty"`
	Signature string      `json:"signature,omitempty"`
	Call      *ToolCall   `json:"call,omitempty"`
	Result    *ToolResult `json:"result,omitempty"`
}

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
