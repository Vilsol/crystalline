//go:build js

package crystalline

import (
	"reflect"
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestJSExposer(t *testing.T) {
	appName := "app"

	e := NewExposer(appName)

	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))

	testza.AssertNoError(t, e.Expose(ExposeArrayTest, "crystalline", "ExposeArrayTest"))
	testza.AssertNoError(t, e.Expose(ExposeSliceTest, "crystalline", "ExposeSliceTest"))
	testza.AssertNoError(t, e.Expose(ExposeStringTest, "crystalline", "ExposeStringTest"))
	testza.AssertNoError(t, e.Expose(ExposeIntTest, "crystalline", "ExposeIntTest"))
	testza.AssertNoError(t, e.Expose(ExposeStructTest, "crystalline", "ExposeStructTest"))
	testza.AssertNoError(t, e.Expose(ExposePointerTest, "crystalline", "ExposePointerTest"))
	testza.AssertNoError(t, e.Expose(ExposeMapTest, "crystalline", "ExposeMapTest"))

	testza.AssertNoError(t, e.AddEntity(nil, "GlobalTest", reflect.TypeOf(GlobalTestObj{})))

	testza.AssertFalse(t, js.Global().Get("go").IsNull())
	testza.AssertFalse(t, js.Global().Get("go").Get(appName).IsNull())
	testza.AssertFalse(t, js.Global().Get("go").Get(appName).Get("crystalline").IsNull())

	obj := js.Global().Get("go").Get(appName).Get("crystalline").Get("ExposeStructTest")
	testza.AssertFalse(t, obj.IsNull())
	testza.AssertEqual(t, "Bob", obj.Get("Name").String())
	testza.AssertEqual(t, js.TypeFunction, obj.Get("NoPointer").Type())

	obj = js.Global().Get("go").Get(appName).Get("crystalline").Get("ExposePointerTest")
	testza.AssertFalse(t, obj.IsNull())
	testza.AssertEqual(t, "Bob", obj.Get("Name").String())
}
