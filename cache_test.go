//go:build js

package crystalline

import (
	"runtime"
	"syscall/js"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

func TestWeakGC(t *testing.T) {
	cache := NewWeak[js.Value]()

	something := map[string]interface{}{
		"hello": "world",
	}

	val, _ := cache.Fetch(0, func() (js.Value, error) {
		return js.ValueOf(something), nil
	})

	testza.AssertEqual(t, "world", val.Get("hello").String())
	testza.AssertEqual(t, 1, cache.Len())

	js.Global().Get("console").Get("log").Invoke(val)

	runtime.KeepAlive(val)
	runtime.GC()

	time.Sleep(time.Millisecond * 100)

	testza.AssertEqual(t, 0, cache.Len())
}
