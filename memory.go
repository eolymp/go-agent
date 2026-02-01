package agent

import (
	"context"
)

// Memory provides a memorization capability for an agent.
type Memory interface {
	List() []Message
	Append(ctx context.Context, m Message) error
}

func LastMessage(memory Memory) (Message, bool) {
	messages := memory.List()
	if len(messages) == 0 {
		return nil, false
	}

	return messages[len(messages)-1], true
}

func LastMessageAsAssistant(memory Memory) (AssistantMessage, bool) {
	last, _ := LastMessage(memory)
	am, ok := last.(AssistantMessage)
	return am, ok
}

func LastMessageAsUser(memory Memory) (UserMessage, bool) {
	last, _ := LastMessage(memory)
	um, ok := last.(UserMessage)
	return um, ok
}
