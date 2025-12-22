package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

type Handoff struct {
	Agent *Agent
}

func (Handoff) Error() string {
	return "handed over"
}

func WithHandoffTool(agents ...*Agent) Option {
	type HandoffRequest struct {
		Specialist string `json:"specialist"`
		Message    string `json:"message"`
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
		Name:        "delegate_to",
		Description: "Handoff conversation to a specialist who can provide expert-level assistance for the user",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Required:             []string{"specialist"},
			Properties: map[string]*jsonschema.Schema{
				"specialist": {
					Type:        "string",
					Description: "name of the specialist in charge of handling the conversation",
				},
				"message": {
					Type:        "string",
					Description: "a description of the task for specialist",
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
				req := HandoffRequest{}
				if err := json.Unmarshal(in, &req); err != nil {
					return nil, fmt.Errorf("failed to unmarshal handoff request: %w", err)
				}

				for _, a := range agents {
					if a.name != req.Specialist {
						continue
					}

					if req.Message != "" {
						agent.memory.Append(NewAssistantMessage(req.Message))
					}

					return "", Handoff{Agent: a}
				}

				return nil, fmt.Errorf("specialist %q does not exist, valid values: %v", req.Specialist, names)
			}),
		}

		for _, opt := range opts {
			opt(agent)
		}
	}
}
