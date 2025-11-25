package agent

import "github.com/openai/openai-go"

type Option func(*Agent)

func WithMemory(memory Memory) Option {
	return func(a *Agent) {
		a.memory = memory
	}
}

func WithToolset(tools Toolset) Option {
	return func(a *Agent) {
		a.tools = tools
	}
}

func WithModel(model string) Option {
	return func(a *Agent) {
		a.model = model
	}
}

func WithDescription(desc string) Option {
	return func(a *Agent) {
		a.description = desc
	}
}

func WithClient(c openai.Client) Option {
	return func(a *Agent) {
		a.cli = c
	}
}

func WithValues(values map[string]any) Option {
	return func(a *Agent) {
		if a.values == nil {
			a.values = make(map[string]any, len(values))
		}

		for k, v := range values {
			a.values[k] = v
		}
	}
}

func WithStructuredOutput() Option {
	return func(a *Agent) {
		a.structured = true
	}
}

func WithOptions(opts ...Option) Option {
	return func(a *Agent) {
		for _, opt := range opts {
			opt(a)
		}
	}
}

func WithModelMapper(mapping map[string]string) Option {
	return func(a *Agent) {
		a.models = mapping
	}
}
