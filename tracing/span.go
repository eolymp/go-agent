package tracing

import (
	"encoding/json"
	"time"
)

type Span struct {
	tracer   *Tracer
	id       string
	root     string // root span id
	parent   string // parent span id
	name     string
	input    any
	output   any
	expected any
	kind     SpanType
	start    time.Time
	end      time.Time
	tags     []string
	metrics  map[string]float64
	metadata map[string]any
	context  map[string]any
	error    error
}

func (s *Span) SetMetadata(key string, value any) {
	if s.metadata == nil {
		s.metadata = map[string]any{}
	}

	s.metadata[key] = value
}

func (s *Span) SetContext(key string, value any) {
	if s.context == nil {
		s.context = map[string]any{}
	}

	s.context[key] = value
}

func (s *Span) SetMetric(key string, value float64) {
	if s.metrics == nil {
		s.metrics = make(map[string]float64)
	}

	s.metrics[key] = value
}

func (s *Span) SetOutput(output any) {
	switch val := output.(type) {
	case string:
		if json.Valid([]byte(val)) {
			s.output = json.RawMessage(val)
			return
		}
	case []byte:
		if json.Valid(val) {
			s.output = json.RawMessage(val)
			return
		}
	}

	s.output = output
}

func (s *Span) SetExpected(expected any) {
	switch val := expected.(type) {
	case string:
		if json.Valid([]byte(val)) {
			s.expected = json.RawMessage(val)
			return
		}
	case []byte:
		if json.Valid(val) {
			s.expected = json.RawMessage(val)
			return
		}
	}

	s.expected = expected
}

func (s *Span) SetError(err error) {
	s.error = err
}

func (s *Span) SetTag(tag ...string) {
	s.tags = append(s.tags, tag...)
}

func (s *Span) Close() {
	s.end = time.Now()
	s.tracer.record(*s)
}

func (s *Span) CloseWithError(err error) {
	s.SetError(err)
	s.Close()
}

func (s *Span) CloseWithOutput(output any) {
	s.SetOutput(output)
	s.Close()
}

type SpanType string

const (
	SpanLLM      SpanType = "llm"
	SpanScore    SpanType = "score"
	SpanFunction SpanType = "function"
	SpanEval     SpanType = "eval"
	SpanTask     SpanType = "task"
	SpanTool     SpanType = "tool"
)

type SpanOption func(*Span)

func Attr(key string, value any) SpanOption {
	return func(s *Span) {
		s.SetMetadata(key, value)
	}
}

func Metric(key string, value float64) SpanOption {
	return func(s *Span) {
		s.SetMetric(key, value)
	}
}

func Context(key string, value any) SpanOption {
	return func(s *Span) {
		s.SetContext(key, value)
	}
}

func Tag(tag ...string) SpanOption {
	return func(s *Span) {
		s.SetTag(tag...)
	}
}

func Kind(t SpanType) SpanOption {
	return func(s *Span) {
		s.kind = t
	}
}

func Input(input any) SpanOption {
	return func(s *Span) {
		switch val := input.(type) {
		case string:
			if json.Valid([]byte(val)) {
				s.input = json.RawMessage(val)
				return
			}
		case []byte:
			if json.Valid(val) {
				s.input = json.RawMessage(val)
				return
			}
		}

		s.input = input
	}
}
