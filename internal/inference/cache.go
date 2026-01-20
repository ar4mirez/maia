package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cache provides response caching for inference requests.
// It uses an in-memory LRU cache with optional persistence to MAIA's storage.
type Cache struct {
	config  CacheConfig
	entries map[string]*cacheEntry
	order   []string // LRU order tracking
	mu      sync.RWMutex
	maxSize int
	stats   CacheStats
}

// cacheEntry holds a cached response with metadata.
type cacheEntry struct {
	Response  *CompletionResponse `json:"response"`
	Key       string              `json:"key"`
	CreatedAt time.Time           `json:"created_at"`
	ExpiresAt time.Time           `json:"expires_at"`
	HitCount  int                 `json:"hit_count"`
}

// CacheStats holds cache statistics.
type CacheStats struct {
	Hits       int64     `json:"hits"`
	Misses     int64     `json:"misses"`
	Evictions  int64     `json:"evictions"`
	Size       int       `json:"size"`
	LastAccess time.Time `json:"last_access"`
}

// NewCache creates a new inference cache.
func NewCache(config CacheConfig) *Cache {
	maxSize := 1000 // Default max entries
	if config.MaxEntries > 0 {
		maxSize = config.MaxEntries
	}

	return &Cache{
		config:  config,
		entries: make(map[string]*cacheEntry),
		order:   make([]string, 0),
		maxSize: maxSize,
	}
}

// Get retrieves a cached response for the given request.
// Returns nil if not found or expired.
func (c *Cache) Get(ctx context.Context, req *CompletionRequest) *CompletionResponse {
	if !c.config.Enabled {
		return nil
	}

	key := c.generateKey(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.stats.Misses++
		return nil
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		c.removeEntry(key)
		c.stats.Misses++
		return nil
	}

	// Update stats and LRU order
	entry.HitCount++
	c.stats.Hits++
	c.stats.LastAccess = time.Now()
	c.moveToFront(key)

	return entry.Response
}

// Set stores a response in the cache.
func (c *Cache) Set(ctx context.Context, req *CompletionRequest, resp *CompletionResponse) {
	if !c.config.Enabled {
		return
	}

	// Don't cache streaming responses or errors
	if req.Stream || resp == nil || len(resp.Choices) == 0 {
		return
	}

	key := c.generateKey(req)
	now := time.Now()

	ttl := c.config.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	entry := &cacheEntry{
		Response:  resp,
		Key:       key,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		HitCount:  0,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	for len(c.entries) >= c.maxSize && len(c.order) > 0 {
		c.evictLRU()
	}

	c.entries[key] = entry
	c.order = append([]string{key}, c.order...)
}

// Invalidate removes a specific entry from the cache.
func (c *Cache) Invalidate(ctx context.Context, req *CompletionRequest) {
	key := c.generateKey(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeEntry(key)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	c.order = make([]string, 0)
}

// Stats returns current cache statistics.
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.Size = len(c.entries)
	return stats
}

// generateKey creates a unique cache key from a request.
// The key is based on: model, messages, temperature, top_p, max_tokens.
func (c *Cache) generateKey(req *CompletionRequest) string {
	// Build a canonical representation
	var parts []string

	parts = append(parts, fmt.Sprintf("model:%s", req.Model))

	// Sort and include messages
	for i, msg := range req.Messages {
		parts = append(parts, fmt.Sprintf("msg%d:%s:%s", i, msg.Role, msg.Content))
	}

	// Include generation parameters that affect output
	if req.Temperature != nil {
		parts = append(parts, fmt.Sprintf("temp:%.2f", *req.Temperature))
	}
	if req.TopP != nil {
		parts = append(parts, fmt.Sprintf("top_p:%.2f", *req.TopP))
	}
	if req.MaxTokens != nil {
		parts = append(parts, fmt.Sprintf("max_tokens:%d", *req.MaxTokens))
	}

	// Sort stop sequences for consistency
	if len(req.Stop) > 0 {
		sortedStop := make([]string, len(req.Stop))
		copy(sortedStop, req.Stop)
		sort.Strings(sortedStop)
		parts = append(parts, fmt.Sprintf("stop:%s", strings.Join(sortedStop, ",")))
	}

	// Hash the combined parts
	combined := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// moveToFront moves a key to the front of the LRU order.
func (c *Cache) moveToFront(key string) {
	for i, k := range c.order {
		if k == key {
			// Remove from current position
			c.order = append(c.order[:i], c.order[i+1:]...)
			// Add to front
			c.order = append([]string{key}, c.order...)
			return
		}
	}
}

// removeEntry removes an entry from the cache.
func (c *Cache) removeEntry(key string) {
	delete(c.entries, key)

	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// evictLRU removes the least recently used entry.
func (c *Cache) evictLRU() {
	if len(c.order) == 0 {
		return
	}

	// Remove the last entry (least recently used)
	key := c.order[len(c.order)-1]
	c.order = c.order[:len(c.order)-1]
	delete(c.entries, key)
	c.stats.Evictions++
}

// CachingRouter wraps a router with caching capabilities.
type CachingRouter struct {
	router *DefaultRouter
	cache  *Cache
}

// NewCachingRouter creates a new caching router.
func NewCachingRouter(router *DefaultRouter, cache *Cache) *CachingRouter {
	return &CachingRouter{
		router: router,
		cache:  cache,
	}
}

// Complete performs a completion with caching.
func (cr *CachingRouter) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Check cache first
	if cached := cr.cache.Get(ctx, req); cached != nil {
		return cached, nil
	}

	// Execute request
	resp, err := cr.router.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	// Cache the response
	cr.cache.Set(ctx, req, resp)

	return resp, nil
}

// Stream delegates to the underlying router (no caching for streams).
func (cr *CachingRouter) Stream(ctx context.Context, req *CompletionRequest) (StreamReader, error) {
	return cr.router.Stream(ctx, req)
}

// Route delegates to the underlying router.
func (cr *CachingRouter) Route(ctx context.Context, modelID string) (Provider, error) {
	return cr.router.Route(ctx, modelID)
}

// RouteWithOptions delegates to the underlying router.
func (cr *CachingRouter) RouteWithOptions(ctx context.Context, modelID string, explicitProvider string) (Provider, error) {
	return cr.router.RouteWithOptions(ctx, modelID, explicitProvider)
}

// RegisterProvider delegates to the underlying router.
func (cr *CachingRouter) RegisterProvider(p Provider) error {
	return cr.router.RegisterProvider(p)
}

// ListProviders delegates to the underlying router.
func (cr *CachingRouter) ListProviders() []Provider {
	return cr.router.ListProviders()
}

// GetProvider delegates to the underlying router.
func (cr *CachingRouter) GetProvider(name string) (Provider, bool) {
	return cr.router.GetProvider(name)
}

// ListModels delegates to the underlying router.
func (cr *CachingRouter) ListModels(ctx context.Context) ([]Model, error) {
	return cr.router.ListModels(ctx)
}

// GetHealthChecker returns the health checker from the underlying router.
func (cr *CachingRouter) GetHealthChecker() *HealthChecker {
	return cr.router.GetHealthChecker()
}

// Cache returns the cache instance.
func (cr *CachingRouter) Cache() *Cache {
	return cr.cache
}

// Close closes the underlying router.
func (cr *CachingRouter) Close() error {
	return cr.router.Close()
}

// SerializableCache is a JSON-serializable cache state for persistence.
type SerializableCache struct {
	Entries []cacheEntry `json:"entries"`
	Stats   CacheStats   `json:"stats"`
}

// Export exports the cache state for persistence.
func (c *Cache) Export() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]cacheEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, *entry)
	}

	sc := SerializableCache{
		Entries: entries,
		Stats:   c.stats,
	}

	return json.Marshal(sc)
}

// Import imports a previously exported cache state.
func (c *Cache) Import(data []byte) error {
	var sc SerializableCache
	if err := json.Unmarshal(data, &sc); err != nil {
		return fmt.Errorf("unmarshal cache: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.entries = make(map[string]*cacheEntry)
	c.order = make([]string, 0)

	for _, entry := range sc.Entries {
		// Skip expired entries
		if now.After(entry.ExpiresAt) {
			continue
		}

		entryCopy := entry
		c.entries[entry.Key] = &entryCopy
		c.order = append(c.order, entry.Key)
	}

	c.stats = sc.Stats
	c.stats.Size = len(c.entries)

	return nil
}
