package agent

import (
	"context"

	"github.com/eolymp/go-agent/prompting"
)

type Prompt struct {
	Name     string
	Version  string
	Model    string
	Messages []Message
}

type PromptLoader interface {
	Load(ctx context.Context) (*Prompt, error)
}

type PromptLoaderFunc func(ctx context.Context) (*Prompt, error)

func (f PromptLoaderFunc) Load(ctx context.Context) (*Prompt, error) {
	return f(ctx)
}

func SystemPrompt(p string) PromptLoaderFunc {
	return func(ctx context.Context) (*Prompt, error) {
		return &Prompt{
			Name:     "static",
			Version:  "0.1.0",
			Messages: []Message{SystemMessage{Content: p}},
		}, nil
	}
}

func RemotePrompt(slug string) PromptLoaderFunc {
	return func(ctx context.Context) (*Prompt, error) {
		prompt, err := prompting.Load(ctx, slug)
		if err != nil {
			return nil, err
		}

		var messages []Message
		for _, prompt := range prompt.Messages {
			switch prompt.Role {
			case prompting.RoleSystem:
				messages = append(messages, SystemMessage{Content: prompt.Content})
			case prompting.RoleUser:
				messages = append(messages, UserMessage{Content: prompt.Content})
			case prompting.RoleAssistant:
				messages = append(messages, AssistantMessage{Content: prompt.Content})
			}
		}

		return &Prompt{
			Name:     prompt.Name,
			Version:  prompt.Version,
			Model:    prompt.Model,
			Messages: messages,
		}, nil
	}
}
