package agent

import (
	"context"
	"sync"
)

// StaticMemory keeps all messages in-memory.
type StaticMemory struct {
	lock     sync.Mutex
	messages []Message
}

func NewStaticMemory() *StaticMemory {
	return &StaticMemory{}
}

func (m *StaticMemory) Append(ctx context.Context, msg Message) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.messages = append(m.messages, msg)
	return nil
}

func (m *StaticMemory) List() []Message {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.messages
}
