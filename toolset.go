package agent

import (
	"context"
	"fmt"
)

type Toolset interface {
	Call(ctx context.Context, function string, args []byte) (any, error)
	List() []Tool
}

type ToolHandlerFunc func(context.Context, []byte) (any, error)

type StaticToolset struct {
	tools    []Tool
	handlers map[string]ToolHandlerFunc
}

func NewStaticToolset() *StaticToolset {
	return &StaticToolset{handlers: make(map[string]ToolHandlerFunc)}
}

func (t *StaticToolset) Call(ctx context.Context, function string, args []byte) (any, error) {
	h, ok := t.handlers[function]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", function)
	}

	return h(ctx, args)
}

func (t *StaticToolset) List() []Tool {
	return t.tools
}

func (t *StaticToolset) Add(tool Tool, handler ToolHandlerFunc) {
	t.tools = append(t.tools, tool)
	t.handlers[tool.Name] = handler
}
