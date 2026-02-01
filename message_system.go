package agent

type SystemMessage struct {
	Content string `json:"content"`
}

func NewSystemMessage(text string) SystemMessage {
	return SystemMessage{Content: text}
}

func (m SystemMessage) isMessage() {}
