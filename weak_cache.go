package crystalline

import (
	"runtime"
)

type fetch[T any] func() (T, error)

type WeakCache[T any] struct {
	reachable map[uintptr]T
}

func NewWeak[T any]() *WeakCache[T] {
	return &WeakCache[T]{
		reachable: make(map[uintptr]T),
	}
}

func (c *WeakCache[T]) Fetch(key uintptr, fetch fetch[T]) (T, error) {
	if found, ok := c.reachable[key]; ok {
		return found, nil
	}

	value, err := fetch()
	if err != nil {
		return value, err
	}

	runtime.SetFinalizer(&value, func(_ any) {
		c.unref(key)
	})

	c.reachable[key] = value

	return value, nil
}

func (c *WeakCache[T]) unref(index uintptr) {
	if _, ok := c.reachable[index]; !ok {
		return
	}

	delete(c.reachable, index)
}

func (c *WeakCache[T]) Len() int {
	return len(c.reachable)
}
