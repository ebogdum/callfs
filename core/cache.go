package core

import (
	"container/list"
	"strings"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	maxNegativeCacheTTL = time.Second
	generationStripes   = 256
)

var (
	cacheHits   = metrics.CacheLookupsTotal.WithLabelValues("hit")
	cacheMisses = metrics.CacheLookupsTotal.WithLabelValues("miss")
)

type CacheEntry struct {
	Metadata  *metadata.Metadata
	ExpiresAt time.Time
}

func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

type cacheItem struct {
	path  string
	entry *CacheEntry
}

type CacheStats struct {
	Enabled        bool
	TTL            time.Duration
	Entries        int
	MaxEntries     int
	Hits           uint64
	Misses         uint64
	Evictions      uint64
	ExpiredCleaned uint64
	LastCleanupAt  time.Time
}

type CacheGeneration struct {
	stripe uint64
	global uint64
}

func generationStripe(path string) int {
	h := uint32(2166136261)
	for i := 0; i < len(path); i++ {
		h ^= uint32(path[i])
		h *= 16777619
	}
	return int(h % generationStripes)
}

type MetadataCache struct {
	entries        map[string]*list.Element
	order          *list.List
	mu             sync.Mutex
	generation     uint64
	stripes        [generationStripes]uint64
	bypass         bool
	ttl            time.Duration
	maxSize        int
	stopChan       chan struct{}
	closeOnce      sync.Once
	hits           uint64
	misses         uint64
	evictions      uint64
	expiredCleaned uint64
	lastCleanupAt  time.Time
}

func NewMetadataCache(ttl time.Duration, maxSize int) *MetadataCache {
	cache := &MetadataCache{
		entries:  make(map[string]*list.Element),
		order:    list.New(),
		ttl:      ttl,
		maxSize:  maxSize,
		stopChan: make(chan struct{}),
	}

	go cache.cleanupExpiredEntries()

	return cache
}

func (c *MetadataCache) SetBypass(bypass bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bypass = bypass
	c.clearLocked()
}

func (c *MetadataCache) Get(path string) (*metadata.Metadata, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass {
		return nil, false
	}

	elem, exists := c.entries[path]
	if !exists {
		c.misses++
		cacheMisses.Inc()
		return nil, false
	}

	item := elem.Value.(*cacheItem)
	if item.entry.IsExpired() {
		c.removeElement(elem)
		c.misses++
		cacheMisses.Inc()
		return nil, false
	}
	if nil == item.entry.Metadata {
		c.misses++
		cacheMisses.Inc()
		return nil, false
	}

	c.hits++
	cacheHits.Inc()
	c.order.MoveToFront(elem)
	return cloneMetadata(item.entry.Metadata), true
}

func (c *MetadataCache) IsAbsent(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass {
		return false
	}

	elem, exists := c.entries[path]
	if !exists {
		return false
	}

	entry := elem.Value.(*cacheItem).entry
	if entry.IsExpired() {
		c.removeElement(elem)
		return false
	}
	return nil == entry.Metadata
}

func (c *MetadataCache) Generation(path string) CacheGeneration {
	c.mu.Lock()
	defer c.mu.Unlock()

	return CacheGeneration{stripe: c.stripes[generationStripe(path)], global: c.generation}
}

func (c *MetadataCache) SetIfGeneration(path string, md *metadata.Metadata, gen CacheGeneration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass || 0 >= c.ttl || c.generation != gen.global || c.stripes[generationStripe(path)] != gen.stripe {
		return
	}
	c.putLocked(path, &CacheEntry{
		Metadata:  cloneMetadata(md),
		ExpiresAt: time.Now().Add(c.ttl),
	})
}

func (c *MetadataCache) SetAbsent(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass || 0 >= c.ttl {
		return
	}
	c.putLocked(path, &CacheEntry{
		ExpiresAt: time.Now().Add(min(c.ttl, maxNegativeCacheTTL)),
	})
}

func (c *MetadataCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass {
		return
	}
	c.stripes[generationStripe(path)]++
	if elem, exists := c.entries[path]; exists {
		c.removeElement(elem)
	}
}

func (c *MetadataCache) InvalidatePrefix(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bypass {
		return 0
	}
	c.generation++
	dir := strings.TrimSuffix(prefix, "/")
	removed := 0
	for elem := c.order.Front(); elem != nil; {
		next := elem.Next()
		path := elem.Value.(*cacheItem).path
		if path == prefix || strings.HasPrefix(path, dir+"/") {
			c.removeElement(elem)
			removed++
		}
		elem = next
	}
	return removed
}

func (c *MetadataCache) Clear() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.clearLocked()
}

func (c *MetadataCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return CacheStats{
		Enabled:        0 < c.ttl,
		TTL:            c.ttl,
		Entries:        c.order.Len(),
		MaxEntries:     c.maxSize,
		Hits:           c.hits,
		Misses:         c.misses,
		Evictions:      c.evictions,
		ExpiredCleaned: c.expiredCleaned,
		LastCleanupAt:  c.lastCleanupAt,
	}
}

func (c *MetadataCache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = ttl
	c.clearLocked()
}

func (c *MetadataCache) SetMaxSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxSize = size
	c.clearLocked()
}

func (c *MetadataCache) Close() {
	c.closeOnce.Do(func() { close(c.stopChan) })
}

func (c *MetadataCache) clearLocked() int {
	c.generation++
	removed := c.order.Len()
	c.entries = make(map[string]*list.Element)
	c.order.Init()
	metrics.CacheEntries.Set(0)
	return removed
}

func (c *MetadataCache) putLocked(path string, entry *CacheEntry) {
	if elem, exists := c.entries[path]; exists {
		elem.Value.(*cacheItem).entry = entry
		c.order.MoveToFront(elem)
		return
	}

	for c.order.Len() >= c.maxSize && 0 < c.order.Len() {
		c.removeElement(c.order.Back())
		c.evictions++
		metrics.CacheEvictionsTotal.Inc()
	}
	c.entries[path] = c.order.PushFront(&cacheItem{path: path, entry: entry})
	metrics.CacheEntries.Set(float64(c.order.Len()))
}

func (c *MetadataCache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	delete(c.entries, elem.Value.(*cacheItem).path)
	metrics.CacheEntries.Set(float64(c.order.Len()))
}

func (c *MetadataCache) cleanupExpiredEntries() {
	ticker := time.NewTicker(time.Minute)
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

func (c *MetadataCache) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for elem := c.order.Front(); elem != nil; {
		next := elem.Next()
		if elem.Value.(*cacheItem).entry.IsExpired() {
			c.removeElement(elem)
			c.expiredCleaned++
		}
		elem = next
	}
	c.lastCleanupAt = time.Now()
}

func cloneMetadata(md *metadata.Metadata) *metadata.Metadata {
	cp := *md
	if cp.ParentID != nil {
		v := *cp.ParentID
		cp.ParentID = &v
	}
	if cp.CallFSInstanceID != nil {
		v := *cp.CallFSInstanceID
		cp.CallFSInstanceID = &v
	}
	if cp.SymlinkTarget != nil {
		v := *cp.SymlinkTarget
		cp.SymlinkTarget = &v
	}
	return &cp
}
