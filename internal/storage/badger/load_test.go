package badger

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/storage"
)

// LoadTestConfig configures a load test scenario.
type LoadTestConfig struct {
	// Number of concurrent goroutines
	Concurrency int
	// Total number of operations to perform
	Operations int
	// Ratio of read to write operations (0.0-1.0)
	ReadRatio float64
	// Number of memories to pre-populate
	PrePopulate int
	// Duration for time-based tests
	Duration time.Duration
}

// LoadTestResult holds the results of a load test.
type LoadTestResult struct {
	TotalOps      int64
	SuccessfulOps int64
	FailedOps     int64
	ReadOps       int64
	WriteOps      int64
	Duration      time.Duration
	OpsPerSecond  float64
	AvgLatencyUs  float64
	P50LatencyUs  float64
	P95LatencyUs  float64
	P99LatencyUs  float64
	MaxLatencyUs  float64
	ErrorsByType  map[string]int64
}

// setupLoadTestStore creates a store with optional pre-populated data.
func setupLoadTestStore(t testing.TB, prePopulate int) (*Store, []string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "maia-load-test-*")
	require.NoError(t, err)

	store, err := NewWithPath(dir)
	require.NoError(t, err)

	// Pre-populate memories
	var memoryIDs []string
	ctx := context.Background()

	// Create default namespace
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "load-test",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	require.NoError(t, err)

	for i := 0; i < prePopulate; i++ {
		mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace:  "load-test",
			Content:    fmt.Sprintf("Pre-populated memory content %d with some text for testing", i),
			Type:       storage.MemoryTypeSemantic,
			Tags:       []string{"pre-populated", fmt.Sprintf("batch-%d", i/100)},
			Confidence: 0.8,
		})
		require.NoError(t, err)
		memoryIDs = append(memoryIDs, mem.ID)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(dir)
	}

	return store, memoryIDs, cleanup
}

// TestLoad_ConcurrentReads tests concurrent read performance.
func TestLoad_ConcurrentReads(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		prePopulate  = 1000
		concurrency  = 50
		opsPerWorker = 100
	)

	store, memoryIDs, cleanup := setupLoadTestStore(t, prePopulate)
	defer cleanup()

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount, failCount int64
	var totalLatency int64

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(workerID)))

			for i := 0; i < opsPerWorker; i++ {
				// Random memory ID from pre-populated set
				memID := memoryIDs[rng.Intn(len(memoryIDs))]

				opStart := time.Now()
				_, err := store.GetMemory(ctx, memID)
				latency := time.Since(opStart)

				atomic.AddInt64(&totalLatency, latency.Microseconds())

				if err != nil {
					atomic.AddInt64(&failCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := concurrency * opsPerWorker
	opsPerSec := float64(totalOps) / duration.Seconds()
	avgLatency := float64(totalLatency) / float64(totalOps)

	t.Logf("Concurrent Reads Test Results:")
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Avg Latency: %.2f µs", avgLatency)

	assert.Equal(t, int64(totalOps), successCount, "All reads should succeed")
	assert.Less(t, avgLatency, float64(20000), "Avg latency should be < 20ms")
}

// TestLoad_ConcurrentWrites tests concurrent write performance.
func TestLoad_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		concurrency  = 20
		opsPerWorker = 50
	)

	store, _, cleanup := setupLoadTestStore(t, 0)
	defer cleanup()

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount, failCount int64
	var totalLatency int64
	var createdIDs sync.Map

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				opStart := time.Now()
				mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
					Namespace:  "load-test",
					Content:    fmt.Sprintf("Worker %d memory %d content with some text for testing", workerID, i),
					Type:       storage.MemoryTypeSemantic,
					Tags:       []string{"load-test", fmt.Sprintf("worker-%d", workerID)},
					Confidence: 0.7,
				})
				latency := time.Since(opStart)

				atomic.AddInt64(&totalLatency, latency.Microseconds())

				if err != nil {
					atomic.AddInt64(&failCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
					createdIDs.Store(mem.ID, true)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := concurrency * opsPerWorker
	opsPerSec := float64(totalOps) / duration.Seconds()
	avgLatency := float64(totalLatency) / float64(totalOps)

	// Count unique IDs to verify no duplicates
	var uniqueCount int
	createdIDs.Range(func(_, _ interface{}) bool {
		uniqueCount++
		return true
	})

	t.Logf("Concurrent Writes Test Results:")
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Unique IDs: %d", uniqueCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Avg Latency: %.2f µs", avgLatency)

	assert.Equal(t, int64(totalOps), successCount, "All writes should succeed")
	assert.Equal(t, totalOps, uniqueCount, "All IDs should be unique")
	assert.Less(t, avgLatency, float64(50000), "Avg latency should be < 50ms")
}

// TestLoad_MixedWorkload tests a realistic mixed read/write workload.
func TestLoad_MixedWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		prePopulate  = 500
		concurrency  = 30
		opsPerWorker = 100
		readRatio    = 0.8 // 80% reads, 20% writes
	)

	store, memoryIDs, cleanup := setupLoadTestStore(t, prePopulate)
	defer cleanup()

	ctx := context.Background()
	var wg sync.WaitGroup
	var readSuccess, readFail, writeSuccess, writeFail int64
	var readLatency, writeLatency int64

	// Add initial IDs to map for read access
	var currentIDs sync.Map
	for _, id := range memoryIDs {
		currentIDs.Store(id, true)
	}

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(workerID) + time.Now().UnixNano()))

			for i := 0; i < opsPerWorker; i++ {
				isRead := rng.Float64() < readRatio

				if isRead {
					// Read operation - pick random existing ID
					var targetID string
					currentIDs.Range(func(key, _ interface{}) bool {
						if rng.Float64() < 0.01 || targetID == "" { // 1% chance to pick each
							targetID = key.(string)
						}
						return targetID == ""
					})

					if targetID == "" {
						continue // No IDs available yet
					}

					opStart := time.Now()
					_, err := store.GetMemory(ctx, targetID)
					latency := time.Since(opStart)
					atomic.AddInt64(&readLatency, latency.Microseconds())

					if err != nil {
						atomic.AddInt64(&readFail, 1)
					} else {
						atomic.AddInt64(&readSuccess, 1)
					}
				} else {
					// Write operation
					opStart := time.Now()
					mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
						Namespace:  "load-test",
						Content:    fmt.Sprintf("Mixed workload memory from worker %d iteration %d", workerID, i),
						Type:       storage.MemoryTypeSemantic,
						Tags:       []string{"mixed-workload"},
						Confidence: 0.75,
					})
					latency := time.Since(opStart)
					atomic.AddInt64(&writeLatency, latency.Microseconds())

					if err != nil {
						atomic.AddInt64(&writeFail, 1)
					} else {
						atomic.AddInt64(&writeSuccess, 1)
						currentIDs.Store(mem.ID, true)
					}
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalReads := readSuccess + readFail
	totalWrites := writeSuccess + writeFail
	totalOps := totalReads + totalWrites

	var avgReadLatency, avgWriteLatency float64
	if totalReads > 0 {
		avgReadLatency = float64(readLatency) / float64(totalReads)
	}
	if totalWrites > 0 {
		avgWriteLatency = float64(writeLatency) / float64(totalWrites)
	}

	t.Logf("Mixed Workload Test Results:")
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Reads: %d success, %d fail (%.1f%% of total)", readSuccess, readFail, float64(totalReads)/float64(totalOps)*100)
	t.Logf("  Writes: %d success, %d fail (%.1f%% of total)", writeSuccess, writeFail, float64(totalWrites)/float64(totalOps)*100)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", float64(totalOps)/duration.Seconds())
	t.Logf("  Avg Read Latency: %.2f µs", avgReadLatency)
	t.Logf("  Avg Write Latency: %.2f µs", avgWriteLatency)

	assert.Equal(t, int64(0), readFail, "All reads should succeed")
	assert.Equal(t, int64(0), writeFail, "All writes should succeed")
}

// TestLoad_ListPerformance tests list operation performance under load.
func TestLoad_ListPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		prePopulate  = 2000
		concurrency  = 20
		opsPerWorker = 30
	)

	store, _, cleanup := setupLoadTestStore(t, prePopulate)
	defer cleanup()

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount, failCount int64
	var totalLatency int64

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(workerID)))

			// Different list limits
			limits := []int{10, 20, 50, 100}

			for i := 0; i < opsPerWorker; i++ {
				limit := limits[rng.Intn(len(limits))]

				opStart := time.Now()
				_, err := store.ListMemories(ctx, "load-test", &storage.ListOptions{
					Limit: limit,
				})
				latency := time.Since(opStart)

				atomic.AddInt64(&totalLatency, latency.Microseconds())

				if err != nil {
					atomic.AddInt64(&failCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := concurrency * opsPerWorker
	opsPerSec := float64(totalOps) / duration.Seconds()
	avgLatency := float64(totalLatency) / float64(totalOps)

	t.Logf("List Performance Test Results:")
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Avg Latency: %.2f µs", avgLatency)

	assert.Equal(t, int64(totalOps), successCount, "All list operations should succeed")
}

// TestLoad_NamespaceIsolation tests that namespaces are properly isolated under concurrent access.
func TestLoad_NamespaceIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numNamespaces = 5
		opsPerNS      = 100
		concurrency   = 10
	)

	store, _, cleanup := setupLoadTestStore(t, 0)
	defer cleanup()

	ctx := context.Background()

	// Create namespaces
	for i := 0; i < numNamespaces; i++ {
		_, err := store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
			Name: fmt.Sprintf("ns-%d", i),
			Config: storage.NamespaceConfig{
				TokenBudget: 5000,
			},
		})
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	memoryCountPerNS := make([]int64, numNamespaces)

	// Concurrent writes to different namespaces
	for ns := 0; ns < numNamespaces; ns++ {
		for w := 0; w < concurrency/numNamespaces; w++ {
			wg.Add(1)
			go func(nsID, workerID int) {
				defer wg.Done()

				for i := 0; i < opsPerNS; i++ {
					_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
						Namespace:  fmt.Sprintf("ns-%d", nsID),
						Content:    fmt.Sprintf("NS %d Worker %d Memory %d", nsID, workerID, i),
						Type:       storage.MemoryTypeSemantic,
						Confidence: 0.8,
					})
					if err == nil {
						atomic.AddInt64(&memoryCountPerNS[nsID], 1)
					}
				}
			}(ns, w)
		}
	}

	wg.Wait()

	// Verify isolation - each namespace should have its own memories
	for ns := 0; ns < numNamespaces; ns++ {
		memories, err := store.ListMemories(ctx, fmt.Sprintf("ns-%d", ns), &storage.ListOptions{
			Limit: 10000,
		})
		require.NoError(t, err)

		expectedCount := memoryCountPerNS[ns]
		actualCount := int64(len(memories))

		t.Logf("Namespace ns-%d: expected %d, got %d memories", ns, expectedCount, actualCount)
		assert.Equal(t, expectedCount, actualCount, "Namespace %d should have correct memory count", ns)

		// Verify all memories belong to the correct namespace
		for _, mem := range memories {
			assert.Equal(t, fmt.Sprintf("ns-%d", ns), mem.Namespace)
		}
	}
}

// TestLoad_BatchOperations tests batch operation performance.
func TestLoad_BatchOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const iterations = 10

	store, _, cleanup := setupLoadTestStore(t, 0)
	defer cleanup()

	ctx := context.Background()
	sizes := []int{10, 50, 100, 250, 500}

	for _, size := range sizes {
		var totalLatency int64
		var successCount int64

		for iter := 0; iter < iterations; iter++ {
			// Create batch of memories
			inputs := make([]*storage.CreateMemoryInput, size)
			for i := 0; i < size; i++ {
				inputs[i] = &storage.CreateMemoryInput{
					Namespace:  "load-test",
					Content:    fmt.Sprintf("Batch %d memory %d content for batch operation testing", iter, i),
					Type:       storage.MemoryTypeSemantic,
					Tags:       []string{"batch-test"},
					Confidence: 0.8,
				}
			}

			opStart := time.Now()
			memories, err := store.BatchCreateMemories(ctx, inputs)
			latency := time.Since(opStart)

			if err == nil && len(memories) == size {
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalLatency, latency.Microseconds())
			}
		}

		avgLatency := float64(totalLatency) / float64(iterations)
		avgPerItem := avgLatency / float64(size)

		t.Logf("Batch Size %d: %d/%d success, avg batch latency %.2f µs, avg per item %.2f µs",
			size, successCount, iterations, avgLatency, avgPerItem)

		assert.Equal(t, int64(iterations), successCount, "All batch operations should succeed for size %d", size)
	}
}

// Benchmark functions for more precise measurements

// BenchmarkStore_CreateMemory benchmarks single memory creation.
func BenchmarkStore_CreateMemory(b *testing.B) {
	dir, err := os.MkdirTemp("", "maia-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewWithPath(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "bench",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace:  "bench",
			Content:    fmt.Sprintf("Benchmark memory content %d", i),
			Type:       storage.MemoryTypeSemantic,
			Confidence: 0.8,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_GetMemory benchmarks single memory retrieval.
func BenchmarkStore_GetMemory(b *testing.B) {
	dir, err := os.MkdirTemp("", "maia-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewWithPath(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "bench",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	// Create test memory
	mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "bench",
		Content:    "Benchmark memory content for get operation",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 0.8,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetMemory(ctx, mem.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_ListMemories benchmarks listing memories.
func BenchmarkStore_ListMemories(b *testing.B) {
	dir, err := os.MkdirTemp("", "maia-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewWithPath(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "bench",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	// Pre-populate with 1000 memories
	for i := 0; i < 1000; i++ {
		_, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace:  "bench",
			Content:    fmt.Sprintf("Benchmark memory content %d", i),
			Type:       storage.MemoryTypeSemantic,
			Confidence: 0.8,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.ListMemories(ctx, "bench", &storage.ListOptions{
			Limit: 100,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_BatchCreateMemories benchmarks batch memory creation.
func BenchmarkStore_BatchCreateMemories(b *testing.B) {
	dir, err := os.MkdirTemp("", "maia-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewWithPath(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "bench",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	// Create batch input
	batchSize := 100
	inputs := make([]*storage.CreateMemoryInput, batchSize)
	for i := 0; i < batchSize; i++ {
		inputs[i] = &storage.CreateMemoryInput{
			Namespace:  "bench",
			Content:    fmt.Sprintf("Batch benchmark memory content %d", i),
			Type:       storage.MemoryTypeSemantic,
			Confidence: 0.8,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.BatchCreateMemories(ctx, inputs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_ConcurrentReads benchmarks concurrent read operations.
func BenchmarkStore_ConcurrentReads(b *testing.B) {
	dir, err := os.MkdirTemp("", "maia-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewWithPath(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: "bench",
		Config: storage.NamespaceConfig{
			TokenBudget: 10000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	// Pre-populate
	var memIDs []string
	for i := 0; i < 1000; i++ {
		mem, err := store.CreateMemory(ctx, &storage.CreateMemoryInput{
			Namespace:  "bench",
			Content:    fmt.Sprintf("Benchmark memory content %d", i),
			Type:       storage.MemoryTypeSemantic,
			Confidence: 0.8,
		})
		if err != nil {
			b.Fatal(err)
		}
		memIDs = append(memIDs, mem.ID)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			memID := memIDs[rng.Intn(len(memIDs))]
			_, err := store.GetMemory(ctx, memID)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
