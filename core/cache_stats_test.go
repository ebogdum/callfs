package core

import (
	"sync"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

func TestCacheStatsCountHitsMissesAndEvictions(t *testing.T) {
	cache := NewMetadataCache(time.Minute, 2)
	defer cache.Close()

	cache.SetIfGeneration("/a", &metadata.Metadata{Path: "/a"}, cache.Generation("/a"))
	cache.SetIfGeneration("/b", &metadata.Metadata{Path: "/b"}, cache.Generation("/b"))
	cache.SetIfGeneration("/c", &metadata.Metadata{Path: "/c"}, cache.Generation("/c"))

	if _, ok := cache.Get("/a"); ok {
		t.Fatal("expected /a to be evicted")
	}
	if _, ok := cache.Get("/c"); !ok {
		t.Fatal("expected /c to be cached")
	}

	stats := cache.Stats()
	if !stats.Enabled || time.Minute != stats.TTL {
		t.Fatalf("got Enabled=%v TTL=%v, want true and 1m", stats.Enabled, stats.TTL)
	}
	if 2 != stats.Entries || 2 != stats.MaxEntries {
		t.Fatalf("got Entries=%d MaxEntries=%d, want 2 and 2", stats.Entries, stats.MaxEntries)
	}
	if 1 != stats.Hits || 1 != stats.Misses || 1 != stats.Evictions {
		t.Fatalf("got Hits=%d Misses=%d Evictions=%d, want 1, 1 and 1", stats.Hits, stats.Misses, stats.Evictions)
	}
}

func TestCacheInvalidatePrefixReturnsRemovedCount(t *testing.T) {
	cache := NewMetadataCache(time.Minute, 10)
	defer cache.Close()

	cache.SetIfGeneration("/dir/a", &metadata.Metadata{Path: "/dir/a"}, cache.Generation("/dir/a"))
	cache.SetIfGeneration("/dir/sub/b", &metadata.Metadata{Path: "/dir/sub/b"}, cache.Generation("/dir/sub/b"))
	cache.SetIfGeneration("/other", &metadata.Metadata{Path: "/other"}, cache.Generation("/other"))

	if removed := cache.InvalidatePrefix("/dir"); 2 != removed {
		t.Fatalf("got %d removed, want 2", removed)
	}
	if _, ok := cache.Get("/other"); !ok {
		t.Fatal("expected /other to survive the prefix invalidation")
	}
}

func TestCacheClearEmptiesAndReportsCount(t *testing.T) {
	cache := NewMetadataCache(time.Minute, 10)
	defer cache.Close()

	cache.SetIfGeneration("/a", &metadata.Metadata{Path: "/a"}, cache.Generation("/a"))
	cache.SetAbsent("/b")

	if removed := cache.Clear(); 2 != removed {
		t.Fatalf("got %d removed, want 2", removed)
	}
	if entries := cache.Stats().Entries; 0 != entries {
		t.Fatalf("got %d entries after Clear, want 0", entries)
	}
}

func TestCacheSetTTLIsSafeUnderConcurrentUse(t *testing.T) {
	engine := &Engine{metadataCache: NewMetadataCache(time.Minute, 100)}
	defer engine.metadataCache.Close()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				engine.metadataCache.SetIfGeneration("/a", &metadata.Metadata{Path: "/a"}, engine.metadataCache.Generation("/a"))
				engine.metadataCache.Get("/a")
			}
		}()
	}
	for range 50 {
		engine.SetMetadataCacheTTL(time.Minute)
		engine.SetMetadataCacheSize(100)
	}
	wg.Wait()
}
