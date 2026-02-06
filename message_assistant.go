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

func (m AssistantMessage) render(values map[string]any) Message {
	content := make([]AssistantMessageBlock, len(m.Content))
	for i, block := range m.Content {
		content[i] = block
		if block.Text != "" {
			content[i].Text = MessageRender(block.Text, values)
		}
	}

	return AssistantMessage{Content: content}
}

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
	Text string    `json:"text,omitempty"`
	Call *ToolCall `json:"call,omitempty"`
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
