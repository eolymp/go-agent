package agent

import (
	"encoding/json"
	"fmt"
	"os"
)

type FileMemory struct {
	m Memory
	f *os.File
}

func NewFileMemory(fn string, m Memory) *FileMemory {
	f, err := os.Create(fn)
	if err != nil {
		panic(err)
	}
	return &FileMemory{f: f, m: m}
}

func (m *FileMemory) Close() {
	m.f.Close()
}

func (m *FileMemory) Append(msg Message) {
	switch v := msg.(type) {
	case SystemMessage:
		fmt.Fprintln(m.f, "System: ", v.Name, v.Content)
	case UserMessage:
		fmt.Fprintln(m.f, "User: ", v.Content)
	case AssistantMessage:
		fmt.Fprintln(m.f, "Assistant: ", v.Name, v.Content)
	case AssistantToolCall:
		for _, call := range v.Calls {
			fmt.Println("Tool call: ", call.Name, string(call.Arguments))
		}
	case ToolResult:
		data, _ := json.Marshal(v.Result)
		fmt.Fprintln(m.f, "Tool result: ", string(data))
	case ToolError:
		fmt.Fprintln(m.f, "Tool error: ", v.Error)
	}

	m.m.Append(msg)
}

func (m *FileMemory) Last() Message {
	return m.m.Last()
}

func (m *FileMemory) List() []Message {
	return m.m.List()
}
