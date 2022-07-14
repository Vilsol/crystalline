//go:build js

package crystalline

import (
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestStructPointers(t *testing.T) {
	obj := &FnSample{
		FirstValue: "hello",
	}

	js.Global().Set("TestPointers", MapOrPanic(obj))

	js.Global().Get("eval").Invoke("global.TestPointers.FirstValue = 'world'")

	testza.AssertEqual(t, "world", obj.FirstValue)
}

func TestStructNoPointers(t *testing.T) {
	obj := FnSample{
		FirstValue: "hello",
	}

	js.Global().Set("TestPointers", MapOrPanic(obj))

	js.Global().Get("eval").Invoke("global.TestPointers.FirstValue = 'world'")

	testza.AssertEqual(t, "hello", obj.FirstValue)
}
