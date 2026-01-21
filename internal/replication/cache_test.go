package replication

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockManager implements a minimal ReplicationManager for testing
type mockManager struct {
	placements map[string]*TenantPlacement
	mu         sync.RWMutex
	fetchCount int
	fetchErr   error
}

func newMockManager() *mockManager {
	return &mockManager{
		placements: make(map[string]*TenantPlacement),
	}
}

func (m *mockManager) GetTenantPlacement(_ context.Context, tenantID string) (*TenantPlacement, error) {
	m.mu.Lock()
	m.fetchCount++
	m.mu.Unlock()

	if m.fetchErr != nil {
		return nil, m.fetchErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.placements[tenantID]; ok {
		return p, nil
	}
	return nil, ErrInvalidPlacement
}

func (m *mockManager) setPlacement(p *TenantPlacement) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.placements[p.TenantID] = p
}

func (m *mockManager) getFetchCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fetchCount
}

// Implement other ReplicationManager methods (not used in cache tests)
func (m *mockManager) Start(_ context.Context) error                                      { return nil }
func (m *mockManager) Stop(_ context.Context) error                                       { return nil }
func (m *mockManager) Role() Role                                                         { return RoleLeader }
func (m *mockManager) Region() string                                                     { return "us-west-1" }
func (m *mockManager) Position(_ context.Context) (*WALPosition, error)                   { return nil, nil }
func (m *mockManager) Stats(_ context.Context) (*ReplicationStats, error)                 { return nil, nil }
func (m *mockManager) AddFollower(_ context.Context, _ *FollowerConfig) error             { return nil }
func (m *mockManager) RemoveFollower(_ context.Context, _ string) error                   { return nil }
func (m *mockManager) GetFollowerStatus(_ context.Context, _ string) (*FollowerStatus, error) {
	return nil, nil
}
func (m *mockManager) ListFollowers(_ context.Context) ([]FollowerStatus, error)      { return nil, nil }
func (m *mockManager) GetLeaderInfo(_ context.Context) (*LeaderInfo, error)           { return nil, nil }
func (m *mockManager) SetLeader(_ context.Context, _ string) error                    { return nil }
func (m *mockManager) SetTenantPlacement(_ context.Context, _ *TenantPlacement) error { return nil }
func (m *mockManager) IsLocalTenant(_ context.Context, _ string) (bool, error)        { return true, nil }

func TestPlacementCache_Get_CacheMiss(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()
	placement, err := cache.Get(ctx, "tenant-1")

	require.NoError(t, err)
	assert.Equal(t, "tenant-1", placement.TenantID)
	assert.Equal(t, "us-west-1", placement.PrimaryRegion)
	assert.Equal(t, 1, manager.getFetchCount())

	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestPlacementCache_Get_CacheHit(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()

	// First call - cache miss
	_, err := cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.getFetchCount())

	// Second call - cache hit
	placement, err := cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", placement.TenantID)
	assert.Equal(t, 1, manager.getFetchCount()) // Still 1, no new fetch

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestPlacementCache_Get_Expiration(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     10 * time.Millisecond,
	})

	ctx := context.Background()

	// First call - cache miss
	_, err := cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.getFetchCount())

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Second call - cache expired, fetch again
	_, err = cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, 2, manager.getFetchCount())
}

func TestPlacementCache_Get_Error(t *testing.T) {
	manager := newMockManager()
	manager.fetchErr = errors.New("database error")

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()
	_, err := cache.Get(ctx, "tenant-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestPlacementCache_Get_NotFound(t *testing.T) {
	manager := newMockManager()
	// No placement set for tenant-1

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()
	_, err := cache.Get(ctx, "tenant-1")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlacement)
}

func TestPlacementCache_Invalidate(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()

	// Populate cache
	_, err := cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.getFetchCount())

	// Invalidate
	cache.Invalidate("tenant-1")

	// Fetch again - should hit manager
	_, err = cache.Get(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, 2, manager.getFetchCount())
}

func TestPlacementCache_InvalidateAll(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-2",
		PrimaryRegion: "eu-central-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()

	// Populate cache
	_, _ = cache.Get(ctx, "tenant-1")
	_, _ = cache.Get(ctx, "tenant-2")
	assert.Equal(t, 2, manager.getFetchCount())

	// Invalidate all
	cache.InvalidateAll()

	stats := cache.Stats()
	assert.Equal(t, 0, stats.Size)

	// Fetch again - both should hit manager
	_, _ = cache.Get(ctx, "tenant-1")
	_, _ = cache.Get(ctx, "tenant-2")
	assert.Equal(t, 4, manager.getFetchCount())
}

func TestPlacementCache_Stats(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0, stats.Size)
	assert.Equal(t, 0.0, stats.HitRate)

	// One miss
	_, _ = cache.Get(ctx, "tenant-1")
	stats = cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 1, stats.Size)
	assert.Equal(t, 0.0, stats.HitRate)

	// One hit
	_, _ = cache.Get(ctx, "tenant-1")
	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 0.5, stats.HitRate)

	// More hits
	_, _ = cache.Get(ctx, "tenant-1")
	_, _ = cache.Get(ctx, "tenant-1")
	stats = cache.Stats()
	assert.Equal(t, int64(3), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 0.75, stats.HitRate)
}

func TestPlacementCache_Cleanup(t *testing.T) {
	manager := newMockManager()
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-1",
		PrimaryRegion: "us-west-1",
		Mode:          PlacementSingle,
	})
	manager.setPlacement(&TenantPlacement{
		TenantID:      "tenant-2",
		PrimaryRegion: "eu-central-1",
		Mode:          PlacementSingle,
	})

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     10 * time.Millisecond,
	})

	ctx := context.Background()

	// Populate cache
	_, _ = cache.Get(ctx, "tenant-1")
	_, _ = cache.Get(ctx, "tenant-2")
	assert.Equal(t, 2, cache.Stats().Size)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Run cleanup
	cache.Cleanup()

	assert.Equal(t, 0, cache.Stats().Size)
}

func TestPlacementCache_Concurrent(t *testing.T) {
	manager := newMockManager()
	for i := 0; i < 100; i++ {
		manager.setPlacement(&TenantPlacement{
			TenantID:      "tenant-" + string(rune('a'+i%26)),
			PrimaryRegion: "us-west-1",
			Mode:          PlacementSingle,
		})
	}

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     time.Minute,
	})

	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tenantID := "tenant-" + string(rune('a'+n%26))
			_, _ = cache.Get(ctx, tenantID)
		}(i)
	}

	// Concurrent invalidations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tenantID := "tenant-" + string(rune('a'+n%26))
			cache.Invalidate(tenantID)
		}(i)
	}

	wg.Wait()
	// Just verify no race conditions - no specific assertions
}

func TestPlacementCache_DefaultTTL(t *testing.T) {
	manager := newMockManager()

	cache := NewPlacementCache(&PlacementCacheConfig{
		Manager: manager,
		TTL:     0, // Should use default
	})

	assert.Equal(t, 30*time.Second, cache.ttl)
}
