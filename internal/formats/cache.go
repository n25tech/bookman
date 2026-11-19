package formats

import (
	"container/list"
	"sync"
)

// PageCache is a byte-bounded LRU of extracted comic pages, so turning pages in
// a CBR does not re-decode the archive each time.
type PageCache struct {
	mu    sync.Mutex
	max   int
	size  int
	ll    *list.List
	items map[string]*list.Element
}

type cacheEntry struct {
	key  string
	data []byte
	ct   string
}

func NewPageCache(maxBytes int) *PageCache {
	return &PageCache{max: maxBytes, ll: list.New(), items: map[string]*list.Element{}}
}

func (c *PageCache) Get(key string) ([]byte, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		ce := e.Value.(*cacheEntry)
		return ce.data, ce.ct, true
	}
	return nil, "", false
}

func (c *PageCache) Put(key string, data []byte, ct string) {
	if c.max <= 0 || len(data) > c.max/4 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		return
	}
	c.items[key] = c.ll.PushFront(&cacheEntry{key, data, ct})
	c.size += len(data)
	for c.size > c.max {
		e := c.ll.Back()
		ce := e.Value.(*cacheEntry)
		c.ll.Remove(e)
		delete(c.items, ce.key)
		c.size -= len(ce.data)
	}
}
