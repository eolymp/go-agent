package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

func WithSpecialistTool(agents ...*Agent) Option {
	type SpecialistRequest struct {
		Specialist string `json:"specialist"`
		Task       string `json:"task"`
		Context    string `json:"context"`
	}

	type SpecialistDesc struct {
		Specialist  string `json:"specialist"`
		Description string `json:"description"`
	}

	names := make([]any, len(agents))
	for i, agent := range agents {
		names[i] = agent.name
	}

	list := Tool{
		Name:        "list_specialists",
		Description: "List available specialists",
	}

	delegate := Tool{
		Name:        "ask_specialist",
		Description: "Handoff conversation to a specialist who can provide expert-level assistance for the user",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Required:             []string{"specialist", "task", "context"},
			Properties: map[string]*jsonschema.Schema{
				"specialist": {
					Type:        "string",
					Description: "name of the specialist in charge of handling the conversation",
				},
				"task": {
					Type:        "string",
					Description: "detailed explanation of the task for the specialist, what she has to do",
				},
				"context": {
					Type:        "string",
					Description: "an extended summarization of the conversation so far",
				},
			},
		},
	}

	return func(agent *Agent) {
		opts := []Option{
			WithTool(list, func(ctx context.Context, in []byte) (any, error) {
				var items []SpecialistDesc
				for _, a := range agents {
					items = append(items, SpecialistDesc{Specialist: a.name, Description: a.description})
				}

				return items, nil
			}),
			WithTool(delegate, func(ctx context.Context, in []byte) (any, error) {
				req := SpecialistRequest{}
				if err := json.Unmarshal(in, &req); err != nil {
					return nil, fmt.Errorf("failed to unmarshal handoff request: %w", err)
				}

				for _, a := range agents {
					if a.name != req.Specialist {
						continue
					}

					m := NewStaticMemory()

					if err := m.Append(ctx, NewAssistantMessage("The summary of the conversation so far:\n"+req.Context)); err != nil {
						return nil, err
					}

					if err := m.Append(ctx, NewUserMessage(req.Task)); err != nil {
						return nil, err
					}

					if _, err := a.Run(ctx, WithMemory(m)); err != nil {
						return nil, fmt.Errorf("failed to ask specialist %q: %w", req.Specialist, err)
					}

					reply := ""
					messages := m.List()
					for i := len(messages) - 1; i >= 0; i-- {
						if m, ok := messages[i].(AssistantMessage); ok {
							reply += m.Text() + "\n"
						}
					}

					return reply, nil
				}

				return nil, fmt.Errorf("specialist %q does not exist, valid values: %v", req.Specialist, names)
			}),
		}

		for _, opt := range opts {
			opt(agent)
		}
	}
}
