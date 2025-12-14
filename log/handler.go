package log

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// Default keys
const (
	DefaultTraceIDKey      = "trace_id"
	DefaultSpanIDKey       = "span_id"
	DefaultTraceSampledKey = "trace_sampled"
)

// TraceHandlerConfig holds configuration for TraceHandler
type TraceHandlerConfig struct {
	TraceIDKey      string
	SpanIDKey       string
	TraceSampledKey string
}

// TraceHandler is a slog.Handler that adds trace ID and span ID to the record
type TraceHandler struct {
	slog.Handler
	config TraceHandlerConfig
}

// NewTraceHandler creates a new TraceHandler
func NewTraceHandler(h slog.Handler, config *TraceHandlerConfig) *TraceHandler {
	cfg := TraceHandlerConfig{
		TraceIDKey:      DefaultTraceIDKey,
		SpanIDKey:       DefaultSpanIDKey,
		TraceSampledKey: DefaultTraceSampledKey,
	}
	if config != nil {
		if config.TraceIDKey != "" {
			cfg.TraceIDKey = config.TraceIDKey
		}
		if config.SpanIDKey != "" {
			cfg.SpanIDKey = config.SpanIDKey
		}
		if config.TraceSampledKey != "" {
			cfg.TraceSampledKey = config.TraceSampledKey
		}
	}

	return &TraceHandler{
		Handler: h,
		config:  cfg,
	}
}

// Handle adds trace_id and span_id to the record if a span is found in the context
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Add trace_id and span_id attributes
		r.AddAttrs(
			slog.String(h.config.TraceIDKey, span.SpanContext().TraceID().String()),
			slog.String(h.config.SpanIDKey, span.SpanContext().SpanID().String()),
			slog.Bool(h.config.TraceSampledKey, span.SpanContext().TraceFlags().IsSampled()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new TraceHandler with attributes added to the underlying handler
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{
		Handler: h.Handler.WithAttrs(attrs),
		config:  h.config,
	}
}

// WithGroup returns a new TraceHandler with a group added to the underlying handler
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{
		Handler: h.Handler.WithGroup(name),
		config:  h.config,
	}
}

