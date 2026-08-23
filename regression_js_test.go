//go:build js

package crystalline

import (
	"runtime"
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func constructorName(value js.Value) string {
	return value.Get("constructor").Get("name").String()
}

func TestByteSlicesMapToUint8Array(t *testing.T) {
	plain := MapOrPanic([]byte{1, 2, 3}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(plain))
	testza.AssertEqual(t, 3, plain.Get("length").Int())

	// A defined type over []byte must not degrade into an array of numbers.
	named := MapOrPanic(NamedBytes{4, 5, 6}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(named))
	testza.AssertEqual(t, 6, named.Index(2).Int())

	// Neither must a fixed size byte array.
	array := MapOrPanic([4]byte{7, 8, 9, 10}).(js.Value)
	testza.AssertEqual(t, "Uint8Array", constructorName(array))
	testza.AssertEqual(t, 4, array.Get("length").Int())
	testza.AssertEqual(t, 10, array.Index(3).Int())
}

func TestStructWrapperIsCachedPerObject(t *testing.T) {
	obj := &FnSample{FirstValue: "hello"}

	first := MapOrPanic(obj)
	second := MapOrPanic(obj)

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
	before := weakCache.Len()

	func() {
		obj := &FnSample{FirstValue: "temporary"}
		MapOrPanic(obj)
		runtime.KeepAlive(obj)
	}()

	testza.AssertEqual(t, before+1, waitForLen(weakCache, before))
}
