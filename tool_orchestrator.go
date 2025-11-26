package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

func WithOrchestratorTool(agents ...Agent) Option {
	names := map[string]Agent{}
	var desc []string

	for _, agent := range agents {
		names[agent.name] = agent
		desc = append(desc, fmt.Sprintf("  - `%s`: %s", agent.name, agent.description))
	}

	type Task struct {
		Agent   string `json:"agent"`
		Task    string `json:"task"`
		Outcome string `json:"outcome"`
		Status  string `json:"status"`
	}

	type Outcome struct {
		Reasoning string `json:"outcome"`
		Status    string `json:"status"`
	}

	type OrchestrationRequest struct {
		Context string `json:"context"`
		Tasks   []Task `json:"tasks"`
	}

	type OrchestrationResponse struct {
		Tasks []Task `json:"tasks"`
	}

	planner := Tool{
		Name:        "execute_tasks",
		Description: "Execute tasks in the todo list",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Required:             []string{"context", "tasks"},
			Properties: map[string]*jsonschema.Schema{
				"context": {
					Type:        "string",
					Description: "an extended summarization of the conversation so far, context details, requirements, identifiers, names etc",
				},
				"tasks": {
					Type:        "array",
					Description: "list of tasks to complete",
					Items: &jsonschema.Schema{
						Type:                 "object",
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
						Required:             []string{"agent", "task"},
						Properties: map[string]*jsonschema.Schema{
							"agent": {
								Type:        "string",
								Description: "name of the agent in charge of performing this task, available agents are:\n" + strings.Join(desc, "\n"),
							},
							"task": {
								Type:        "string",
								Description: "detailed explanation of the task for the agent, what she has to do and what outcome is expected",
							},
						},
					},
				},
			},
		},
	}

	return WithOptions(
		WithTool(planner, func(ctx context.Context, in []byte) (any, error) {
			req := OrchestrationRequest{}
			if err := json.Unmarshal(in, &req); err != nil {
				return nil, fmt.Errorf("failed to unmarshal todo list: %w", err)
			}

			todo := req.Tasks

			for _, task := range todo {
				if _, ok := names[task.Agent]; !ok {
					return nil, fmt.Errorf("agent %q assigned to one of the tasks does not exist", task.Agent)
				}
			}

			for idx, task := range todo {
				agent, ok := names[task.Agent]
				if !ok {
					continue
				}

				m := NewStaticMemory()
				m.Append(&AssistantMessage{Name: "supervisor", Content: req.Context})
				m.Append(&UserMessage{Content: task.Task})

				if err := agent.Ask(ctx, WithMemory(m), WithStructuredOutput()); err != nil {
					todo[idx].Status = "FAILED"
					todo[idx].Outcome = "ERROR: " + err.Error()
					break
				}

				reply, ok := m.Last().(AssistantMessage)
				if !ok {
					todo[idx].Status = "FAILED"
					todo[idx].Outcome = "ERROR: agent did not respond"
					break
				}

				outcome := Outcome{}

				if err := reply.Unmarshal(&outcome); err != nil {
					todo[idx].Status = "FAILED"
					todo[idx].Outcome = "ERROR: Agent responded in invalid format. Response: " + reply.Content
					break
				}

				todo[idx].Status = outcome.Status
				todo[idx].Outcome = outcome.Reasoning

				if outcome.Status != "COMPLETE" {
					break
				}
			}

			return &OrchestrationResponse{Tasks: todo}, nil
		}),
	)
}
