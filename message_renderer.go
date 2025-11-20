package agent

import (
	"time"

	"github.com/hoisie/mustache"
)

func renderMessage(name string, m Message, values map[string]any) Message {
	if values == nil {
		values = map[string]any{}
	}

	values["name"] = name
	values["date"] = time.Now().Format(time.DateOnly)
	values["time"] = time.Now().Format(time.TimeOnly)
	values["datetime"] = time.Now().Format(time.RFC3339)

	switch x := m.(type) {
	case SystemMessage:
		c := x
		c.Content = mustache.Render(x.Content, values)
		c.Name = name
		return c
	case AssistantMessage:
		c := x
		c.Content = mustache.Render(x.Content, values)
		c.Name = name
		return c
	case UserMessage:
		c := x
		c.Content = mustache.Render(x.Content, values)
		c.Name = name
		return c
	default:
		return m
	}
}
