package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, "maia", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.ServiceVersion)
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "otlp-http", cfg.ExporterType)
	assert.Equal(t, "localhost:4318", cfg.Endpoint)
	assert.True(t, cfg.Insecure)
	assert.Equal(t, 1.0, cfg.SampleRate)
}

func TestInit_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Enabled = false

	tp, err := Init(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, tp)

	// Should be able to shutdown without error
	err = tp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestInit_NoopExporter(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ExporterType = "noop"

	tp, err := Init(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, tp)

	// Should be able to shutdown without error
	err = tp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestInit_InvalidExporter(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ExporterType = "invalid"

	_, err := Init(ctx, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported exporter type")
}

func TestInit_SamplerConfigurations(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
	}{
		{"always sample", 1.0},
		{"never sample", 0.0},
		{"ratio based", 0.5},
		{"above 1.0", 1.5},
		{"below 0.0", -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := DefaultConfig()
			cfg.Enabled = true
			cfg.ExporterType = "noop"
			cfg.SampleRate = tt.sampleRate

			tp, err := Init(ctx, cfg)
			require.NoError(t, err)
			require.NotNil(t, tp)

			err = tp.Shutdown(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestTracer(t *testing.T) {
	tracer := Tracer("test-tracer")
	assert.NotNil(t, tracer)
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()

	// Initialize with noop exporter for testing
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ExporterType = "noop"
	tp, err := Init(ctx, cfg)
	require.NoError(t, err)
	defer func() {
		_ = tp.Shutdown(ctx)
	}()

	spanCtx, span := StartSpan(ctx, "test-operation")
	defer span.End()

	assert.NotNil(t, span)
	assert.True(t, span.SpanContext().IsValid())
	assert.NotNil(t, spanCtx) // Use the context
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()

	// Without span
	span := SpanFromContext(ctx)
	assert.NotNil(t, span)

	// With span
	ctx, newSpan := StartSpan(ctx, "test-operation")
	defer newSpan.End()

	span = SpanFromContext(ctx)
	assert.Equal(t, newSpan.SpanContext(), span.SpanContext())
}

func TestSetSpanAttributes(t *testing.T) {
	ctx := context.Background()

	// Initialize with noop exporter for testing
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ExporterType = "noop"
	tp, err := Init(ctx, cfg)
	require.NoError(t, err)
	defer func() {
		_ = tp.Shutdown(ctx)
	}()

	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()

	// Should not panic
	SetSpanAttributes(ctx,
		AttrMemoryID.String("test-id"),
		AttrMemoryNamespace.String("test-ns"),
	)
}

func TestRecordError(t *testing.T) {
	ctx := context.Background()

	// Initialize with noop exporter for testing
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ExporterType = "noop"
	tp, err := Init(ctx, cfg)
	require.NoError(t, err)
	defer func() {
		_ = tp.Shutdown(ctx)
	}()

	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()

	// Should not panic with nil error
	RecordError(ctx, nil)

	// Should not panic with error
	RecordError(ctx, assert.AnError)
}

func TestAttributeKeys(t *testing.T) {
	// Verify attribute keys are properly defined
	assert.Equal(t, attribute.Key("maia.memory.id"), AttrMemoryID)
	assert.Equal(t, attribute.Key("maia.memory.namespace"), AttrMemoryNamespace)
	assert.Equal(t, attribute.Key("maia.memory.type"), AttrMemoryType)
	assert.Equal(t, attribute.Key("maia.query.text"), AttrQueryText)
	assert.Equal(t, attribute.Key("maia.query.intent"), AttrQueryIntent)
	assert.Equal(t, attribute.Key("maia.search.limit"), AttrSearchLimit)
	assert.Equal(t, attribute.Key("maia.search.results"), AttrSearchResults)
	assert.Equal(t, attribute.Key("maia.context.tokens"), AttrContextTokens)
	assert.Equal(t, attribute.Key("maia.context.budget"), AttrContextBudget)
	assert.Equal(t, attribute.Key("maia.embedding.dimensions"), AttrEmbeddingDim)
}

func TestNoopExporter(t *testing.T) {
	ctx := context.Background()
	exp := &noopExporter{}

	err := exp.ExportSpans(ctx, nil)
	assert.NoError(t, err)

	err = exp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestTracerProvider_Shutdown_NilProvider(t *testing.T) {
	tp := &TracerProvider{provider: nil}
	err := tp.Shutdown(context.Background())
	assert.NoError(t, err)
}
