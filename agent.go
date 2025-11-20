package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eolymp/go-agent/tracing"
	"github.com/eolymp/go-packages/env"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"golang.org/x/sync/errgroup"
)

type Agent struct {
	cli         openai.Client
	name        string
	description string
	tools       Toolset
	memory      Memory
	prompt      PromptLoader
	values      map[string]any
	model       string
	iterations  int
	structured  bool
}

func New(name string, prompt PromptLoader, opts ...AgentOption) *Agent {
	a := &Agent{
		cli:        openai.NewClient(option.WithAPIKey(env.String("OPENAI_API_KEY"))),
		name:       name,
		prompt:     prompt,
		iterations: 120,
		tools:      NewStaticToolset(),
		memory:     NewStaticMemory(),
		model:      openai.ChatModelGPT4_1,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

func (a Agent) Ask(ctx context.Context, opts ...AgentOption) (err error) {
	c := a
	for _, opt := range opts {
		opt(&c)
	}

	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("agent %q", c.name), tracing.Kind(tracing.SpanTask))
	defer span.CloseWithError(err)

	var tools = c.toolList()
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

	for i := 0; i < c.iterations; i++ {
		var messages []openai.ChatCompletionMessageParamUnion

		if prompt != nil {
			for _, p := range prompt.Messages {
				messages = append(messages, renderMessage(c.name, p, c.values).toOpenAIMessage())
			}
		}

		for _, message := range c.memory.List() {
			messages = append(messages, message.toOpenAIMessage())
		}

		req := openai.ChatCompletionNewParams{
			Model:    model,
			Messages: messages,
		}

		if len(tools) > 0 {
			req.Tools = tools
			req.ParallelToolCalls = openai.Bool(true)
			req.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt(string(openai.AssistantToolChoiceOptionAutoAuto)),
			}
		}

		choice, err := c.complete(ctx, req)
		if err != nil {
			return err
		}

		switch choice.FinishReason {
		case "tool_calls":
			if c.isNotEmptyResponse(choice.Message.Content) {
				c.memory.Append(AssistantMessage{Name: c.name, Content: choice.Message.Content})
			}

			if err := c.callTools(ctx, choice.Message.ToolCalls); err != nil {
				var ho Handoff
				if errors.As(err, &ho) {
					return ho.Agent.Ask(ctx, WithMemory(c.memory))
				}

				return err
			}

			continue
		default:
			if c.structured {
				content := strings.TrimPrefix(strings.Trim(choice.Message.Content, "`"), "json")

				c.memory.Append(AssistantMessage{Name: c.name, Content: content})

				if !json.Valid([]byte(content)) {
					c.memory.Append(UserMessage{Content: "You must return a valid JSON response"})
					continue
				}
			}

			if c.isNotEmptyResponse(choice.Message.Content) {
				c.memory.Append(AssistantMessage{Name: c.name, Content: choice.Message.Content})
			}
		}

		break
	}

	return nil
}

func (a Agent) complete(ctx context.Context, req openai.ChatCompletionNewParams) (choice openai.ChatCompletionChoice, err error) {
	span, ctx := tracing.StartSpan(ctx, "chat_completion", tracing.Kind(tracing.SpanLLM), tracing.Input(req.Messages), tracing.Attr("model", req.Model))
	defer span.CloseWithError(err)

	resp, err := a.cli.Chat.Completions.New(ctx, req)
	if err != nil {
		return openai.ChatCompletionChoice{}, err
	}

	if len(resp.Choices) == 0 {
		return openai.ChatCompletionChoice{}, errors.New("no response")
	}

	choice = resp.Choices[0]

	span.SetOutput(resp.Choices[0].Message)
	span.SetMetric("tokens", float64(resp.Usage.TotalTokens))
	span.SetMetric("prompt_tokens", float64(resp.Usage.PromptTokens))
	span.SetMetric("completion_tokens", float64(resp.Usage.CompletionTokens))
	span.SetMetric("prompt_cached_tokens", float64(resp.Usage.PromptTokensDetails.CachedTokens))

	return choice, nil
}

func (a Agent) callTools(ctx context.Context, calls []openai.ChatCompletionMessageToolCall) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(5)

	results := make([]Message, len(calls))

	for index, call := range calls {
		index, call := index, call

		eg.Go(func() (err error) {
			span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("tool_call %q", call.Function.Name), tracing.Kind(tracing.SpanTool), tracing.Input(json.RawMessage(call.Function.Arguments)))
			defer span.Close()

			res, err := a.tools.Call(ctx, call.Function.Name, []byte(call.Function.Arguments))
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
		msg.Calls = append(msg.Calls, &ToolCall{CallID: call.ID, Name: call.Function.Name, Arguments: []byte(call.Function.Arguments)})
	}

	a.memory.Append(msg)

	for _, result := range results {
		a.memory.Append(result)
	}

	return nil
}

func (a Agent) toolList() []openai.ChatCompletionToolParam {
	var tools []openai.ChatCompletionToolParam

	for _, tool := range a.tools.List() {
		function := openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
		}

		if tool.InputSchema != nil && tool.InputSchema.Type != "" {
			if tool.InputSchema.Type != "object" {
				panic(fmt.Errorf("tool %q input schema must be object", tool.Name))
			}

			//function.Strict = openai.Bool(true)
			function.Parameters = openai.FunctionParameters{
				"type":                 "object",
				"properties":           tool.InputSchema.Properties,
				"required":             tool.InputSchema.Required,
				"additionalProperties": false,
			}
		}

		tools = append(tools, openai.ChatCompletionToolParam{Function: function})
	}

	return tools
}

func (a Agent) isNotEmptyResponse(reply string) bool {
	reply = strings.TrimSpace(strings.ToUpper(reply))
	return reply != "NO RESPONSE" && reply != ""
}

// Memory provides access to agent's memory
// todo: not sure should be exposed
func (a Agent) Memory() Memory {
	return a.memory
}

func (a Agent) Name() string {
	return a.name
}
