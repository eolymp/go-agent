package agent

type Message interface {
	isMessage()
	render(values map[string]any) Message
}
