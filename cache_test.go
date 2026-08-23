package crystalline

import (
	"errors"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/MarvinJWendt/testza"
)

var errCacheTest = errors.New("cache test failure")

type cacheOwner struct {
	value int
}

// waitForLen polls until the cache reaches want, giving cleanup goroutines a
// chance to run. Cleanups are asynchronous, so a single GC is not enough.
func waitForLen[T any](cache *WeakCache[T], want int) int {
	for i := 0; i < 100; i++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
		if cache.Len() == want {
			return want
		}
	}
	return cache.Len()
}

func TestWeakCacheKeepsLiveEntries(t *testing.T) {
	cache := NewWeak[string]()

	owners := make([]*cacheOwner, 0, 5)
	for i := 0; i < 5; i++ {
		owner := &cacheOwner{value: i}
		owners = append(owners, owner)

		val, err := cache.Fetch(unsafe.Pointer(owner), func() (string, error) {
			return "wrapper", nil
		})
		testza.AssertNoError(t, err)
		testza.AssertEqual(t, "wrapper", val)
	}

	testza.AssertEqual(t, 5, cache.Len())

	// Owners are all still strongly referenced, so nothing may be evicted.
	testza.AssertEqual(t, 5, waitForLen(cache, 0))

	runtime.KeepAlive(owners)
}

func TestWeakCacheHitsWhileOwnerAlive(t *testing.T) {
	cache := NewWeak[string]()
	owner := &cacheOwner{value: 1}

	calls := 0
	build := func() (string, error) {
		calls++
		return "wrapper", nil
	}

	for i := 0; i < 3; i++ {
		val, err := cache.Fetch(unsafe.Pointer(owner), build)
		testza.AssertNoError(t, err)
		testza.AssertEqual(t, "wrapper", val)
	}

	testza.AssertEqual(t, 1, calls)
	runtime.KeepAlive(owner)
}

func TestWeakCacheEvictsDeadOwners(t *testing.T) {
	cache := NewWeak[string]()

	func() {
		owner := &cacheOwner{value: 1}
		_, err := cache.Fetch(unsafe.Pointer(owner), func() (string, error) {
			return "wrapper", nil
		})
		testza.AssertNoError(t, err)
		runtime.KeepAlive(owner)
	}()

	testza.AssertEqual(t, 0, waitForLen(cache, 0))
}

func TestWeakCacheFetchError(t *testing.T) {
	cache := NewWeak[string]()
	owner := &cacheOwner{value: 1}

	_, err := cache.Fetch(unsafe.Pointer(owner), func() (string, error) {
		return "", errCacheTest
	})

	testza.AssertErrorIs(t, err, errCacheTest)
	testza.AssertEqual(t, 0, cache.Len())
	runtime.KeepAlive(owner)
}
