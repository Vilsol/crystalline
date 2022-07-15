package crystalline

import (
	"runtime"
)

type fetch func() (interface{}, error)

type WeakCache struct {
	reachable map[uintptr]interface{}
}

func NewWeak() *WeakCache {
	return &WeakCache{
		reachable: make(map[uintptr]interface{}),
	}
}

func (c *WeakCache) Fetch(key uintptr, fetch fetch) (interface{}, error) {
	if found, ok := c.reachable[key]; ok {
		return found, nil
	}

	value, err := fetch()
	if err != nil {
		return nil, err
	}

	runtime.SetFinalizer(&value, func(_ any) {
		c.unref(key)
	})

	c.reachable[key] = value

	return value, nil
}

func (c *WeakCache) unref(index uintptr) {
	if _, ok := c.reachable[index]; !ok {
		return
	}

	delete(c.reachable, index)
}

func (c *WeakCache) Len() int {
	return len(c.reachable)
}
