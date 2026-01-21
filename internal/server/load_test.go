package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
	"github.com/ar4mirez/maia/internal/tenant"
)

// ServerLoadTestConfig configures a server load test.
type ServerLoadTestConfig struct {
	Concurrency int
	Operations  int
	ReadRatio   float64
	WithTenant  bool
}

// ServerLoadTestResult holds the results.
type ServerLoadTestResult struct {
	TotalOps     int64
	SuccessOps   int64
	FailedOps    int64
	Duration     time.Duration
	OpsPerSecond float64
	AvgLatencyUs float64
}

func setupLoadTestServer(t testing.TB, withTenant bool) (*Server, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create store
	store, err := badger.NewWithPath(tmpDir + "/data")
	require.NoError(t, err)

	cfg := &config.Config{
		Log: config.LogConfig{Level: "error"},
	}
	logger := zap.NewNop()

	var deps *ServerDeps
	if withTenant {
		// Create tenant manager
		opts := badgerdb.DefaultOptions(tmpDir + "/tenants")
		opts.Logger = nil
		db, err := badgerdb.Open(opts)
		require.NoError(t, err)

		manager := tenant.NewBadgerManager(db)

		// Create test tenant
		_, err = manager.Create(context.Background(), &tenant.CreateTenantInput{
			Name: "load-test-tenant",
			Plan: tenant.PlanStandard,
		})
		require.NoError(t, err)

		deps = &ServerDeps{
			TenantManager: manager,
		}
	}

	server := NewWithDeps(cfg, store, logger, deps)

	cleanup := func() {
		store.Close()
	}

	return server, cleanup
}

func TestServerLoad_ConcurrentMemoryOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	server, cleanup := setupLoadTestServer(t, false)
	defer cleanup()

	config := ServerLoadTestConfig{
		Concurrency: 20,
		Operations:  1000,
		ReadRatio:   0.8,
		WithTenant:  false,
	}

	result := runServerLoadTest(t, server, config)

	t.Logf("Server Load Test Results:")
	t.Logf("  Total Operations: %d", result.TotalOps)
	t.Logf("  Successful: %d, Failed: %d", result.SuccessOps, result.FailedOps)
	t.Logf("  Duration: %v", result.Duration)
	t.Logf("  Throughput: %.2f ops/sec", result.OpsPerSecond)
	t.Logf("  Avg Latency: %.2f µs", result.AvgLatencyUs)

	// Verify at least 90% success rate
	successRate := float64(result.SuccessOps) / float64(result.TotalOps)
	require.GreaterOrEqual(t, successRate, 0.90, "Success rate should be at least 90%")
}

func TestServerLoad_TenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	server, cleanup := setupLoadTestServer(t, true)
	defer cleanup()

	// Get tenant ID
	tenantResp, err := server.tenants.GetByName(context.Background(), "load-test-tenant")
	require.NoError(t, err)
	tenantID := tenantResp.ID

	var successOps int64
	var failedOps int64
	var wg sync.WaitGroup

	start := time.Now()

	// Create memories with tenant context
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			body := fmt.Sprintf(`{"namespace":"default","content":"Memory %d","type":"semantic"}`, idx)
			req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-MAIA-Tenant-ID", tenantID)

			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			if w.Code == http.StatusCreated {
				atomic.AddInt64(&successOps, 1)
			} else {
				atomic.AddInt64(&failedOps, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Tenant Isolation Load Test Results:")
	t.Logf("  Successful: %d, Failed: %d", successOps, failedOps)
	t.Logf("  Duration: %v", duration)

	// Verify all operations succeeded
	require.Equal(t, int64(100), successOps, "All operations should succeed")
}

func TestServerLoad_NamespaceOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	server, cleanup := setupLoadTestServer(t, false)
	defer cleanup()

	var successOps int64
	var failedOps int64
	var wg sync.WaitGroup

	start := time.Now()

	// Create multiple namespaces concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			body := fmt.Sprintf(`{"name":"namespace-%d","config":{"token_budget":4000}}`, idx)
			req := httptest.NewRequest(http.MethodPost, "/v1/namespaces", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			if w.Code == http.StatusCreated {
				atomic.AddInt64(&successOps, 1)
			} else {
				atomic.AddInt64(&failedOps, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Namespace Load Test Results:")
	t.Logf("  Successful: %d, Failed: %d", successOps, failedOps)
	t.Logf("  Duration: %v", duration)

	// Verify high success rate
	require.GreaterOrEqual(t, successOps, int64(45), "At least 90% of namespace creations should succeed")
}

func runServerLoadTest(t testing.TB, server *Server, config ServerLoadTestConfig) ServerLoadTestResult {
	t.Helper()

	// First, create some memories for read operations
	ctx := context.Background()
	var memoryIDs []string
	for i := 0; i < 100; i++ {
		body := fmt.Sprintf(`{"namespace":"default","content":"Pre-populated memory %d","type":"semantic"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		if w.Code == http.StatusCreated {
			var mem storage.Memory
			if err := json.NewDecoder(w.Body).Decode(&mem); err == nil {
				memoryIDs = append(memoryIDs, mem.ID)
			}
		}
	}
	_ = ctx // Used for context

	var successOps int64
	var failedOps int64
	var totalLatencyNs int64
	var wg sync.WaitGroup

	opsPerWorker := config.Operations / config.Concurrency
	start := time.Now()

	for w := 0; w < config.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				opStart := time.Now()
				var success bool

				// Determine operation type based on read ratio
				if len(memoryIDs) > 0 && float64(i%100)/100.0 < config.ReadRatio {
					// Read operation
					memID := memoryIDs[i%len(memoryIDs)]
					req := httptest.NewRequest(http.MethodGet, "/v1/memories/"+memID, nil)
					w := httptest.NewRecorder()
					server.router.ServeHTTP(w, req)
					success = w.Code == http.StatusOK
				} else {
					// Write operation
					body := fmt.Sprintf(`{"namespace":"default","content":"Worker %d memory %d","type":"semantic"}`, workerID, i)
					req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					server.router.ServeHTTP(w, req)
					success = w.Code == http.StatusCreated
				}

				latency := time.Since(opStart).Nanoseconds()
				atomic.AddInt64(&totalLatencyNs, latency)

				if success {
					atomic.AddInt64(&successOps, 1)
				} else {
					atomic.AddInt64(&failedOps, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := successOps + failedOps
	opsPerSec := float64(totalOps) / duration.Seconds()
	avgLatency := float64(totalLatencyNs) / float64(totalOps) / 1000.0 // Convert to µs

	return ServerLoadTestResult{
		TotalOps:     totalOps,
		SuccessOps:   successOps,
		FailedOps:    failedOps,
		Duration:     duration,
		OpsPerSecond: opsPerSec,
		AvgLatencyUs: avgLatency,
	}
}

func BenchmarkServer_CreateMemory(b *testing.B) {
	server, cleanup := setupLoadTestServer(b, false)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := fmt.Sprintf(`{"namespace":"default","content":"Benchmark memory %d","type":"semantic"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

func BenchmarkServer_GetMemory(b *testing.B) {
	server, cleanup := setupLoadTestServer(b, false)
	defer cleanup()

	// Create a memory to read
	body := `{"namespace":"default","content":"Benchmark read memory","type":"semantic"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	var mem storage.Memory
	err := json.NewDecoder(w.Body).Decode(&mem)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/memories/"+mem.ID, nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

func BenchmarkServer_SearchMemories(b *testing.B) {
	server, cleanup := setupLoadTestServer(b, false)
	defer cleanup()

	// Create some memories to search
	for i := 0; i < 100; i++ {
		body := fmt.Sprintf(`{"namespace":"default","content":"Searchable memory about topic %d","type":"semantic"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := `{"namespace":"default","limit":10}`
		req := httptest.NewRequest(http.MethodPost, "/v1/memories/search", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}
