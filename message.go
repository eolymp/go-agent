package agent

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

type Role string

type Message interface {
	toOpenAIMessage() openai.ChatCompletionMessageParamUnion
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

func (m SystemMessage) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	system := &openai.ChatCompletionSystemMessageParam{
		Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: param.NewOpt(m.Content)},
	}

	if m.Name != "" {
		system.Name = param.NewOpt(normalizeName(m.Name))
	}

	return openai.ChatCompletionMessageParamUnion{OfSystem: system}
}

type AssistantMessage struct {
	Name    string
	Content string
}

func (m AssistantMessage) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	assistant := &openai.ChatCompletionAssistantMessageParam{
		Content: openai.ChatCompletionAssistantMessageParamContentUnion{OfString: param.NewOpt(m.Content)},
	}

	if m.Name != "" {
		assistant.Name = param.NewOpt(normalizeName(m.Name))
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: assistant}
}

func (m AssistantMessage) Unmarshal(v any) error {
	return json.Unmarshal([]byte(strings.TrimPrefix(strings.Trim(m.Content, "`"), "json")), v)
}

type AssistantToolCall struct {
	Calls []*ToolCall
}

func (m AssistantToolCall) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	var msg openai.ChatCompletionAssistantMessageParam

	msg.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(m.Calls))

	for i, v := range m.Calls {
		msg.ToolCalls[i].ID = v.CallID
		msg.ToolCalls[i].Function.Arguments = string(v.Arguments)
		msg.ToolCalls[i].Function.Name = v.Name
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &msg}
}

type UserMessage struct {
	Name    string
	Content string
}

func (m UserMessage) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	user := &openai.ChatCompletionUserMessageParam{
		Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: param.NewOpt(m.Content)},
	}

	if m.Name != "" {
		user.Name = param.NewOpt(normalizeName(m.Name))
	}

	return openai.ChatCompletionMessageParamUnion{OfUser: user}
}

type ToolResult struct {
	CallID string
	Result any
}

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

func (c ToolResult) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	return openai.ToolMessage(c.String(), c.CallID)
}

type ToolError struct {
	CallID string
	Error  error
}

func (c ToolError) String() string {
	return "ERROR: " + c.Error.Error()
}

func (c ToolError) toOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	return openai.ToolMessage(c.String(), c.CallID)
}
