package braintrust

import (
	"context"
	"os"
	"sync"

	"github.com/braintrustdata/braintrust-go"
	"github.com/eolymp/go-agent"
)

var prompter *Prompter
var initialize sync.Once

func DefaultPrompter() *Prompter {
	initialize.Do(func() {
		prompter = NewPrompter(braintrust.NewClient(), os.Getenv("BRAINTRUST_PROJECT"))
	})

	return prompter
}

func SetDefaultPrompter(t *Prompter) {
	initialize.Do(func() {})
	prompter = t
}

func Load(ctx context.Context, slug string) (*Prompt, error) {
	return DefaultPrompter().Load(ctx, slug)
}

func WithPrompt(slug string) agent.OptionLoader {
	return WithPrompter(DefaultPrompter(), slug)
}

func WithPrompter(prompter *Prompter, slug string) agent.OptionLoader {
	return func(ctx context.Context, a *agent.Agent) error {
		prompt, err := prompter.Load(ctx, slug)
		if err != nil {
			return err
		}

		var opts []agent.Option

		for _, msg := range prompt.Messages {
			switch msg.Role {
			case RoleSystem:
				opts = append(opts, agent.WithSystemMessage(msg.Content))
			case RoleUser:
				opts = append(opts, agent.WithUserMessage(msg.Content))
			case RoleAssistant:
				opts = append(opts, agent.WithAssistantMessage(msg.Content))
			}
		}

		if prompt.Model != "" {
			opts = append(opts, agent.WithModel(prompt.Model))
		}

		for _, opt := range opts {
			opt(a)
		}

		return nil
	}
}
