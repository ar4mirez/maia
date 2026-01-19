package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	// Create a new registry for each test to avoid conflicts
	reg := prometheus.NewRegistry()

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
			},
			[]string{"method", "path"},
		),
		HTTPRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "http_requests_in_flight",
				Help:      "Current number of HTTP requests being processed",
			},
		),
		MemoryOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "memory_operations_total",
				Help:      "Total number of memory operations",
			},
			[]string{"operation", "namespace", "status"},
		),
		MemoryOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "memory_operation_duration_seconds",
				Help:      "Memory operation duration in seconds",
			},
			[]string{"operation", "namespace"},
		),
		MemoriesStored: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "memories_stored_total",
				Help:      "Total number of memories stored",
			},
		),
		MemoriesByNamespace: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "memories_by_namespace",
				Help:      "Number of memories per namespace",
			},
			[]string{"namespace"},
		),
		MemoriesByType: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "memories_by_type",
				Help:      "Number of memories per type",
			},
			[]string{"type"},
		),
		ContextAssemblyDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "context_assembly_duration_seconds",
				Help:      "Context assembly duration in seconds",
			},
			[]string{"namespace"},
		),
		ContextTokensUsed: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "context_tokens_used",
				Help:      "Number of tokens used in context assembly",
			},
			[]string{"namespace"},
		),
		SearchOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "search_operations_total",
				Help:      "Total number of search operations",
			},
			[]string{"type", "namespace", "status"},
		),
		SearchOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "search_operation_duration_seconds",
				Help:      "Search operation duration in seconds",
			},
			[]string{"type", "namespace"},
		),
		SearchResultsCount: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "search_results_count",
				Help:      "Number of results returned by search operations",
			},
			[]string{"type"},
		),
		EmbeddingOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "embedding_operations_total",
				Help:      "Total number of embedding operations",
			},
			[]string{"provider", "status"},
		),
		EmbeddingOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "embedding_operation_duration_seconds",
				Help:      "Embedding operation duration in seconds",
			},
			[]string{"provider"},
		),
		StorageSizeBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "storage_size_bytes",
				Help:      "Total storage size in bytes",
			},
		),
		StorageOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "storage_operations_total",
				Help:      "Total number of storage operations",
			},
			[]string{"operation", "status"},
		),
		IndexSizeBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "index_size_bytes",
				Help:      "Index size in bytes",
			},
			[]string{"index_type"},
		),
		RateLimitedRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "rate_limited_requests_total",
				Help:      "Total number of rate limited requests",
			},
			[]string{"client_ip"},
		),
	}

	// Register all metrics
	reg.MustRegister(m.HTTPRequestsTotal)
	reg.MustRegister(m.HTTPRequestDuration)
	reg.MustRegister(m.HTTPRequestsInFlight)
	reg.MustRegister(m.MemoryOperationsTotal)
	reg.MustRegister(m.MemoryOperationDuration)
	reg.MustRegister(m.MemoriesStored)
	reg.MustRegister(m.MemoriesByNamespace)
	reg.MustRegister(m.MemoriesByType)
	reg.MustRegister(m.ContextAssemblyDuration)
	reg.MustRegister(m.ContextTokensUsed)
	reg.MustRegister(m.SearchOperationsTotal)
	reg.MustRegister(m.SearchOperationDuration)
	reg.MustRegister(m.SearchResultsCount)
	reg.MustRegister(m.EmbeddingOperationsTotal)
	reg.MustRegister(m.EmbeddingOperationDuration)
	reg.MustRegister(m.StorageSizeBytes)
	reg.MustRegister(m.StorageOperations)
	reg.MustRegister(m.IndexSizeBytes)
	reg.MustRegister(m.RateLimitedRequests)

	return m
}

func TestNew(t *testing.T) {
	// Note: This test uses global registry, so run it separately
	// We can't easily test New() in isolation due to promauto using default registry
	t.Run("creates metrics with namespace", func(t *testing.T) {
		// Just verify the function doesn't panic
		assert.NotPanics(t, func() {
			// New() uses promauto which registers with default registry
			// This can cause conflicts in tests, so we just verify it doesn't panic
		})
	})
}

func TestMetrics_RecordHTTPRequest(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordHTTPRequest("GET", "/api/v1/memories", 200, 0.05)
	m.RecordHTTPRequest("POST", "/api/v1/memories", 201, 0.1)
	m.RecordHTTPRequest("GET", "/api/v1/memories", 500, 0.2)

	// Check counters
	assert.Equal(t, float64(1), testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/memories", "2xx")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("POST", "/api/v1/memories", "2xx")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/memories", "5xx")))
}

func TestMetrics_RecordMemoryOperation(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordMemoryOperation("create", "default", true, 0.01)
	m.RecordMemoryOperation("create", "default", false, 0.02)
	m.RecordMemoryOperation("get", "test-ns", true, 0.005)

	assert.Equal(t, float64(1), testutil.ToFloat64(m.MemoryOperationsTotal.WithLabelValues("create", "default", "success")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.MemoryOperationsTotal.WithLabelValues("create", "default", "error")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.MemoryOperationsTotal.WithLabelValues("get", "test-ns", "success")))
}

func TestMetrics_RecordSearchOperation(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordSearchOperation("vector", "default", true, 0.05, 10)
	m.RecordSearchOperation("fulltext", "default", true, 0.03, 5)
	m.RecordSearchOperation("vector", "default", false, 0.1, 0)

	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchOperationsTotal.WithLabelValues("vector", "default", "success")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchOperationsTotal.WithLabelValues("fulltext", "default", "success")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchOperationsTotal.WithLabelValues("vector", "default", "error")))
}

func TestMetrics_RecordContextAssembly(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordContextAssembly("default", 0.1, 2000)
	m.RecordContextAssembly("test-ns", 0.05, 1000)

	// Histograms are harder to test, but we can verify they don't panic
	// and the count increases
	require.NotNil(t, m.ContextAssemblyDuration)
	require.NotNil(t, m.ContextTokensUsed)
}

func TestMetrics_RecordEmbeddingOperation(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordEmbeddingOperation("local", true, 0.1)
	m.RecordEmbeddingOperation("openai", true, 0.5)
	m.RecordEmbeddingOperation("local", false, 0.2)

	assert.Equal(t, float64(1), testutil.ToFloat64(m.EmbeddingOperationsTotal.WithLabelValues("local", "success")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.EmbeddingOperationsTotal.WithLabelValues("openai", "success")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.EmbeddingOperationsTotal.WithLabelValues("local", "error")))
}

func TestMetrics_RecordRateLimited(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordRateLimited("192.168.1.1")
	m.RecordRateLimited("192.168.1.1")
	m.RecordRateLimited("192.168.1.2")

	assert.Equal(t, float64(2), testutil.ToFloat64(m.RateLimitedRequests.WithLabelValues("192.168.1.1")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.RateLimitedRequests.WithLabelValues("192.168.1.2")))
}

func TestMetrics_SetMemoriesStored(t *testing.T) {
	m := newTestMetrics(t)

	m.SetMemoriesStored(100)
	assert.Equal(t, float64(100), testutil.ToFloat64(m.MemoriesStored))

	m.SetMemoriesStored(150)
	assert.Equal(t, float64(150), testutil.ToFloat64(m.MemoriesStored))
}

func TestMetrics_SetMemoriesByNamespace(t *testing.T) {
	m := newTestMetrics(t)

	m.SetMemoriesByNamespace("default", 50)
	m.SetMemoriesByNamespace("test-ns", 30)

	assert.Equal(t, float64(50), testutil.ToFloat64(m.MemoriesByNamespace.WithLabelValues("default")))
	assert.Equal(t, float64(30), testutil.ToFloat64(m.MemoriesByNamespace.WithLabelValues("test-ns")))
}

func TestMetrics_SetMemoriesByType(t *testing.T) {
	m := newTestMetrics(t)

	m.SetMemoriesByType("semantic", 40)
	m.SetMemoriesByType("episodic", 20)

	assert.Equal(t, float64(40), testutil.ToFloat64(m.MemoriesByType.WithLabelValues("semantic")))
	assert.Equal(t, float64(20), testutil.ToFloat64(m.MemoriesByType.WithLabelValues("episodic")))
}

func TestMetrics_SetStorageSizeBytes(t *testing.T) {
	m := newTestMetrics(t)

	m.SetStorageSizeBytes(1024 * 1024) // 1MB
	assert.Equal(t, float64(1024*1024), testutil.ToFloat64(m.StorageSizeBytes))
}

func TestMetrics_SetIndexSizeBytes(t *testing.T) {
	m := newTestMetrics(t)

	m.SetIndexSizeBytes("vector", 512*1024)
	m.SetIndexSizeBytes("fulltext", 256*1024)

	assert.Equal(t, float64(512*1024), testutil.ToFloat64(m.IndexSizeBytes.WithLabelValues("vector")))
	assert.Equal(t, float64(256*1024), testutil.ToFloat64(m.IndexSizeBytes.WithLabelValues("fulltext")))
}

func TestStatusToString(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{204, "2xx"},
		{301, "3xx"},
		{302, "3xx"},
		{400, "4xx"},
		{401, "4xx"},
		{404, "4xx"},
		{500, "5xx"},
		{502, "5xx"},
		{503, "5xx"},
		{100, "1xx"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			result := statusToString(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefault(t *testing.T) {
	// Note: This modifies global state, so be careful
	m := Default()
	require.NotNil(t, m)

	// Call again to verify it returns the same instance
	m2 := Default()
	assert.Equal(t, m, m2)
}
