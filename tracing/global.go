package tracing

import (
	"context"
	"os"
	"sync"

	"github.com/braintrustdata/braintrust-go"
)

var tracer *Tracer
var initialize sync.Once

func DefaultTracer() *Tracer {
	initialize.Do(func() {
		tracer = NewTracer(braintrust.NewClient(), os.Getenv("BRAINTRUST_PROJECT"))
	})

	return tracer
}

func SetDefaultTracer(t *Tracer) {
	initialize.Do(func() {})
	tracer = t
}

func StartSpan(ctx context.Context, name string, opts ...SpanOption) (Span, context.Context) {
	return DefaultTracer().StartSpan(ctx, name, opts...)
}
