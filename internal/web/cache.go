package web

import (
	"sync"
	"time"
)

// Cache provides thread-safe caching with TTL for expensive operations.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// NewCache creates a new cache instance.
func NewCache() *Cache {
	return &Cache{
		entries: make(map[string]*cacheEntry),
	}
}

// Get retrieves a value from the cache. Returns nil if not found or expired.
func (c *Cache) Get(key string) interface{} {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.value
}

// Set stores a value in the cache with the given TTL.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// GetOrCompute retrieves from cache or computes and caches the value.
func (c *Cache) GetOrCompute(key string, ttl time.Duration, compute func() interface{}) interface{} {
	// Try cache first
	if val := c.Get(key); val != nil {
		return val
	}

	// Compute and cache
	val := compute()
	if val != nil {
		c.Set(key, val, ttl)
	}
	return val
}

// StatusCache caches the full status response.
type StatusCache struct {
	mu        sync.RWMutex
	status    *StatusResponse
	expiresAt time.Time
	ttl       time.Duration
}

// NewStatusCache creates a status cache with the given TTL.
func NewStatusCache(ttl time.Duration) *StatusCache {
	return &StatusCache{
		ttl: ttl,
	}
}

// Get returns the cached status if valid, nil otherwise.
func (s *StatusCache) Get() *StatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status == nil || time.Now().After(s.expiresAt) {
		return nil
	}
	return s.status
}

// Set stores a status response in the cache.
func (s *StatusCache) Set(status *StatusResponse) {
	s.mu.Lock()
	s.status = status
	s.expiresAt = time.Now().Add(s.ttl)
	s.mu.Unlock()
}

// GetOrBuild returns cached status or builds and caches a new one.
func (s *StatusCache) GetOrBuild(build func() StatusResponse) StatusResponse {
	if cached := s.Get(); cached != nil {
		return *cached
	}

	status := build()
	s.Set(&status)
	return status
}

// Cache TTL constants for different data types.
const (
	// StatusCacheTTL is how long to cache the full dashboard status.
	// 10 seconds is a good balance between freshness and performance.
	StatusCacheTTL = 10 * time.Second

	// ConvoyCacheTTL is how long to cache convoy data.
	ConvoyCacheTTL = 15 * time.Second

	// AgentCacheTTL is how long to cache agent data.
	AgentCacheTTL = 10 * time.Second

	// IssuesCacheTTL is how long to cache issue lists.
	IssuesCacheTTL = 20 * time.Second

	// GitCacheTTL is how long to cache git data.
	GitCacheTTL = 30 * time.Second

	// SystemCacheTTL is how long to cache system info.
	SystemCacheTTL = 60 * time.Second
)
