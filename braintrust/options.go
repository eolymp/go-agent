package braintrust

import (
	"context"

	"github.com/eolymp/go-agent"
)

func WithPrompt(slug string) agent.Option {
	return WithPrompter(DefaultPrompter(), slug)
}

func WithPrompter(prompter *Prompter, slug string) agent.Option {
	return agent.WithOptionLoader(func(ctx context.Context, a *agent.Agent) error {
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

		if prompt.Temperature != nil {
			opts = append(opts, agent.WithTemperature(*prompt.Temperature))
		}

		if prompt.MaxTokens != nil {
			opts = append(opts, agent.WithMaxTokens(*prompt.MaxTokens))
		}

		if prompt.TopP != nil {
			opts = append(opts, agent.WithTopP(*prompt.TopP))
		}

		if prompt.TopK != nil {
			opts = append(opts, agent.WithTopK(*prompt.TopK))
		}

		if prompt.UseCache != nil {
			opts = append(opts, agent.WithUseCache(*prompt.UseCache))
		}

		if prompt.Reasoning != nil {
			thinking := &agent.Reasoning{}
			if prompt.Reasoning.Enabled != nil {
				thinking.Enabled = *prompt.Reasoning.Enabled
			}

			if prompt.Reasoning.Budget != nil {
				thinking.Budget = int(*prompt.Reasoning.Budget)
			}

			if prompt.Reasoning.Effort != nil {
				thinking.Effort = *prompt.Reasoning.Effort
			}

			opts = append(opts, agent.WithReasoning(thinking))
		}

		if prompt.Metadata != nil {
			if len(prompt.Metadata.AutoApproveTools) > 0 {
				opts = append(opts, agent.WithAutoApproveTools(prompt.Metadata.AutoApproveTools...))
			}

			if len(prompt.Metadata.Betas) > 0 {
				opts = append(opts, agent.WithBetas(prompt.Metadata.Betas...))
			}

			if len(prompt.Metadata.Skills) > 0 {
				skills := make([]agent.Skill, len(prompt.Metadata.Skills))
				for i, skill := range prompt.Metadata.Skills {
					skills[i] = agent.Skill{
						SkillID: skill.ID,
						Type:    skill.Type,
						Version: skill.Version,
					}
				}

				opts = append(opts, agent.WithContainer(&agent.Container{
					Skills: skills,
				}))
			}

			if len(prompt.Metadata.Tools) > 0 {
				for _, tool := range prompt.Metadata.Tools {
					opts = append(opts, agent.WithBuiltinTool(tool.Name, tool.Type))
				}
			}
		}

		for _, opt := range opts {
			opt(a)
		}

		return nil
	})
}
