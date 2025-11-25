package agent

import (
	"context"
	"errors"

	"github.com/google/jsonschema-go/jsonschema"
)

type Tool struct {
	Name         string
	Description  string
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
}

func WithTool(tool Tool, fn func(context.Context, []byte) (any, error)) Option {
	return func(a *Agent) {
		adder, ok := a.tools.(interface {
			Add(t Tool, h ToolHandlerFunc)
		})

		if !ok {
			panic(errors.New("toolset does not allow adding tools"))
		}

		adder.Add(tool, fn)
	}
}
