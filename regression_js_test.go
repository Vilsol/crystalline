//go:build js

package crystalline

import (
	"runtime"
	"strings"
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func constructorName(value js.Value) string {
	return value.Get("constructor").Get("name").String()
}

func TestByteSlicesMapToUint8Array(t *testing.T) {
	plain := MustMap([]byte{1, 2, 3}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(plain))
	testza.AssertEqual(t, 3, plain.Get("length").Int())

	// A defined type over []byte must not degrade into an array of numbers.
	named := MustMap(NamedBytes{4, 5, 6}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(named))
	testza.AssertEqual(t, 6, named.Index(2).Int())

	// Neither must a fixed size byte array.
	array := MustMap([4]byte{7, 8, 9, 10}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(array))
	testza.AssertEqual(t, 4, array.Get("length").Int())
	testza.AssertEqual(t, 10, array.Index(3).Int())
}

func TestStructWrapperIsCachedPerObject(t *testing.T) {
	obj := &FnSample{FirstValue: "hello"}

	first := MustMap(obj)
	second := MustMap(obj)

	testza.AssertTrue(t, first.(js.Value).Equal(second.(js.Value)),
		"the same Go struct must map to the same JS object")

	runtime.KeepAlive(obj)
}

// Known limitation: a wrapper's getters close over the wrapped struct, and the
// js.Func values holding them are never Released, so the struct stays reachable
// from the JS bridge no matter what the caller does. The cache entry therefore
// outlives the caller's reference. Releasing it needs JS-side finalization
// (WeakRef + FinalizationRegistry); a Go-side cleanup can never observe the
// struct becoming unreachable. See TestWeakCacheEvictsDeadOwners for proof that
// the cache itself evicts correctly when nothing pins the owner.
func TestStructWrapperCacheRetainsWrappedObjects(t *testing.T) {
	before := structCache.Len()

	func() {
		obj := &FnSample{FirstValue: "temporary"}
		MustMap(obj)
		runtime.KeepAlive(obj)
	}()

	testza.AssertEqual(t, before+1, waitForLen(structCache, before))
}

// awaitPromise settles promise and reports whether it rejected, plus the
// settled value.
func awaitPromise(promise js.Value) (rejected bool, value js.Value) {
	done := make(chan struct{})

	onOk := js.FuncOf(func(_ js.Value, args []js.Value) any {
		value = args[0]
		close(done)
		return nil
	})
	onErr := js.FuncOf(func(_ js.Value, args []js.Value) any {
		rejected = true
		value = args[0]
		close(done)
		return nil
	})

	promise.Call("then", onOk, onErr)
	<-done

	return rejected, value
}

func TestAsyncPanicRejectsWithError(t *testing.T) {
	js.Global().Set("goInternalError", js.Undefined())

	fn := MustMap(func() string {
		panic("boom")
	}, AsPromise()).(js.Func)

	rejected, value := awaitPromise(fn.Invoke())

	testza.AssertTrue(t, rejected, "a panic must reject the promise, not resolve it")
	testza.AssertEqual(t, "Error", constructorName(value),
		"rejection must carry an Error, not a bare string")
	testza.AssertTrue(t, strings.Contains(value.Get("message").String(), "boom"), value.Get("message").String())

	// The error belongs to this call only. Leaving it on the global would make
	// the next unrelated wrapped call throw it.
	testza.AssertTrue(t, js.Global().Get("goInternalError").IsUndefined(),
		"async errors must not leak into the global error slot")
}

func TestAsyncSuccessDoesNotLeak(t *testing.T) {
	js.Global().Set("goInternalError", js.Undefined())

	fn := MustMap(func() string { return "fine" }, AsPromise()).(js.Func)

	rejected, value := awaitPromise(fn.Invoke())

	testza.AssertFalse(t, rejected)
	testza.AssertEqual(t, "fine", value.String())
	testza.AssertTrue(t, js.Global().Get("goInternalError").IsUndefined())
}

func TestSyncPanicStillSetsGlobalForWrap(t *testing.T) {
	js.Global().Set("goInternalError", js.Undefined())

	fn := MustMap(func() string {
		panic("sync boom")
	}).(js.Func)

	fn.Invoke()

	// The generated wrap() helper reads this slot, so the sync contract stands.
	leaked := js.Global().Get("goInternalError")
	testza.AssertFalse(t, leaked.IsUndefined())
	testza.AssertTrue(t, strings.Contains(leaked.String(), "sync boom"), leaked.String())

	js.Global().Set("goInternalError", js.Undefined())
}

func TestUnsupportedSignatureFailsAtRegistration(t *testing.T) {
	// Discovering this at first invoke means the failure surfaces to an end
	// user rather than to the developer wiring the binding up.
	for _, fn := range []any{
		func(chan bool) {},
		func(complex64) {},
		func(complex128) {},
	} {
		_, err := Map(fn)
		testza.AssertNotNil(t, err, "unsupported parameter must fail when exposed")
	}

	_, err := Map(func(string) {})
	testza.AssertNoError(t, err)
}
