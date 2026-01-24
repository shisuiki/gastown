package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Cache provides disk-backed caching with stale-while-revalidate pattern.
// Data is served immediately from cache (even if stale), then refreshed in background.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	cacheDir string
	refresh  map[string]bool // tracks keys currently being refreshed
}

type cacheEntry struct {
	Value     interface{} `json:"value"`
	ExpiresAt time.Time   `json:"expires_at"`
	StaleAt   time.Time   `json:"stale_at"` // when to start background refresh
}

// diskEntry is the JSON structure for disk storage.
type diskEntry struct {
	Value     json.RawMessage `json:"value"`
	ExpiresAt time.Time       `json:"expires_at"`
	StaleAt   time.Time       `json:"stale_at"`
}

// NewCache creates a cache instance with disk persistence.
func NewCache() *Cache {
	cacheDir := os.Getenv("GT_CACHE_DIR")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".cache", "gastown-web")
	}
	os.MkdirAll(cacheDir, 0755)

	c := &Cache{
		entries:  make(map[string]*cacheEntry),
		cacheDir: cacheDir,
		refresh:  make(map[string]bool),
	}

	// Load existing cache from disk
	c.loadFromDisk()

	return c
}

// loadFromDisk loads all cached entries from disk into memory.
func (c *Cache) loadFromDisk() {
	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		key := f.Name()[:len(f.Name())-5] // remove .json

		data, err := os.ReadFile(filepath.Join(c.cacheDir, f.Name()))
		if err != nil {
			continue
		}

		var entry diskEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}

		// Parse the value back to interface{}
		var value interface{}
		if err := json.Unmarshal(entry.Value, &value); err != nil {
			continue
		}

		c.entries[key] = &cacheEntry{
			Value:     value,
			ExpiresAt: entry.ExpiresAt,
			StaleAt:   entry.StaleAt,
		}
	}
}

// saveToDisk persists a cache entry to disk.
func (c *Cache) saveToDisk(key string, entry *cacheEntry) {
	valueJSON, err := json.Marshal(entry.Value)
	if err != nil {
		return
	}

	diskData := diskEntry{
		Value:     valueJSON,
		ExpiresAt: entry.ExpiresAt,
		StaleAt:   entry.StaleAt,
	}

	data, err := json.Marshal(diskData)
	if err != nil {
		return
	}

	// Sanitize key for filename
	safeKey := sanitizeKey(key)
	os.WriteFile(filepath.Join(c.cacheDir, safeKey+".json"), data, 0644)
}

// deleteFromDisk removes a cached entry from disk.
func (c *Cache) deleteFromDisk(key string) {
	os.Remove(filepath.Join(c.cacheDir, sanitizeKey(key)+".json"))
}

// sanitizeKey converts a cache key to a safe filename.
func sanitizeKey(key string) string {
	// Replace unsafe characters
	safe := ""
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			safe += string(r)
		} else {
			safe += "_"
		}
	}
	return safe
}

// Get retrieves a value from cache. Returns value and whether refresh is needed.
// ALWAYS returns cached value if available (even if stale/expired).
func (c *Cache) Get(key string) interface{} {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil
	}

	// Always return the value, even if expired
	return entry.Value
}

// IsStale checks if a cached value needs refresh.
func (c *Cache) IsStale(key string) bool {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return true
	}
	return time.Now().After(entry.StaleAt)
}

// IsRefreshing checks if a key is currently being refreshed.
func (c *Cache) IsRefreshing(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.refresh[key]
}

// SetRefreshing marks a key as being refreshed.
func (c *Cache) SetRefreshing(key string, refreshing bool) {
	c.mu.Lock()
	if refreshing {
		c.refresh[key] = true
	} else {
		delete(c.refresh, key)
	}
	c.mu.Unlock()
}

// Invalidate removes a cache entry immediately.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	delete(c.refresh, key)
	c.mu.Unlock()

	c.deleteFromDisk(key)
}

// InvalidatePrefix removes cache entries with the provided prefix.
func (c *Cache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	var keys []string
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		delete(c.entries, key)
		delete(c.refresh, key)
	}
	c.mu.Unlock()

	for _, key := range keys {
		c.deleteFromDisk(key)
	}
}

// Set stores a value in cache with TTL. StaleAt is set to 80% of TTL.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	now := time.Now()
	entry := &cacheEntry{
		Value:     value,
		ExpiresAt: now.Add(ttl),
		StaleAt:   now.Add(ttl * 80 / 100), // Start refresh at 80% of TTL
	}

	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()

	// Persist to disk asynchronously
	go c.saveToDisk(key, entry)
}

// GetStaleOrRefresh returns cached value immediately, triggers background refresh if stale.
// The refreshFunc is called in a goroutine if data is stale and not already refreshing.
func (c *Cache) GetStaleOrRefresh(key string, ttl time.Duration, refreshFunc func() interface{}) interface{} {
	value := c.Get(key)

	// If stale and not already refreshing, trigger background refresh
	if c.IsStale(key) && !c.IsRefreshing(key) {
		c.SetRefreshing(key, true)
		go func() {
			defer c.SetRefreshing(key, false)
			if newValue := refreshFunc(); newValue != nil {
				c.Set(key, newValue, ttl)
			}
		}()
	}

	return value
}

// StatusCache caches the full status response with stale-while-revalidate.
type StatusCache struct {
	cache *Cache
	ttl   time.Duration
	key   string
}

// NewStatusCache creates a status cache.
func NewStatusCache(ttl time.Duration) *StatusCache {
	return &StatusCache{
		cache: NewCache(),
		ttl:   ttl,
		key:   "status",
	}
}

// GetOrBuild returns cached status immediately, triggers background build if stale.
func (s *StatusCache) GetOrBuild(build func() StatusResponse) StatusResponse {
	// Try to get from cache first
	cached := s.cache.Get(s.key)

	// If we have cached data, check if we need to refresh
	if cached != nil {
		if s.cache.IsStale(s.key) && !s.cache.IsRefreshing(s.key) {
			s.cache.SetRefreshing(s.key, true)
			go func() {
				defer s.cache.SetRefreshing(s.key, false)
				status := build()
				s.cache.Set(s.key, statusToMap(status), s.ttl)
			}()
		}
		// Return cached data (convert from map back to struct)
		if m, ok := cached.(map[string]interface{}); ok {
			return mapToStatus(m)
		}
	}

	// No cache - must build synchronously (first request)
	status := build()
	s.cache.Set(s.key, statusToMap(status), s.ttl)
	return status
}

// statusToMap converts StatusResponse to a map for JSON storage.
func statusToMap(s StatusResponse) map[string]interface{} {
	data, _ := json.Marshal(s)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	return m
}

// mapToStatus converts a map back to StatusResponse.
func mapToStatus(m map[string]interface{}) StatusResponse {
	data, _ := json.Marshal(m)
	var s StatusResponse
	json.Unmarshal(data, &s)
	return s
}

// Cache TTL constants for different data types.
const (
	// StatusCacheTTL for dashboard status.
	StatusCacheTTL = 5 * time.Second

	// ConvoyCacheTTL for convoy data.
	ConvoyCacheTTL = 15 * time.Second

	// AgentCacheTTL for agent data.
	AgentCacheTTL = 10 * time.Second

	// ModelsCacheTTL for model discovery data.
	ModelsCacheTTL = 10 * time.Second

	// IssuesCacheTTL for issue lists.
	IssuesCacheTTL = 20 * time.Second

	// CrewCacheTTL for crew lists and status.
	CrewCacheTTL = 10 * time.Second

	// CICDStatusCacheTTL for CI/CD dashboard snapshot.
	CICDStatusCacheTTL = 5 * time.Second

	// CICDWorkflowsCacheTTL for CI/CD workflow/run lists.
	CICDWorkflowsCacheTTL = 15 * time.Second

	// GitCacheTTL for git data.
	GitCacheTTL = 30 * time.Second

	// SystemCacheTTL for system info.
	SystemCacheTTL = 60 * time.Second

	// ClaudeUsageCacheTTL for Claude usage data (expensive to fetch).
	ClaudeUsageCacheTTL = 120 * time.Second

	// CLIUsageCacheTTL for CLI usage summaries.
	CLIUsageCacheTTL = 120 * time.Second

	// CLILimitsCacheTTL for CLI weekly limit data.
	CLILimitsCacheTTL = 120 * time.Second
)

// Preload triggers background refresh of common cache keys.
func (c *Cache) Preload(refreshFuncs map[string]func() interface{}, ttls map[string]time.Duration) {
	for key, fn := range refreshFuncs {
		ttl := ttls[key]
		if ttl == 0 {
			ttl = 30 * time.Second
		}
		// If no cached data or data is expired, refresh in background
		if c.Get(key) == nil || c.IsStale(key) {
			go func(k string, f func() interface{}, t time.Duration) {
				if val := f(); val != nil {
					c.Set(k, val, t)
				}
			}(key, fn, ttl)
		}
	}
}
