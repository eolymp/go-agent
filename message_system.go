package agent

type SystemMessage struct {
	Content string `json:"content"`
}

func NewSystemMessage(text string) SystemMessage {
	return SystemMessage{Content: text}
}

func (m SystemMessage) isMessage() {}

func (m SystemMessage) render(values map[string]any) Message {
	return SystemMessage{
		Content: MessageRender(m.Content, values),
	}
}
