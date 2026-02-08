package braintrust

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/braintrustdata/braintrust-go"
	"github.com/braintrustdata/braintrust-go/packages/param"
)

type Prompter struct {
	cli     braintrust.Client
	project string
}

func NewPrompter(cli braintrust.Client, project string) *Prompter {
	return &Prompter{cli: cli, project: project}
}

func (p *Prompter) Load(ctx context.Context, slug string) (*Prompt, error) {
	prompts, err := p.cli.Prompts.List(ctx, braintrust.PromptListParams{
		Limit:     param.NewOpt[int64](1),
		ProjectID: param.NewOpt(p.project),
		Slug:      param.NewOpt(slug),
	})

	if err != nil {
		return nil, err
	}

	if len(prompts.Objects) == 0 {
		return nil, errors.New("system prompt not found")
	}

	prompt := prompts.Objects[0]

	var messages []Message
	for _, m := range prompt.PromptData.Prompt.Messages {
		if m.Content.OfString == "" {
			continue
		}

		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			continue
		}

		messages = append(messages, Message{
			Role:    Role(m.Role),
			Content: m.Content.OfString,
		})
	}

	result := &Prompt{
		Name:        prompt.Name,
		Description: prompt.Description,
		Version:     prompt.Created.Format(time.RFC3339),
		Model:       prompt.PromptData.Options.Model,
		Messages:    messages,
	}

	// Extract parameters from prompt_data.options.params
	params := prompt.PromptData.Options.Params
	if params.JSON.Temperature.Valid() {
		result.Temperature = Ref(float32(params.Temperature))
	}

	if params.JSON.MaxTokens.Valid() {
		result.MaxTokens = Ref(int64(params.MaxTokens))
	}

	if params.JSON.TopP.Valid() {
		result.TopP = Ref(float32(params.TopP))
	}

	if params.JSON.TopK.Valid() {
		result.TopK = Ref(int32(params.TopK))
	}

	if params.JSON.UseCache.Valid() {
		result.UseCache = Ref(params.UseCache)
	}

	// Extract tool choice
	if string(params.ToolChoice.OfPromptOptionssOpenAIModelParamsToolChoiceString) != "" {
		result.ToolChoice = Ref(string(params.ToolChoice.OfPromptOptionssOpenAIModelParamsToolChoiceString))
	} else if params.ToolChoice.Type != "" {
		result.ToolChoice = Ref(params.ToolChoice.Type)
		if params.ToolChoice.Function.Name != "" {
			result.ToolFunction = Ref(params.ToolChoice.Function.Name)
		}
	}

	// Extract response format
	if params.ResponseFormat.Type != "" {
		result.ResponseFormat = &Response{
			Type: params.ResponseFormat.Type,
		}

		if params.ResponseFormat.JsonSchema.Name != "" {
			result.ResponseFormat.Schema = &Schema{
				Name:        params.ResponseFormat.JsonSchema.Name,
				Description: params.ResponseFormat.JsonSchema.Description,
				Schema:      []byte(params.ResponseFormat.JsonSchema.Schema),
				Strict:      params.ResponseFormat.JsonSchema.Strict,
			}
		}
	}

	// Extract reasoning effort (OpenAI specific)
	if params.JSON.ReasoningEffort.Valid() && params.ReasoningEffort != "" {
		if result.Reasoning == nil {
			result.Reasoning = &Reasoning{}
		}
		result.Reasoning.Effort = Ref(params.ReasoningEffort)
	}

	// Extract metadata
	if len(prompt.Metadata) > 0 {
		if bytes, err := json.Marshal(prompt.Metadata); err == nil {
			var metadata Metadata
			if err := json.Unmarshal(bytes, &metadata); err == nil {
				result.Metadata = &metadata
			}
		}
	}

	return result, nil
}
