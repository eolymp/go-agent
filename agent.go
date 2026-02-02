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
	values      map[string]any    // value for system prompt substitutions
	model       string            // default model to use
	models      map[string]string // model name mapping
	iterations  int               // maximum number of iterations for agentic loop
	approver    []func(call ToolCall) ToolCallApproval
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

func (a Agent) Name() string {
	return a.name
}

func (a Agent) Memory() Memory {
	return a.memory
}

// Ask is deprecated, use Run instead.
func (a Agent) Ask(ctx context.Context, opts ...Option) (err error) {
	_, err = a.Run(ctx, opts...)
	return err
}

func (a Agent) Run(ctx context.Context, opts ...Option) (reply AssistantMessage, err error) {
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
		prompt, err = c.prompt.Load(ctx)
		if err != nil {
			return reply, fmt.Errorf("failed to load prompt: %w", err)
		}

		if prompt.Model != "" {
			model = prompt.Model
		}

		span.SetMetadata("model", model)
		span.SetMetadata("prompt_name", prompt.Name)
		span.SetMetadata("prompt_version", prompt.Version)
	}

	// if last message is from assistant, try calling tools
	if last, ok := LastMessageAsAssistant(c.memory); ok {
		if err := c.call(ctx, last); err != nil {
			return last, err
		}
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

		resp, err := c.complete(ctx, CompletionRequest{
			Model:             model,
			Messages:          messages,
			Tools:             tools,
			ParallelToolCalls: true,
			ToolChoice:        ToolChoiceAuto,
		})

		if err != nil {
			return reply, err
		}

		// convert completion response to assistant message
		reply = AssistantMessage{Content: resp.Content}

		if err := c.memory.Append(ctx, reply); err != nil {
			return reply, err
		}

		switch resp.FinishReason {
		case FinishReasonToolCalls:
			// call tools
			if err := c.call(ctx, reply); err != nil {
				return reply, err
			}

			continue
		default:
			// first normalize response
			for _, n := range c.normalizer {
				n(&reply)
			}

			// make sure all finalizers are ok with the response
			for _, f := range c.finalizer {
				if err := f(&reply); err != nil {
					if err := c.memory.Append(ctx, NewUserMessage("ERROR: "+err.Error())); err != nil {
						return reply, err
					}

					continue loop
				}
			}
		}

		break
	}

	return reply, nil
}

func (a Agent) complete(ctx context.Context, req CompletionRequest) (resp *CompletionResponse, err error) {
	span, ctx := tracing.StartSpan(ctx, "chat_completion", tracing.Kind(tracing.SpanLLM), tracing.Input(req.Messages), tracing.Attr("model", req.Model))
	defer span.CloseWithError(err)

	if m, ok := a.models[req.Model]; ok {
		req.Model = m
	}

	if s, ok := a.memory.(StreamingMemory); ok {
		req.StreamCallback = s.Chunk
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

func (a Agent) call(ctx context.Context, reply AssistantMessage) error {
	var calls []ToolCall
	var undecided []ToolCall
	approved := map[string]bool{}

	// verify approvals for tool calls
	for _, block := range reply.Content {
		if block.Call == nil {
			continue
		}

		calls = append(calls, *block.Call)

		switch a.approve(*block.Call) {
		case ToolCallUndecided:
			undecided = append(undecided, *block.Call)
		case ToolCallApproved:
			approved[block.Call.ID] = true
		default:
			continue
		}
	}

	if len(undecided) > 0 {
		return ToolApprovalRequest{Calls: undecided}
	}

	// execute all tool calls
	results := make([]Message, len(calls))

	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(5)

	for index, call := range calls {
		index, call := index, call

		eg.Go(func() (err error) {
			span, gctx := tracing.StartSpan(gctx, fmt.Sprintf("tool_call %q", call.Name), tracing.Kind(tracing.SpanTool), tracing.Input(json.RawMessage(call.Arguments)))
			defer span.Close()

			var result any

			if approved[call.ID] {
				result, err = a.tools.Call(gctx, call.Name, []byte(call.Arguments))
			} else {
				err = errors.New("tool call has been rejected by the user")
			}

			if err != nil {
				span.SetError(err)
				if errors.As(err, &Handoff{}) {
					return err
				}

				results[index] = NewToolError(call.ID, err)
				return nil
			}

			span.SetOutput(result)
			results[index] = NewToolResult(call.ID, result)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	// write down tool execution results
	var errs []error
	for _, result := range results {
		if err := a.memory.Append(ctx, result); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (a Agent) approve(call ToolCall) ToolCallApproval {
	approved := false
	for _, p := range a.approver {
		switch p(call) {
		case ToolCallRejected: // reject immediately
			return ToolCallRejected
		case ToolCallApproved:
			approved = true
		default:
			continue
		}
	}

	if approved {
		return ToolCallApproved
	}

	return ToolCallUndecided
}
