package agent

import (
	"sync"
)

// Memory provides a memorization capability for an agent.
type Memory interface {
	Last() Message
	List() []Message
	Append(m Message)
}

// ForgetfulMemory keeps memory for the last user message, every new user message erases all memories.
type ForgetfulMemory struct {
	lock     sync.Mutex
	messages []Message
}

func NewForgetfulMemory() *ForgetfulMemory {
	return &ForgetfulMemory{}
}

func (m *ForgetfulMemory) Append(msg Message) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// reset previous memories once new user message is added
	if _, ok := msg.(UserMessage); ok {
		m.messages = nil
	}

	m.messages = append(m.messages, msg)
}

func (m *ForgetfulMemory) List() []Message {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.messages
}

func (m *ForgetfulMemory) Last() Message {
	m.lock.Lock()
	defer m.lock.Unlock()

	if len(m.messages) == 0 {
		return nil
	}

	return m.messages[len(m.messages)-1]
}
