package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Option func(*Agent)

// OptionLoader is called in the beginning of the agentic loop to load dynamic options
type OptionLoader func(ctx context.Context, agent *Agent) error

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

func WithChatCompleter(completer ChatCompleter) Option {
	return func(a *Agent) {
		a.completer = completer
	}
}

// SystemPrompt is for backwards compatibility
// Deprecated, use WithSystemMessage instead
func SystemPrompt(text string) Option {
	return func(a *Agent) {
		a.messages = append(a.messages, SystemMessage{Content: text})
	}
}

func WithSystemMessage(text string) Option {
	return func(a *Agent) {
		a.messages = append(a.messages, SystemMessage{Content: text})
	}
}

func WithUserMessage(text string) Option {
	return func(a *Agent) {
		a.messages = append(a.messages, UserMessage{Content: text})
	}
}

func WithAssistantMessage(text string) Option {
	return func(a *Agent) {
		a.messages = append(a.messages, AssistantMessage{
			Content: []MessageBlock{{Type: MessageBlockTypeText, Text: text}},
		})
	}
}

func WithMessages(messages []Message) Option {
	return func(a *Agent) {
		a.messages = append(a.messages, messages...)
	}
}

func WithOptionLoader(loaders ...OptionLoader) Option {
	return func(a *Agent) {
		a.dynamics = append(a.dynamics, loaders...)
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
		a.finalizer = append(a.finalizer, func(reply *AssistantMessage) error {
			text := reply.Text()
			text = strings.TrimPrefix(strings.Trim(text, "`"), "json")

			if !json.Valid([]byte(text)) {
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

func WithFinalizer(ff ...func(*AssistantMessage) error) Option {
	return func(a *Agent) {
		a.finalizer = append(a.finalizer, ff...)
	}
}

func WithApprover(aa ...func(call ToolCall) ToolCallApproval) Option {
	return func(a *Agent) {
		a.approver = append(a.approver, aa...)
	}
}

func WithToolParallelism(limit int) Option {
	return func(a *Agent) {
		a.parallelism = limit
	}
}

func WithBetas(betas ...string) Option {
	return func(a *Agent) {
		a.betas = append(a.betas, betas...)
	}
}

func WithContainer(container *Container) Option {
	return func(a *Agent) {
		a.container = container
	}
}

func WithReasoning(config *Reasoning) Option {
	return func(a *Agent) {
		a.reasoning = config
	}
}

func WithTemperature(temperature float32) Option {
	return func(a *Agent) {
		a.temperature = &temperature
	}
}

func WithMaxTokens(maxTokens int64) Option {
	return func(a *Agent) {
		a.maxTokens = &maxTokens
	}
}

func WithTopP(topP float32) Option {
	return func(a *Agent) {
		a.topP = &topP
	}
}

func WithTopK(topK int32) Option {
	return func(a *Agent) {
		a.topK = &topK
	}
}

func WithUseCache(useCache bool) Option {
	return func(a *Agent) {
		a.useCache = &useCache
	}
}

// WithApprovals creates approver which approves specific calls
func WithApprovals(calls ...string) Option {
	m := map[string]bool{}
	for _, call := range calls {
		m[call] = true
	}

	return WithApprover(func(call ToolCall) ToolCallApproval {
		if m[call.ID] {
			return ToolCallApproved
		}

		return ToolCallUndecided
	})
}

// WithRejections creates approver which rejects specific calls
func WithRejections(calls ...string) Option {
	m := map[string]bool{}
	for _, call := range calls {
		m[call] = true
	}

	return WithApprover(func(call ToolCall) ToolCallApproval {
		if m[call.ID] {
			return ToolCallRejected
		}

		return ToolCallUndecided
	})
}

// WithAutoApproveAll creates approver which approves all calls automatically
func WithAutoApproveAll() Option {
	return WithApprover(func(call ToolCall) ToolCallApproval {
		return ToolCallApproved
	})
}

// WithAutoApproveTools creates approver which approves calls for specific tools automatically
func WithAutoApproveTools(names ...string) Option {
	m := map[string]bool{}
	for _, name := range names {
		m[name] = true
	}

	return WithApprover(func(call ToolCall) ToolCallApproval {
		if m[call.Name] {
			return ToolCallApproved
		}

		return ToolCallUndecided
	})
}
