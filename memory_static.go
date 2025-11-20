package agent

import "sync"

// StaticMemory keeps all messages in-memory.
type StaticMemory struct {
	lock     sync.Mutex
	messages []Message
}

func NewStaticMemory() *StaticMemory {
	return &StaticMemory{}
}

func (m *StaticMemory) Append(msg Message) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.messages = append(m.messages, msg)
}

func (m *StaticMemory) List() []Message {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.messages
}

func (m *StaticMemory) Last() Message {
	m.lock.Lock()
	defer m.lock.Unlock()

	if len(m.messages) == 0 {
		return nil
	}

	return m.messages[len(m.messages)-1]
}
