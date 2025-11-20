package tracing

import (
	"context"
)

type contextKey int

const (
	contextSpan contextKey = iota
	contextRoot
)

func SpanFromContext(ctx context.Context) (Span, bool) {
	s, ok := ctx.Value(contextSpan).(Span)
	return s, ok
}

func RootFromContext(ctx context.Context) (Span, bool) {
	s, ok := ctx.Value(contextRoot).(Span)
	return s, ok
}
