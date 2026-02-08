package agent

import (
	"context"
	"fmt"
)

func WithBuiltinTool(name, kind string) Option {
	return WithTool(Tool{Name: name, Type: kind}, func(ctx context.Context, data []byte) (any, error) {
		return nil, fmt.Errorf("attempting to execute built-in tool %q", name)
	})
}
