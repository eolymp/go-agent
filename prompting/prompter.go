package prompting

import (
	"context"
	"errors"
	"time"

	"github.com/braintrustdata/braintrust-go"
	"github.com/braintrustdata/braintrust-go/packages/param"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Prompter struct {
	cli     braintrust.Client
	project string
}

type Prompt struct {
	Name     string
	Version  string
	Model    string
	Messages []Message
}

type Message struct {
	Role    Role
	Content string
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

	return &Prompt{
		Name:     prompt.Name,
		Version:  prompt.Created.Format(time.RFC3339),
		Model:    prompt.PromptData.Options.Model,
		Messages: messages,
	}, nil
}
