package agent

import (
	"time"

	"github.com/hoisie/mustache"
)

var MessageRender = func(t string, v map[string]any) string {
	if v == nil {
		v = map[string]any{}
	}

	v["date"] = time.Now().Format(time.DateOnly)
	v["time"] = time.Now().Format(time.TimeOnly)
	v["datetime"] = time.Now().Format(time.RFC3339)

	return mustache.Render(t, v)
}
