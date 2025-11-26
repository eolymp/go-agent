package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/jsonschema-go/jsonschema"
)

func WithOrchestratorTool(agents ...*Agent) Option {
	names := map[string]*Agent{}
	var desc []string

	for idx := range agents {
		names[agents[idx].name] = agents[idx]
		desc = append(desc, fmt.Sprintf("  - `%s`: %s", agents[idx].name, agents[idx].description))
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
				idx := idx

				agent, ok := names[task.Agent]
				if !ok {
					continue
				}

				m := NewStaticMemory()
				m.Append(&AssistantMessage{Name: "supervisor", Content: req.Context})
				m.Append(&UserMessage{Content: "You have to perform the task described below and call `complete_task` to communicate the results. \n\nThe task: " + task.Task})

				complete := func(s, r string) {
					todo[idx].Status = s
					todo[idx].Outcome = r
				}

				if err := agent.Ask(ctx, WithMemory(m), withCompletionTool(complete)); err != nil {
					todo[idx].Status = "FAILED"
					todo[idx].Outcome = "ERROR: " + err.Error()
					break
				}

				if todo[idx].Status == "" {
					todo[idx].Status = "FAILED"
					todo[idx].Outcome = "ERROR: agent did not respond"
					break
				}

				if todo[idx].Status == "FAILED" {
					break
				}
			}

			return &OrchestrationResponse{Tasks: todo}, nil
		}),
	)
}

func withCompletionTool(f func(status string, reasoning string)) Option {
	type CompletionRequest struct {
		Status    string `json:"status"`
		Reasoning string `json:"reasoning"`
	}

	completer := Tool{
		Name:        "complete_task",
		Description: "Mark task as completed or to report an failure",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Required:             []string{"status", "reasoning"},
			Properties: map[string]*jsonschema.Schema{
				"status": {
					Type:        "string",
					Description: "COMPLETE if task is successfully complete; FAILURE if task is incomplete",
				},
				"reasoning": {
					Type:        "string",
					Description: "the summary of what actions have been taken and their outcome",
				},
			},
		},
	}

	acked := atomic.Bool{}

	return WithOptions(
		WithTool(completer, func(ctx context.Context, in []byte) (any, error) {
			req := CompletionRequest{}
			if err := json.Unmarshal(in, &req); err != nil {
				return nil, fmt.Errorf("failed to unmarshal todo list: %w", err)
			}

			if req.Status != "COMPLETE" && req.Status != "FAILURE" {
				return nil, fmt.Errorf("invalid status: %s, must be COMPLETE or FAILURE", req.Status)
			}

			if req.Reasoning == "" {
				return nil, errors.New("reasoning is required")
			}

			f(req.Status, req.Reasoning)

			acked.Store(true)

			return "Acknowledged", nil
		}),
		WithFinalizer(func(*AssistantMessage) error {
			if !acked.Load() {
				return errors.New("you must call `complete_task` tool to report task completion status")
			}

			return nil
		}),
	)
}
