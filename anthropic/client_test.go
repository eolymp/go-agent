package anthropic_test

import (
	"context"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	agent "github.com/eolymp/go-agent"
	"github.com/eolymp/go-agent/anthropic"
	"github.com/google/jsonschema-go/jsonschema"
)

const DefaultModel = "claude-haiku-4-5-20251001"

func TestComplete_NonStreaming(t *testing.T) {
	t.Run("say-hello", func(t *testing.T) {
		ctx := context.Background()

		resp, err := newCompleter(t).
			Complete(ctx, agent.CompletionRequest{
				Model: DefaultModel,
				Messages: []agent.Message{
					agent.NewUserMessage("Say hello in one word."),
				},
			})

		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		if len(resp.Content) == 0 {
			t.Fatal("expected at least one content block in response")
		}

		block := resp.Content[0]
		if block.Type != agent.MessageBlockTypeText {
			t.Fatalf("expected text block, got %q", block.Type)
		}

		if block.Text == "" {
			t.Fatal("expected non-empty text in response")
		}

		t.Logf("Anthropic response: %s", block.Text)
	})

	t.Run("use-tool", func(t *testing.T) {
		ctx := context.Background()

		resp, err := newCompleter(t).
			Complete(ctx, agent.CompletionRequest{
				Model: DefaultModel,
				Tools: []agent.Tool{{
					Name:        "greeter",
					Description: "Greeter tool prompts user with a greeting",
					InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"name": {Type: "string"}}},
				}},
				Messages: []agent.Message{
					agent.NewUserMessage("Greet a Riccardo using greeter tool."),
				},
			})

		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		if len(resp.Content) == 0 {
			t.Fatal("expected at least one content block in response")
		}

		block := resp.Content[0]
		if block.Type != agent.MessageBlockTypeToolCall {
			t.Fatalf("expected tool call block, got %q", block.Type)
		}

		if block.ToolCall == nil {
			t.Fatal("expected non-empty tool call")
		}

		if block.ToolCall.Name != "greeter" {
			t.Fatalf("expected greeter tool call, got %q", block.ToolCall.Name)
		}

		t.Logf("Anthropic response: %s %s", block.ToolCall.Name, block.ToolCall.Arguments)
	})
}

func newCompleter(t *testing.T) *anthropic.Completer {
	t.Helper()

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}

	return anthropic.New(option.WithAPIKey(key))
}
