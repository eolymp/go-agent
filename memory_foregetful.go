package agent

import (
	"context"
	"sync"
)

// ForgetfulMemory keeps memory for the last user message, every new user message erases all memories.
type ForgetfulMemory struct {
	lock     sync.Mutex
	messages []Message
}

func NewForgetfulMemory() *ForgetfulMemory {
	return &ForgetfulMemory{}
}

func (m *ForgetfulMemory) Append(ctx context.Context, msg Message) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// reset previous memories once new user message is added
	if _, ok := msg.(UserMessage); ok {
		m.messages = nil
	}

	m.messages = append(m.messages, msg)
	return nil
}

func (m *ForgetfulMemory) List(ctx context.Context) ([]Message, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.messages, nil
}
