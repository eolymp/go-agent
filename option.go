package agent

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/openai/openai-go"
)

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
		a.normalizer = append(a.normalizer, func(reply *AssistantMessage) {
			reply.Content = strings.TrimPrefix(strings.Trim(reply.Content, "`"), "json")
		})

		a.finalizer = append(a.finalizer, func(reply *AssistantMessage) error {
			if !json.Valid([]byte(reply.Content)) {
				return errors.New("response must be a valid JSON")
			}

			return nil
		})
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

func WithNormalizer(ff ...func(*AssistantMessage)) Option {
	return func(a *Agent) {
		a.normalizer = append(a.normalizer, ff...)
	}
}

func WithFinalizer(ff ...func(*AssistantMessage) error) Option {
	return func(a *Agent) {
		a.finalizer = append(a.finalizer, ff...)
	}
}
