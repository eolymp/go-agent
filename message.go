package agent

import (
	"encoding/json"
	"strings"
)

type Role string

type Message interface {
	isMessage()
}

type ToolCall struct {
	CallID    string
	Name      string
	Arguments []byte
}

type SystemMessage struct {
	Content string
}

func (m SystemMessage) isMessage() {}

// NewSystemMessage creates a new system message with the given text.
func NewSystemMessage(text string) SystemMessage {
	return SystemMessage{Content: text}
}

type AssistantMessage struct {
	Content []ContentBlock
}

func (m AssistantMessage) isMessage() {}

// Text returns the concatenated text from all text content blocks.
func (m AssistantMessage) Text() string {
	var result strings.Builder
	for _, block := range m.Content {
		if block.Type == ContentBlockTypeText {
			result.WriteString(block.Text)
		}
	}

	return result.String()
}

// Unmarshal attempts to unmarshal the text content as JSON.
func (m AssistantMessage) Unmarshal(v any) error {
	return json.Unmarshal([]byte(strings.TrimPrefix(strings.Trim(m.Text(), "`"), "json")), v)
}

// NewAssistantMessage creates a new assistant message with text blocks.
// Tool calls are created internally by the system and should not be manually constructed.
func NewAssistantMessage(text ...string) AssistantMessage {
	content := make([]ContentBlock, len(text))
	for i, t := range text {
		content[i] = ContentBlock{Type: ContentBlockTypeText, Text: t}
	}
	
	return AssistantMessage{Content: content}
}

type UserMessage struct {
	Content string
}

func (m UserMessage) isMessage() {}

// NewUserMessage creates a new user message with the given text.
func NewUserMessage(text string) UserMessage {
	return UserMessage{Content: text}
}

type ToolResult struct {
	CallID string
	Result any
}

func (c ToolResult) isMessage() {}

func (c ToolResult) String() string {
	switch o := c.Result.(type) {
	case nil:
		return ""
	case string:
		return o
	case []byte:
		return string(o)
	default:
		jsn, _ := json.Marshal(c.Result)
		return string(jsn)
	}
}

// NewToolResult creates a new tool result message.
func NewToolResult(callID string, result any) ToolResult {
	return ToolResult{CallID: callID, Result: result}
}

type ToolError struct {
	CallID string
	Error  error
}

func (c ToolError) isMessage() {}

func (c ToolError) String() string {
	return "ERROR: " + c.Error.Error()
}

// NewToolError creates a new tool error message.
func NewToolError(callID string, err error) ToolError {
	return ToolError{CallID: callID, Error: err}
}
