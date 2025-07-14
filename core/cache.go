package core

import (
	"sync"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

// CacheEntry represents a cached metadata entry with expiration
type CacheEntry struct {
	Metadata  *metadata.Metadata
	ExpiresAt time.Time
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// MetadataCache provides a simple in-memory cache for metadata with TTL support
type MetadataCache struct {
	cache    map[string]*CacheEntry
	mu       sync.RWMutex
	ttl      time.Duration
	maxSize  int
	stopChan chan struct{}
}

// NewMetadataCache creates a new metadata cache with the specified TTL and max size
func NewMetadataCache(ttl time.Duration, maxSize int) *MetadataCache {
	cache := &MetadataCache{
		cache:    make(map[string]*CacheEntry),
		ttl:      ttl,
		maxSize:  maxSize,
		stopChan: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go cache.cleanupExpiredEntries()

	return cache
}

// Get retrieves metadata from the cache
func (c *MetadataCache) Get(path string) (*metadata.Metadata, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[path]
	if !exists {
		return nil, false
	}

	if entry.IsExpired() {
		// Entry expired but we'll clean it up asynchronously
		return nil, false
	}

	return entry.Metadata, true
}

// Set stores metadata in the cache
func (c *MetadataCache) Set(path string, md *metadata.Metadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is at max capacity
	if len(c.cache) >= c.maxSize {
		// Simple eviction: remove one expired entry or oldest entry
		c.evictOneEntry()
	}

	c.cache[path] = &CacheEntry{
		Metadata:  md,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes an entry from the cache
func (c *MetadataCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, path)
}

// InvalidatePrefix removes all entries with the given path prefix
func (c *MetadataCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for path := range c.cache {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			delete(c.cache, path)
		}
	}
}

// evictOneEntry removes one entry to make space (caller must hold lock)
func (c *MetadataCache) evictOneEntry() {
	now := time.Now()

	// First try to find an expired entry
	for path, entry := range c.cache {
		if now.After(entry.ExpiresAt) {
			delete(c.cache, path)
			return
		}
	}

	// If no expired entries, remove the first one we find
	// In a production implementation, you might want LRU eviction
	for path := range c.cache {
		delete(c.cache, path)
		return
	}
}

// cleanupExpiredEntries runs periodically to clean up expired cache entries
func (c *MetadataCache) cleanupExpiredEntries() {
	ticker := time.NewTicker(time.Minute) // Clean up every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.stopChan:
			return
		}
	}
}

// performCleanup removes expired entries from the cache
func (c *MetadataCache) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for path, entry := range c.cache {
		if now.After(entry.ExpiresAt) {
			delete(c.cache, path)
		}
	}
}
