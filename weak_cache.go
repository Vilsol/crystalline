package crystalline

import (
	"runtime"
	"sync"
	"unsafe"
)

type fetch[T any] func() (T, error)

// weakCache caches values against the Go object they were derived from.
//
// An entry is evicted once its owner becomes unreachable, which is also what
// makes the owner address safe to use as a key: while an entry is live its
// owner is live, so the allocator cannot hand that address to another object.
type weakCache[T any] struct {
	mu        sync.Mutex
	reachable map[uintptr]T
}

func newWeak[T any]() *weakCache[T] {
	return &weakCache[T]{
		reachable: make(map[uintptr]T),
	}
}

// Fetch returns the value cached against owner, building it on a miss.
// owner must point at the Go object whose lifetime governs the entry.
func (c *weakCache[T]) Fetch(owner unsafe.Pointer, fetch fetch[T]) (T, error) {
	key := uintptr(owner)

	if found, ok := c.load(key); ok {
		return found, nil
	}

	value, err := fetch()
	if err != nil {
		return value, err
	}

	c.mu.Lock()
	if found, ok := c.reachable[key]; ok {
		c.mu.Unlock()
		return found, nil
	}
	c.reachable[key] = value
	c.mu.Unlock()

	// AddCleanup permits interior pointers, so owner may point at a field
	// within a larger allocation. byte keeps T non zero-sized, which cleanups
	// require in order to run at all.
	runtime.AddCleanup((*byte)(owner), c.unref, key)

	return value, nil
}

func (c *weakCache[T]) load(key uintptr) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	found, ok := c.reachable[key]
	return found, ok
}

// unref runs on a cleanup goroutine, concurrently with Fetch.
func (c *weakCache[T]) unref(key uintptr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reachable, key)
}

func (c *weakCache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reachable)
}
