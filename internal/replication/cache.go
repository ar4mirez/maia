package replication

import (
	"context"
	"sync"
	"time"
)

// PlacementCache provides TTL-based caching for tenant placements.
type PlacementCache struct {
	manager ReplicationManager
	ttl     time.Duration
	entries map[string]*cacheEntry
	mu      sync.RWMutex

	// Metrics
	hits   int64
	misses int64
}

type cacheEntry struct {
	placement *TenantPlacement
	expiresAt time.Time
}

// PlacementCacheConfig configures the placement cache.
type PlacementCacheConfig struct {
	// TTL is how long entries are cached before refresh.
	TTL time.Duration

	// Manager is the replication manager to fetch placements from.
	Manager ReplicationManager
}

// NewPlacementCache creates a new placement cache.
func NewPlacementCache(cfg *PlacementCacheConfig) *PlacementCache {
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 30 * time.Second
	}

	return &PlacementCache{
		manager: cfg.Manager,
		ttl:     ttl,
		entries: make(map[string]*cacheEntry),
	}
}

// Get retrieves a tenant placement from cache or fetches from manager.
func (c *PlacementCache) Get(ctx context.Context, tenantID string) (*TenantPlacement, error) {
	// Try cache first
	c.mu.RLock()
	entry, ok := c.entries[tenantID]
	if ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		c.hits++
		c.mu.Unlock()
		return entry.placement, nil
	}
	c.mu.RUnlock()

	// Fetch from manager
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()

	placement, err := c.manager.GetTenantPlacement(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	c.mu.Lock()
	c.entries[tenantID] = &cacheEntry{
		placement: placement,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return placement, nil
}

// Invalidate removes a specific tenant from the cache.
func (c *PlacementCache) Invalidate(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, tenantID)
}

// InvalidateAll clears the entire cache.
func (c *PlacementCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// Stats returns cache statistics.
func (c *PlacementCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    len(c.entries),
		HitRate: c.hitRate(),
	}
}

// CacheStats contains cache performance statistics.
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Size    int     `json:"size"`
	HitRate float64 `json:"hit_rate"`
}

func (c *PlacementCache) hitRate() float64 {
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

// Cleanup removes expired entries from the cache.
func (c *PlacementCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for tenantID, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, tenantID)
		}
	}
}

// StartCleanup starts a background goroutine to periodically clean expired entries.
func (c *PlacementCache) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Cleanup()
			}
		}
	}()
}
