package agent

import (
	"time"

	"github.com/hoisie/mustache"
)

type Message interface {
	isMessage()
}

func render(m Message, values map[string]any) Message {
	if values == nil {
		values = map[string]any{}
	}

	values["date"] = time.Now().Format(time.DateOnly)
	values["time"] = time.Now().Format(time.TimeOnly)
	values["datetime"] = time.Now().Format(time.RFC3339)

	switch v := m.(type) {
	case AssistantMessage:
		content := make([]MessageBlock, len(v.Content))
		for i, block := range v.Content {
			content[i] = block
			if block.Text != "" {
				content[i].Text = mustache.Render(block.Text, values)
			}
		}

		return AssistantMessage{Content: content}
	case SystemMessage:
		return SystemMessage{Content: mustache.Render(v.Content, values)}
	case UserMessage:
		return UserMessage{Content: mustache.Render(v.Content, values)}
	default:
		return m
	}
}
