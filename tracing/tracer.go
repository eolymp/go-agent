package tracing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/braintrustdata/braintrust-go"
	"github.com/braintrustdata/braintrust-go/packages/param"
	"github.com/braintrustdata/braintrust-go/shared"
	"github.com/eolymp/go-packages/logger"
	"github.com/google/uuid"
)

const SpanBufferSize = 1000

type Tracer struct {
	cli     braintrust.Client
	project string
	opts    []SpanOption
	wg      sync.WaitGroup
	stream  chan Span
}

func NewTracer(cli braintrust.Client, project string, opts ...SpanOption) *Tracer {
	t := &Tracer{cli: cli, project: project, opts: opts, stream: make(chan Span, SpanBufferSize)}
	t.run()

	return t
}

func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (Span, context.Context) {
	sid := uuid.New().String()

	span := Span{
		tracer: t,
		id:     sid,
		root:   sid,
		name:   name,
		start:  time.Now(),
	}

	if root, ok := RootFromContext(ctx); ok {
		span.root = root.id
	}

	if parent, ok := SpanFromContext(ctx); ok {
		span.parent = parent.id
	}

	for _, opt := range append(t.opts, opts...) {
		opt(&span)
	}

	if span.root == span.id {
		ctx = context.WithValue(ctx, contextRoot, span)
	}

	return span, context.WithValue(ctx, contextSpan, span)
}

func (t *Tracer) run() {
	// do not do anything if project is not configured
	if t.project == "" {
		return
	}

	// start a sending routine
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		ticket := time.NewTicker(15 * time.Second)
		defer ticket.Stop()

		var batch []Span

		defer func() {
			_ = t.send(batch)
		}()

		for {
			select {
			case <-ticket.C:
				if err := t.send(batch); err != nil {
					logger.Warningf("Unable to upload tracing span buffer: %v", err)

					if strings.Contains(err.Error(), "400 Bad Request") {
						batch = nil
					}

					// truncate events to avoid overflowing
					if len(batch) > SpanBufferSize {
						batch = batch[len(batch)-SpanBufferSize:]
					}

				} else {
					batch = nil
				}

			case span, ok := <-t.stream:
				if !ok {
					return
				}

				batch = append(batch, span)
			}
		}
	}()
}

func (t *Tracer) send(spans []Span) error {
	if len(spans) == 0 {
		return nil
	}

	req := braintrust.ProjectLogInsertParams{}
	for _, span := range spans {
		event := shared.InsertProjectLogsEventParam{
			ID:         param.NewOpt(span.id),
			Created:    param.NewOpt(span.start),
			RootSpanID: param.NewOpt(span.root),
			SpanID:     param.NewOpt(span.id),
			Context: shared.InsertProjectLogsEventContextParam{
				ExtraFields: span.context,
			},
			Metadata: shared.InsertProjectLogsEventMetadataParam{
				ExtraFields: span.metadata,
			},
			Metrics: shared.InsertProjectLogsEventMetricsParam{
				Start: param.NewOpt(float64(span.start.UnixMilli()) / 1000.0),
				End:   param.NewOpt(float64(span.end.UnixMilli()) / 1000.0),
			},
			SpanAttributes: shared.SpanAttributesParam{
				Name: param.NewOpt(span.name),
				Type: braintrust.SpanType(span.kind),
			},
			Tags:     span.tags,
			Expected: span.expected,
			Input:    span.input,
			Output:   span.output,
		}

		if span.parent != "" {
			event.SpanParents = append(event.SpanParents, span.parent)
		}

		if span.error != nil {
			event.Error = span.error.Error()
		}

		if m := span.metrics; m != nil {
			if v, ok := m["completion_tokens"]; ok {
				event.Metrics.CompletionTokens = param.NewOpt(int64(v))
				delete(m, "completion_tokens")
			}

			if v, ok := m["prompt_tokens"]; ok {
				event.Metrics.PromptTokens = param.NewOpt(int64(v))
				delete(m, "prompt_tokens")
			}

			if v, ok := m["tokens"]; ok {
				event.Metrics.Tokens = param.NewOpt(int64(v))
				delete(m, "tokens")
			}

			event.Metrics.ExtraFields = m
		}

		if m := span.metadata; m != nil {
			if v, ok := m["model"]; ok {
				event.Metadata.Model = param.NewOpt(fmt.Sprint(v))
				delete(m, "model")
			}

			event.Metadata.ExtraFields = m
		}

		req.Events = append(req.Events, event)
	}

	_, err := t.cli.Projects.Logs.Insert(context.Background(), t.project, req)
	return err
}

func (t *Tracer) record(span Span) {
	// do not do anything if project is not configured
	if t.project == "" {
		return
	}

	// try to record, but if buffer is overflowing, just discard it
	select {
	case t.stream <- span:
	default:
	}
}

func (t *Tracer) Close() {
	close(t.stream)
	t.wg.Wait()
}
