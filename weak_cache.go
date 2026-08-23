package crystalline

import (
	"runtime"
	"sync"
	"unsafe"
)

type fetch[T any] func() (T, error)

// WeakCache caches values against the Go object they were derived from.
//
// An entry is evicted once its owner becomes unreachable, which is also what
// makes the owner address safe to use as a key: while an entry is live its
// owner is live, so the allocator cannot hand that address to another object.
type WeakCache[T any] struct {
	mu        sync.Mutex
	reachable map[uintptr]T
}

func NewWeak[T any]() *WeakCache[T] {
	return &WeakCache[T]{
		reachable: make(map[uintptr]T),
	}
}

// Fetch returns the value cached against owner, building it on a miss.
// owner must point at the Go object whose lifetime governs the entry.
func (c *WeakCache[T]) Fetch(owner unsafe.Pointer, fetch fetch[T]) (T, error) {
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

func (c *WeakCache[T]) load(key uintptr) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	found, ok := c.reachable[key]
	return found, ok
}

// unref runs on a cleanup goroutine, concurrently with Fetch.
func (c *WeakCache[T]) unref(key uintptr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reachable, key)
}

func (c *WeakCache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reachable)
}
