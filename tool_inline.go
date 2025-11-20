package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

func WithInlineTool[In any, Out any](name, desc string, fn func(context.Context, In) (Out, error)) AgentOption {
	is, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Errorf("failed to make input schema for %T: %v", *new(In), err))
	}

	os, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Errorf("failed to make input schema for %T: %v", *new(Out), err))
	}

	tool := Tool{
		Name:         name,
		Description:  desc,
		InputSchema:  is,
		OutputSchema: os,
	}

	return WithTool(tool, func(ctx context.Context, data []byte) (any, error) {
		var in In
		if err := json.Unmarshal(data, &in); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool arguments: %w", err)
		}

		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}

		return out, err
	})
}
