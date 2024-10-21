//go:build js

package crystalline

import (
	"crypto/rand"
	"reflect"
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestJSExposer(t *testing.T) {
	appName := "app"

	e := NewExposer(appName)

	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))
	testza.AssertNoError(t, e.ExposeFunc(ErrorFunc))
	testza.AssertNoError(t, e.ExposeFunc(InterfaceFunc))
	testza.AssertNoError(t, e.ExposeFuncPromise(PromiseFunc, true))
	testza.AssertNoError(t, e.ExposeFunc(FuncFunc))
	testza.AssertNoError(t, e.ExposeFunc(ByteFunc))

	testza.AssertNoError(t, e.Expose(ExposeArrayTest, "crystalline", "ExposeArrayTest"))
	testza.AssertNoError(t, e.Expose(ExposeSliceTest, "crystalline", "ExposeSliceTest"))
	testza.AssertNoError(t, e.Expose(ExposeStringTest, "crystalline", "ExposeStringTest"))
	testza.AssertNoError(t, e.Expose(ExposeIntTest, "crystalline", "ExposeIntTest"))
	testza.AssertNoError(t, e.Expose(ExposeStructTest, "crystalline", "ExposeStructTest"))
	testza.AssertNoError(t, e.Expose(ExposePointerTest, "crystalline", "ExposePointerTest"))
	testza.AssertNoError(t, e.Expose(ExposeMapTest, "crystalline", "ExposeMapTest"))
	testza.AssertNoError(t, e.Expose(ExposeInheritedStructTest, "crystalline", "ExposeInheritedStructTest"))

	testza.AssertNoError(t, e.AddEntity(nil, "GlobalTest", reflect.TypeOf(GlobalTestObj{}), false))

	testza.AssertFalse(t, js.Global().Get("go").IsNull())
	testza.AssertFalse(t, js.Global().Get("go").Get(appName).IsNull())
	testza.AssertFalse(t, js.Global().Get("go").Get(appName).Get("crystalline").IsNull())

	obj := js.Global().Get("go").Get(appName).Get("crystalline").Get("ExposeStructTest")
	testza.AssertFalse(t, obj.IsNull())
	testza.AssertEqual(t, "Bob", obj.Get("Name").String())
	testza.AssertEqual(t, js.TypeFunction, obj.Get("NoPointer").Type())
	testza.AssertEqual(t, js.TypeObject, obj.Get("DefinitelyNotNil").Type())
	testza.AssertEqual(t, 0, obj.Get("DefinitelyNotNil").Get("length").Int())
	testza.AssertEqual(t, js.TypeObject, obj.Get("NotNilMap").Type())
	testza.AssertEqual(t, 0, js.Global().Get("Object").Get("keys").Invoke(obj.Get("NotNilMap")).Get("length").Int())

	obj = js.Global().Get("go").Get(appName).Get("crystalline").Get("ExposePointerTest")
	testza.AssertFalse(t, obj.IsNull())
	testza.AssertEqual(t, "Bob", obj.Get("Name").String())

	errResult := js.Global().Get("go").Get(appName).Get("crystalline").Get("ErrorFunc").Invoke()
	testza.AssertEqual(t, js.TypeObject, errResult.Type())
	testza.AssertEqual(t, "sample error", errResult.Get("message").String())

	interfaceResult := js.Global().Get("go").Get(appName).Get("crystalline").Get("InterfaceFunc").Invoke()
	testza.AssertEqual(t, js.TypeString, interfaceResult.Type())
	testza.AssertEqual(t, "test", interfaceResult.String())

	promiseResult := js.Global().Get("go").Get(appName).Get("crystalline").Get("PromiseFunc").Invoke()
	testza.AssertEqual(t, js.TypeObject, promiseResult.Type())

	promiseArgs := testResolvePromise(promiseResult)
	testza.AssertEqual(t, js.TypeNumber, promiseArgs.Type())
	testza.AssertEqual(t, 10, promiseArgs.Int())

	funcResult := js.Global().Get("go").Get(appName).Get("crystalline").Get("FuncFunc").Invoke(js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, args []js.Value) any {
			// args[0] is resolve function
			go args[0].Invoke("Bob")
			return nil
		}))
	}))

	funcArgs := testResolvePromise(funcResult)
	testza.AssertEqual(t, js.TypeString, funcArgs.Type())
	testza.AssertEqual(t, "Hello, Bob", funcArgs.String())

	sampleBytes := make([]byte, 10_000_000) // ~10 MB

	_, _ = rand.Read(sampleBytes)

	byteFuncResult := js.Global().Get("go").Get(appName).Get("crystalline").Get("ByteFunc").Invoke(js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, args []js.Value) any {
			// args[0] is resolve function
			go args[0].Invoke(MapOrPanic(sampleBytes))
			return nil
		}))
	}))

	byteFuncArgs := testResolvePromise(byteFuncResult)
	testza.AssertEqual(t, js.TypeObject, byteFuncArgs.Type())

	outData := make([]byte, byteFuncArgs.Length())
	js.CopyBytesToGo(outData, byteFuncArgs)

	testza.AssertEqual(t, sampleBytes, outData)

	obj = js.Global().Get("go").Get(appName).Get("crystalline").Get("ExposeInheritedStructTest")
	testza.AssertFalse(t, obj.IsNull())
	testza.AssertEqual(t, 7, obj.Get("AnotherObj").Get("SomeValue").Length())
}

func testResolvePromise(promise js.Value) js.Value {
	dataChan := make(chan js.Value)
	promise.Call("then", js.FuncOf(func(_ js.Value, args []js.Value) any {
		dataChan <- args[0]
		return nil
	}))

	out := <-dataChan
	close(dataChan)
	return out
}
