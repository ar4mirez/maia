// Package tracing provides OpenTelemetry distributed tracing for MAIA.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration.
type Config struct {
	// Enabled controls whether tracing is active.
	Enabled bool
	// ServiceName is the name of the service for tracing.
	ServiceName string
	// ServiceVersion is the version of the service.
	ServiceVersion string
	// Environment is the deployment environment (dev, staging, prod).
	Environment string
	// ExporterType is the type of exporter: "otlp-http", "otlp-grpc", or "noop".
	ExporterType string
	// Endpoint is the OTLP collector endpoint.
	Endpoint string
	// Insecure disables TLS for the exporter connection.
	Insecure bool
	// SampleRate is the sampling rate (0.0 to 1.0).
	SampleRate float64
}

// DefaultConfig returns a default tracing configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		ServiceName:    "maia",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		ExporterType:   "otlp-http",
		Endpoint:       "localhost:4318",
		Insecure:       true,
		SampleRate:     1.0,
	}
}

// TracerProvider wraps the OpenTelemetry TracerProvider with shutdown support.
type TracerProvider struct {
	provider *sdktrace.TracerProvider
}

// Init initializes the OpenTelemetry tracing provider.
func Init(ctx context.Context, cfg Config) (*TracerProvider, error) {
	if !cfg.Enabled {
		// Return a noop provider
		return &TracerProvider{provider: nil}, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on type
	var exporter sdktrace.SpanExporter
	switch cfg.ExporterType {
	case "otlp-http":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	case "otlp-grpc":
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	case "noop":
		// Use a noop exporter for testing
		exporter = &noopExporter{}
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", cfg.ExporterType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
	)

	// Set global provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &TracerProvider{provider: provider}, nil
}

// Shutdown shuts down the tracer provider, flushing any remaining spans.
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// Tracer returns a tracer for the given name.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// SpanFromContext returns the current span from the context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// StartSpan starts a new span with the given name.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer("maia").Start(ctx, name, opts...)
}

// SetSpanAttributes sets attributes on the current span.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error) {
	span := SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
	}
}

// Common attribute keys for MAIA operations.
var (
	AttrMemoryID        = attribute.Key("maia.memory.id")
	AttrMemoryNamespace = attribute.Key("maia.memory.namespace")
	AttrMemoryType      = attribute.Key("maia.memory.type")
	AttrQueryText       = attribute.Key("maia.query.text")
	AttrQueryIntent     = attribute.Key("maia.query.intent")
	AttrSearchLimit     = attribute.Key("maia.search.limit")
	AttrSearchResults   = attribute.Key("maia.search.results")
	AttrContextTokens   = attribute.Key("maia.context.tokens")
	AttrContextBudget   = attribute.Key("maia.context.budget")
	AttrEmbeddingDim    = attribute.Key("maia.embedding.dimensions")
)

// noopExporter is a noop span exporter for testing.
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}
