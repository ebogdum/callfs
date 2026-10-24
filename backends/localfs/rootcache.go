package localfs

import (
	"container/list"
	"os"
	"path"
	"strings"
	"sync"
)

const maxCachedSubRoots = 256

type subRootEntry struct {
	dir     string
	root    *os.Root
	refs    int
	evicted bool
	elem    *list.Element
}

type subRootCache struct {
	mu      sync.Mutex
	parent  *os.Root
	entries map[string]*subRootEntry
	order   *list.List
}

func newSubRootCache(parent *os.Root) *subRootCache {
	return &subRootCache{
		parent:  parent,
		entries: make(map[string]*subRootEntry),
		order:   list.New(),
	}
}

func (c *subRootCache) acquire(dir string) (*subRootEntry, error) {
	c.mu.Lock()
	if entry, ok := c.entries[dir]; ok {
		entry.refs++
		c.order.MoveToFront(entry.elem)
		c.mu.Unlock()
		return entry, nil
	}
	c.mu.Unlock()

	root, err := c.parent.OpenRoot(dir)
	if nil != err {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[dir]; ok {
		root.Close()
		existing.refs++
		c.order.MoveToFront(existing.elem)
		return existing, nil
	}
	entry := &subRootEntry{dir: dir, root: root, refs: 1}
	entry.elem = c.order.PushFront(entry)
	c.entries[dir] = entry
	for c.order.Len() > maxCachedSubRoots {
		oldest := c.order.Back().Value.(*subRootEntry)
		c.evictLocked(oldest)
	}
	return entry, nil
}

func (c *subRootCache) release(entry *subRootEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.refs--
	if entry.evicted && 0 == entry.refs {
		entry.root.Close()
	}
}

func (c *subRootCache) evictLocked(entry *subRootEntry) {
	delete(c.entries, entry.dir)
	c.order.Remove(entry.elem)
	entry.evicted = true
	if 0 == entry.refs {
		entry.root.Close()
	}
}

func (c *subRootCache) invalidate(rel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := rel + "/"
	for dir, entry := range c.entries {
		if "." == rel || dir == rel || strings.HasPrefix(dir, prefix) {
			c.evictLocked(entry)
		}
	}
}

func (c *subRootCache) invalidateAll() {
	c.invalidate(".")
}

func (a *LocalFSAdapter) openFile(rel string) (*os.File, error) {
	dir, base := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	if "" == dir || "." == dir {
		return a.root.Open(rel)
	}
	entry, err := a.subRoots.acquire(dir)
	if nil != err {
		return nil, err
	}
	defer a.subRoots.release(entry)
	return entry.root.Open(base)
}
