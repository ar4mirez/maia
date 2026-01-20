// Package metrics provides Prometheus metrics for MAIA.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all MAIA metrics.
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge

	// Memory operations
	MemoryOperationsTotal   *prometheus.CounterVec
	MemoryOperationDuration *prometheus.HistogramVec
	MemoriesStored          prometheus.Gauge
	MemoriesByNamespace     *prometheus.GaugeVec
	MemoriesByType          *prometheus.GaugeVec

	// Context assembly
	ContextAssemblyDuration *prometheus.HistogramVec
	ContextTokensUsed       *prometheus.HistogramVec

	// Search operations
	SearchOperationsTotal   *prometheus.CounterVec
	SearchOperationDuration *prometheus.HistogramVec
	SearchResultsCount      *prometheus.HistogramVec

	// Embedding operations
	EmbeddingOperationsTotal   *prometheus.CounterVec
	EmbeddingOperationDuration *prometheus.HistogramVec

	// Storage metrics
	StorageSizeBytes    prometheus.Gauge
	StorageOperations   *prometheus.CounterVec
	IndexSizeBytes      *prometheus.GaugeVec

	// Rate limiting
	RateLimitedRequests *prometheus.CounterVec

	// Tenant metrics
	TenantMemoriesTotal    *prometheus.GaugeVec
	TenantStorageBytes     *prometheus.GaugeVec
	TenantRequestsTotal    *prometheus.CounterVec
	TenantQuotaUsage       *prometheus.GaugeVec
	TenantActiveTotal      prometheus.Gauge
	TenantOperationsTotal  *prometheus.CounterVec
}

// New creates a new Metrics instance with all metrics registered.
func New(namespace string) *Metrics {
	if namespace == "" {
		namespace = "maia"
	}

	m := &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Current number of HTTP requests being processed",
			},
		),

		// Memory operations
		MemoryOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "memory_operations_total",
				Help:      "Total number of memory operations",
			},
			[]string{"operation", "namespace", "status"},
		),
		MemoryOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "memory_operation_duration_seconds",
				Help:      "Memory operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .25, .5},
			},
			[]string{"operation", "namespace"},
		),
		MemoriesStored: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memories_stored_total",
				Help:      "Total number of memories stored",
			},
		),
		MemoriesByNamespace: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memories_by_namespace",
				Help:      "Number of memories per namespace",
			},
			[]string{"namespace"},
		),
		MemoriesByType: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memories_by_type",
				Help:      "Number of memories per type",
			},
			[]string{"type"},
		),

		// Context assembly
		ContextAssemblyDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "context_assembly_duration_seconds",
				Help:      "Context assembly duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .2, .5},
			},
			[]string{"namespace"},
		),
		ContextTokensUsed: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "context_tokens_used",
				Help:      "Number of tokens used in context assembly",
				Buckets:   []float64{100, 500, 1000, 2000, 4000, 8000, 16000, 32000},
			},
			[]string{"namespace"},
		),

		// Search operations
		SearchOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "search_operations_total",
				Help:      "Total number of search operations",
			},
			[]string{"type", "namespace", "status"},
		),
		SearchOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "search_operation_duration_seconds",
				Help:      "Search operation duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5},
			},
			[]string{"type", "namespace"},
		),
		SearchResultsCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "search_results_count",
				Help:      "Number of results returned by search operations",
				Buckets:   []float64{0, 1, 5, 10, 25, 50, 100},
			},
			[]string{"type"},
		),

		// Embedding operations
		EmbeddingOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "embedding_operations_total",
				Help:      "Total number of embedding operations",
			},
			[]string{"provider", "status"},
		),
		EmbeddingOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "embedding_operation_duration_seconds",
				Help:      "Embedding operation duration in seconds",
				Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"provider"},
		),

		// Storage metrics
		StorageSizeBytes: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "storage_size_bytes",
				Help:      "Total storage size in bytes",
			},
		),
		StorageOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "storage_operations_total",
				Help:      "Total number of storage operations",
			},
			[]string{"operation", "status"},
		),
		IndexSizeBytes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "index_size_bytes",
				Help:      "Index size in bytes",
			},
			[]string{"index_type"},
		),

		// Rate limiting
		RateLimitedRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limited_requests_total",
				Help:      "Total number of rate limited requests",
			},
			[]string{"client_ip"},
		),

		// Tenant metrics
		TenantMemoriesTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tenant_memories_total",
				Help:      "Total memories stored per tenant",
			},
			[]string{"tenant_id", "plan"},
		),
		TenantStorageBytes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tenant_storage_bytes",
				Help:      "Storage usage in bytes per tenant",
			},
			[]string{"tenant_id"},
		),
		TenantRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tenant_requests_total",
				Help:      "Total requests per tenant",
			},
			[]string{"tenant_id", "method", "status"},
		),
		TenantQuotaUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tenant_quota_usage_ratio",
				Help:      "Quota usage ratio (0-1) per tenant and resource type",
			},
			[]string{"tenant_id", "resource"},
		),
		TenantActiveTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tenants_active_total",
				Help:      "Total number of active tenants",
			},
		),
		TenantOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tenant_operations_total",
				Help:      "Total tenant management operations",
			},
			[]string{"operation", "status"},
		),
	}

	return m
}

// Default returns the default metrics instance.
var defaultMetrics *Metrics

// Default returns the default metrics instance, creating it if needed.
func Default() *Metrics {
	if defaultMetrics == nil {
		defaultMetrics = New("maia")
	}
	return defaultMetrics
}

// RecordHTTPRequest records an HTTP request.
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration float64) {
	statusStr := statusToString(status)
	m.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// RecordMemoryOperation records a memory operation.
func (m *Metrics) RecordMemoryOperation(operation, namespace string, success bool, duration float64) {
	status := "success"
	if !success {
		status = "error"
	}
	m.MemoryOperationsTotal.WithLabelValues(operation, namespace, status).Inc()
	m.MemoryOperationDuration.WithLabelValues(operation, namespace).Observe(duration)
}

// RecordSearchOperation records a search operation.
func (m *Metrics) RecordSearchOperation(searchType, namespace string, success bool, duration float64, resultCount int) {
	status := "success"
	if !success {
		status = "error"
	}
	m.SearchOperationsTotal.WithLabelValues(searchType, namespace, status).Inc()
	m.SearchOperationDuration.WithLabelValues(searchType, namespace).Observe(duration)
	m.SearchResultsCount.WithLabelValues(searchType).Observe(float64(resultCount))
}

// RecordContextAssembly records a context assembly operation.
func (m *Metrics) RecordContextAssembly(namespace string, duration float64, tokensUsed int) {
	m.ContextAssemblyDuration.WithLabelValues(namespace).Observe(duration)
	m.ContextTokensUsed.WithLabelValues(namespace).Observe(float64(tokensUsed))
}

// RecordEmbeddingOperation records an embedding operation.
func (m *Metrics) RecordEmbeddingOperation(provider string, success bool, duration float64) {
	status := "success"
	if !success {
		status = "error"
	}
	m.EmbeddingOperationsTotal.WithLabelValues(provider, status).Inc()
	m.EmbeddingOperationDuration.WithLabelValues(provider).Observe(duration)
}

// RecordRateLimited records a rate limited request.
func (m *Metrics) RecordRateLimited(clientIP string) {
	m.RateLimitedRequests.WithLabelValues(clientIP).Inc()
}

// SetMemoriesStored sets the total number of memories stored.
func (m *Metrics) SetMemoriesStored(count int64) {
	m.MemoriesStored.Set(float64(count))
}

// SetMemoriesByNamespace sets the number of memories for a namespace.
func (m *Metrics) SetMemoriesByNamespace(namespace string, count int64) {
	m.MemoriesByNamespace.WithLabelValues(namespace).Set(float64(count))
}

// SetMemoriesByType sets the number of memories for a type.
func (m *Metrics) SetMemoriesByType(memType string, count int64) {
	m.MemoriesByType.WithLabelValues(memType).Set(float64(count))
}

// SetStorageSizeBytes sets the storage size in bytes.
func (m *Metrics) SetStorageSizeBytes(size int64) {
	m.StorageSizeBytes.Set(float64(size))
}

// SetIndexSizeBytes sets the index size in bytes.
func (m *Metrics) SetIndexSizeBytes(indexType string, size int64) {
	m.IndexSizeBytes.WithLabelValues(indexType).Set(float64(size))
}

func statusToString(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// SetTenantMemories sets the total memories for a tenant.
func (m *Metrics) SetTenantMemories(tenantID, plan string, count int64) {
	m.TenantMemoriesTotal.WithLabelValues(tenantID, plan).Set(float64(count))
}

// SetTenantStorage sets the storage usage for a tenant.
func (m *Metrics) SetTenantStorage(tenantID string, bytes int64) {
	m.TenantStorageBytes.WithLabelValues(tenantID).Set(float64(bytes))
}

// RecordTenantRequest records a request for a tenant.
func (m *Metrics) RecordTenantRequest(tenantID, method string, status int) {
	statusStr := statusToString(status)
	m.TenantRequestsTotal.WithLabelValues(tenantID, method, statusStr).Inc()
}

// SetTenantQuotaUsage sets the quota usage ratio for a tenant resource.
func (m *Metrics) SetTenantQuotaUsage(tenantID, resource string, ratio float64) {
	m.TenantQuotaUsage.WithLabelValues(tenantID, resource).Set(ratio)
}

// SetActiveTenants sets the total number of active tenants.
func (m *Metrics) SetActiveTenants(count int64) {
	m.TenantActiveTotal.Set(float64(count))
}

// RecordTenantOperation records a tenant management operation.
func (m *Metrics) RecordTenantOperation(operation string, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	m.TenantOperationsTotal.WithLabelValues(operation, status).Inc()
}
