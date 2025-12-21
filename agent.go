package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/eolymp/go-agent/tracing"
	"golang.org/x/sync/errgroup"
)

type Agent struct {
	completer   ChatCompleter
	name        string
	description string
	tools       Toolset
	memory      Memory
	prompt      PromptLoader
	values      map[string]any                        // value for system prompt substitutions
	model       string                                // default model to use
	models      map[string]string                     // model name mapping
	iterations  int                                   // maximum number of iterations for agentic loop
	normalizer  []func(reply *AssistantMessage)       // agent output is expected to be structured, the system will retry if LLM produces non-json output
	finalizer   []func(reply *AssistantMessage) error // agent output is expected to be structured, the system will retry if LLM produces non-json output
}

func New(name string, prompt PromptLoader, opts ...Option) *Agent {
	a := &Agent{
		completer:  defaultCompleter,
		name:       name,
		prompt:     prompt,
		iterations: 120,
		tools:      NewStaticToolset(),
		memory:     NewStaticMemory(),
	}

	for _, opt := range opts {
		opt(a)
	}

	if a.completer == nil {
		panic("agent without completer, use WithCompleter option or SetDefaultCompleter")
	}

	return a
}

func (a Agent) Ask(ctx context.Context, opts ...Option) (err error) {
	c := a
	for _, opt := range opts {
		opt(&c)
	}

	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("agent %q", c.name), tracing.Kind(tracing.SpanTask))
	defer span.CloseWithError(err)

	var tools = c.tools.List()
	var prompt *Prompt
	var model = c.model

	if c.prompt != nil {
		prompt, err = a.prompt.Load(ctx)
		if err != nil {
			return fmt.Errorf("failed to load prompt: %w", err)
		}

		if prompt.Model != "" {
			model = prompt.Model
		}

		span.SetMetadata("model", model)
		span.SetMetadata("prompt_name", prompt.Name)
		span.SetMetadata("prompt_version", prompt.Version)
	}

loop:
	for i := 0; i < c.iterations; i++ {
		var messages []Message

		if prompt != nil {
			for _, p := range prompt.Messages {
				messages = append(messages, renderMessage(c.name, p, c.values))
			}
		}

		for _, message := range c.memory.List() {
			messages = append(messages, message)
		}

		req := CompletionRequest{
			Model:             model,
			Messages:          messages,
			Tools:             tools,
			ParallelToolCalls: true,
			ToolChoice:        ToolChoiceAuto,
		}

		resp, err := c.complete(ctx, req)
		if err != nil {
			return err
		}

		// Extract text and tool calls from content blocks
		switch resp.FinishReason {
		case FinishReasonToolCalls:
			var calls []CompletionToolCall

			for _, block := range resp.Content {
				switch block.Type {
				case ContentBlockTypeText:
					if block.Text != "" {
						c.memory.Append(AssistantMessage{Name: c.name, Content: block.Text})
					}

				case ContentBlockTypeToolUse:
					calls = append(calls, CompletionToolCall{ID: block.ID, Name: block.Name, Arguments: block.Arguments})
				}
			}

			if err := c.callTools(ctx, calls); err != nil {
				var ho Handoff
				if errors.As(err, &ho) {
					return ho.Agent.Ask(ctx, WithMemory(c.memory))
				}

				return err
			}

			continue
		default:
			for _, block := range resp.Content {
				if block.Type != ContentBlockTypeText {
					continue
				}

				reply := AssistantMessage{Name: c.name, Content: block.Text}

				// first normalize response
				for _, nn := range c.normalizer {
					nn(&reply)
				}

				c.memory.Append(reply)

				// make sure all finalizers are ok with the response
				for _, ff := range c.finalizer {
					if err := ff(&reply); err != nil {
						c.memory.Append(UserMessage{Content: "ERROR: " + err.Error()})
						continue loop
					}
				}
			}
		}

		break
	}

	return nil
}

func (a Agent) complete(ctx context.Context, req CompletionRequest) (resp *CompletionResponse, err error) {
	span, ctx := tracing.StartSpan(ctx, "chat_completion", tracing.Kind(tracing.SpanLLM), tracing.Input(req.Messages), tracing.Attr("model", req.Model))
	defer span.CloseWithError(err)

	if m, ok := a.models[req.Model]; ok {
		req.Model = m
	}

	resp, err = a.completer.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	span.SetOutput(resp.Content)
	span.SetMetric("tokens", float64(resp.Usage.TotalTokens))
	span.SetMetric("prompt_tokens", float64(resp.Usage.PromptTokens))
	span.SetMetric("completion_tokens", float64(resp.Usage.CompletionTokens))
	span.SetMetric("prompt_cached_tokens", float64(resp.Usage.CachedPromptTokens))

	return resp, nil
}

func (a Agent) callTools(ctx context.Context, calls []CompletionToolCall) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(5)

	results := make([]Message, len(calls))

	for index, call := range calls {
		index, call := index, call

		eg.Go(func() (err error) {
			span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("tool_call %q", call.Name), tracing.Kind(tracing.SpanTool), tracing.Input(json.RawMessage(call.Arguments)))
			defer span.Close()

			res, err := a.tools.Call(ctx, call.Name, []byte(call.Arguments))
			if errors.As(err, &Handoff{}) {
				return err
			}

			if err != nil {
				results[index] = ToolError{CallID: call.ID, Error: err}
				span.SetError(err)
				return nil
			}

			results[index] = ToolResult{CallID: call.ID, Result: res}
			span.SetOutput(res)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	// append conversation after all tools have finished
	msg := AssistantToolCall{}
	for _, call := range calls {
		msg.Calls = append(msg.Calls, &ToolCall{CallID: call.ID, Name: call.Name, Arguments: []byte(call.Arguments)})
	}

	a.memory.Append(msg)

	for _, result := range results {
		a.memory.Append(result)
	}

	return nil
}

// Memory provides access to agent's memory
// todo: not sure should be exposed
func (a Agent) Memory() Memory {
	return a.memory
}

func (a Agent) Name() string {
	return a.name
}
