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
	Name    string
	Content string
}

func (m SystemMessage) isMessage() {}

type AssistantMessage struct {
	Name    string
	Content string
}

func (m AssistantMessage) isMessage() {}

func (m AssistantMessage) Unmarshal(v any) error {
	return json.Unmarshal([]byte(strings.TrimPrefix(strings.Trim(m.Content, "`"), "json")), v)
}

type AssistantToolCall struct {
	Calls []*ToolCall
}

func (m AssistantToolCall) isMessage() {}

type UserMessage struct {
	Name    string
	Content string
}

func (m UserMessage) isMessage() {}

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

type ToolError struct {
	CallID string
	Error  error
}

func (c ToolError) isMessage() {}

func (c ToolError) String() string {
	return "ERROR: " + c.Error.Error()
}
