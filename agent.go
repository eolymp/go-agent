package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/eolymp/go-agent/tracing"
	"golang.org/x/sync/errgroup"
)

type Agent struct {
	completer   ChatCompleter                          // chat completer used to complete agentic request
	name        string                                 // agent name
	description string                                 // agent description
	tools       Toolset                                // toolset for the agent
	memory      Memory                                 // memory provides a backend for storing conversation history between turns
	messages    []Message                              // list of starter messages are added before the messages from memory, this is normally a system message
	values      map[string]any                         // values for template substitution in messages
	model       string                                 // model to be used for completion
	models      map[string]string                      // deprecated, to be moved to completer, additional mapping for model name (probably should be in completer :thinking:...)
	temperature *float32                               // temperature parameter for completion
	maxTokens   *int64                                 // max tokens parameter for completion
	topP        *float32                               // top_p parameter for completion
	topK        *int32                                 // top_k parameter for completion
	useCache    *bool                                  // use prompt caching (Anthropic specific)
	iterations  int                                    // max number of iterations for agentic loop
	parallelism int                                    // number of tool calls executed in parallel, 1 - sequential run, -1 - no limit on parallelism
	betas       []string                               // additional flags to enable beta features
	container   *Container                             // container to be used for LLM (only available in Anthropic models)
	reasoning   *Reasoning                             // reasoning configuration (only supported by Anthropic models)
	dynamics    []OptionLoader                         // lazy loaded options are loaded just before executing agentic loop to define dynamic parameters (load from an external backend)
	approver    []func(call ToolCall) ToolCallApproval // approvers automatically approve tool calls
	finalizer   []func(reply *AssistantMessage) error  // finalizers run with final message to ensure it matches expected value, if finalizer returns error, it's added as user message and an additional turn is executed automatically
}

func New(name string, opts ...Option) *Agent {
	a := &Agent{
		completer:   defaultCompleter,
		name:        name,
		iterations:  120,
		parallelism: 5,
		tools:       NewStaticToolset(),
		memory:      NewStaticMemory(),
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
	c := a.clone()
	for _, opt := range opts {
		opt(&c)
	}

	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("agent %q", c.name), tracing.Kind(tracing.SpanTask))
	defer span.CloseWithError(err)

	for _, d := range c.dynamics {
		if err := d(ctx, &c); err != nil {
			return reply, fmt.Errorf("failed to load options: %w", err)
		}
	}

	var tools = c.tools.List()
	var model = c.model

	// Render starter messages with template values
	system := make([]Message, len(c.messages))
	for i, m := range c.messages {
		system[i] = render(m, c.values)
	}

	// run tool calls, if previous loop ended with unapproved tool calls
	if last, ok := LastMessageAsAssistant(c.memory); ok {
		if err := c.call(ctx, last); err != nil {
			return last, err
		}
	}

loop:
	for i := 0; i < c.iterations; i++ {
		var messages []Message
		messages = append(messages, system...)

		for _, message := range c.memory.List() {
			messages = append(messages, message)
		}

		resp, err := c.complete(ctx, CompletionRequest{
			Model:             model,
			Messages:          messages,
			Tools:             tools,
			ParallelToolCalls: c.parallelism != 1 && c.parallelism != 0,
			ToolChoice:        ToolChoiceAuto,
			Temperature:       c.temperature,
			MaxTokens:         c.maxTokens,
			TopP:              c.topP,
			TopK:              c.topK,
			UseCache:          c.useCache,
			Container:         c.container,
			Betas:             c.betas,
			Reasoning:         c.reasoning,
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

	if s, ok := a.memory.(Streamer); ok {
		req.StreamCallback = s.Stream
	}

	resp, err = a.completer.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	span.SetOutput(resp.Content)
	span.SetMetric("tokens", float64(resp.Usage.TotalTokens))
	span.SetMetric("prompt_tokens", float64(resp.Usage.PromptTokens))
	span.SetMetric("thinking_tokens", float64(resp.Usage.ThinkingTokens))
	span.SetMetric("completion_tokens", float64(resp.Usage.CompletionTokens))
	span.SetMetric("prompt_cached_tokens", float64(resp.Usage.CachedPromptTokens))

	return resp, nil
}

func (a Agent) call(ctx context.Context, reply AssistantMessage) error {
	var undecided []ToolCall
	approved := map[string]bool{}

	// verify approvals for tool calls
	for _, block := range reply.Content {
		if block.Type != MessageBlockTypeToolCall {
			continue
		}

		switch a.approve(*block.ToolCall) {
		case ToolCallUndecided:
			undecided = append(undecided, *block.ToolCall)
		case ToolCallApproved:
			approved[block.ToolCall.ID] = true
		default:
			continue
		}
	}

	if len(undecided) > 0 {
		return ToolApprovalRequest{Calls: undecided}
	}

	// execute all tool calls
	results := make([]Message, len(reply.Content))

	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(a.parallelism)

	for index, block := range reply.Content {
		if block.Type != MessageBlockTypeToolCall {
			continue
		}

		index, call := index, *block.ToolCall
		eg.Go(func() (err error) {
			args := call.Arguments
			if args == "" || args == "null" {
				args = "{}"
			}

			span, gctx := tracing.StartSpan(gctx, fmt.Sprintf("tool_call %q", call.Name), tracing.Kind(tracing.SpanTool), tracing.Input(args))
			defer span.Close()

			if s, ok := a.memory.(Streamer); ok {
				_ = s.Stream(ctx, Chunk{Type: StreamChunkTypeToolCallExecute, Index: index, Call: &ToolCall{ID: call.ID, Name: call.Name}})
				defer func() {
					_ = s.Stream(ctx, Chunk{Type: StreamChunkTypeToolCallComplete, Index: index, Call: &ToolCall{ID: call.ID, Name: call.Name}})
				}()
			}

			var result any

			if approved[call.ID] {
				result, err = a.tools.Call(gctx, call.Name, []byte(args))
			} else {
				err = errors.New("tool call has been rejected by the user")
			}

			if err != nil {
				span.SetError(err)
				if errors.As(err, &Handoff{}) {
					return err
				}

				results[index] = NewToolError(call.ID, err.Error())
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
		if result == nil {
			continue
		}

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

// clone creates a deep copy of the agent to avoid shared state between concurrent calls
func (a Agent) clone() Agent {
	c := Agent{
		completer:   a.completer,
		name:        a.name,
		description: a.description,
		tools:       a.tools,
		memory:      a.memory,
		model:       a.model,
		iterations:  a.iterations,
		parallelism: a.parallelism,
	}

	if a.messages != nil {
		c.messages = make([]Message, len(a.messages))
		copy(c.messages, a.messages)
	}

	if a.models != nil {
		c.models = make(map[string]string, len(a.models))
		for k, v := range a.models {
			c.models[k] = v
		}
	}

	if a.betas != nil {
		c.betas = make([]string, len(a.betas))
		copy(c.betas, a.betas)
	}

	if a.container != nil {
		c.container = &Container{
			ID: a.container.ID,
		}
		if a.container.Skills != nil {
			c.container.Skills = make([]Skill, len(a.container.Skills))
			copy(c.container.Skills, a.container.Skills)
		}
	}

	if a.reasoning != nil {
		c.reasoning = &Reasoning{
			Enabled: a.reasoning.Enabled,
			Budget:  a.reasoning.Budget,
			Effort:  a.reasoning.Effort,
		}
	}

	if a.dynamics != nil {
		c.dynamics = make([]OptionLoader, len(a.dynamics))
		copy(c.dynamics, a.dynamics)
	}

	if a.approver != nil {
		c.approver = make([]func(call ToolCall) ToolCallApproval, len(a.approver))
		copy(c.approver, a.approver)
	}

	if a.finalizer != nil {
		c.finalizer = make([]func(reply *AssistantMessage) error, len(a.finalizer))
		copy(c.finalizer, a.finalizer)
	}

	return c
}
