package tracing

import (
	"context"
)

var tracer *Tracer

func DefaultTracer() *Tracer {
	return tracer
}

func SetDefaultTracer(t *Tracer) {
	tracer = t
}

func StartSpan(ctx context.Context, name string, opts ...SpanOption) (Span, context.Context) {
	if tracer == nil {
		return Span{}, ctx
	}

	return DefaultTracer().StartSpan(ctx, name, opts...)
}
