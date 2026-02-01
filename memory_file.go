package agent

import (
	"context"
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
	_ = m.f.Close()
}

func (m *FileMemory) Append(ctx context.Context, msg Message) error {
	switch v := msg.(type) {
	case SystemMessage:
		_, _ = fmt.Fprintln(m.f, "System: ", v.Content)
	case UserMessage:
		_, _ = fmt.Fprintln(m.f, "User: ", v.Content)
	case AssistantMessage:
		_, _ = fmt.Fprintln(m.f, "Assistant: ", v.Text())
	case ToolResult:
		data, _ := json.Marshal(v.Result)
		_, _ = fmt.Fprintln(m.f, "Call result: ", string(data))
	case ToolError:
		_, _ = fmt.Fprintln(m.f, "Call error: ", v.Error)
	}

	return m.m.Append(ctx, msg)
}

func (m *FileMemory) List() []Message {
	return m.m.List()
}
