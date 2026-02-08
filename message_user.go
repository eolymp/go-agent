package agent

import (
	"encoding/json"
)

type UserMessage struct {
	Content string `json:"content"`
}

func (m UserMessage) isMessage() {}

func NewUserMessage(text string) UserMessage {
	return UserMessage{Content: text}
}

type ToolResult struct {
	CallID string `json:"call_id"`
	Result any    `json:"result"`
}

func NewToolResult(callID string, result any) ToolResult {
	return ToolResult{CallID: callID, Result: result}
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
		data, _ := json.Marshal(c.Result)
		return string(data)
	}
}

type ToolError struct {
	CallID string `json:"call_id"`
	Error  error  `json:"error"`
}

func NewToolError(callID string, err error) ToolError {
	return ToolError{CallID: callID, Error: err}
}

func (c ToolError) isMessage() {}

func (c ToolError) String() string {
	return "ERROR: " + c.Error.Error()
}
