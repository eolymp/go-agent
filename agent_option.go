package agent

type AgentOption func(*Agent)

func WithMemory(memory Memory) AgentOption {
	return func(a *Agent) {
		a.memory = memory
	}
}

func WithToolset(tools Toolset) AgentOption {
	return func(a *Agent) {
		a.tools = tools
	}
}

func WithModel(model string) AgentOption {
	return func(a *Agent) {
		a.model = model
	}
}

func WithDescription(desc string) AgentOption {
	return func(a *Agent) {
		a.description = desc
	}
}

func WithValues(values map[string]any) AgentOption {
	return func(a *Agent) {
		a.values = values
	}
}

func WithStructuredOutput() AgentOption {
	return func(a *Agent) {
		a.structured = true
	}
}

func WithOptions(opts ...AgentOption) AgentOption {
	return func(a *Agent) {
		for _, opt := range opts {
			opt(a)
		}
	}
}
