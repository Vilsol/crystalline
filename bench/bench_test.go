//go:build js

package bench

import (
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"testing"
)

// The benchmarks hand JavaScript a loop to run rather than calling each binding
// from Go.
//
// A Go benchmark that invoked a binding directly would pay a Go to JS to Go
// round trip per call, several microseconds of it, which is not what a page
// pays and is large enough to bury what is being measured. Each iteration
// therefore runs a batch of calls inside JavaScript, and the reported ns/op is
// the batch divided back out.
//
// What the numbers do not include is the one extra closure the generated ES
// module wraps around each binding, since a Go test cannot import an ES module.

// api and rawAPI resolve lazily: package-level variables are initialised before
// init runs, which is where both surfaces are published.
func api() js.Value {
	return js.Global().Get("go").Get("bench").Get("payload")
}

func rawAPI() js.Value {
	return js.Global().Get("raw")
}

// perCall runs batches until the harness is satisfied and reports the cost of a
// single call, overriding the per-batch figure the harness would print.
func perCall(b *testing.B, batch int, run func()) {
	b.Helper()

	batches := 0

	for b.Loop() {
		run()

		batches++
	}

	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(batches*batch), "ns/op")
}

// pair runs one benchmark against the generated binding and against the
// hand-written one. Whatever the hand-written line costs is the wasm bridge,
// which no binding layer avoids; the difference is what crystalline adds.
func pair(b *testing.B, name string, run func(b *testing.B, fn js.Value)) {
	b.Helper()

	b.Run("crystalline", func(b *testing.B) { run(b, api().Get(name)) })
	b.Run("raw", func(b *testing.B) { run(b, rawAPI().Get(name)) })
}

const (
	// Enough calls to amortise the single crossing into the driver.
	batch = 500

	// Anything that turns the event loop is far slower, so it runs fewer.
	asyncBatch = 20
)

// BenchmarkCall is the floor: a call that converts nothing.
func BenchmarkCall(b *testing.B) {
	pair(b, "Noop", func(b *testing.B, fn js.Value) {
		perCall(b, batch, func() {
			drivers().call0.Invoke(fn, batch)
		})
	})
}

// BenchmarkInts adds two numbers, so what is measured is two arguments and a
// result rather than the work.
func BenchmarkInts(b *testing.B) {
	pair(b, "AddInts", func(b *testing.B, fn js.Value) {
		perCall(b, batch, func() {
			drivers().call2.Invoke(fn, 1, 2, batch)
		})
	})
}

// BenchmarkString sends a string in and takes the same one back, so each call
// pays for two encodings.
func BenchmarkString(b *testing.B) {
	for _, size := range []int{16, 1024, 65536} {
		text := js.ValueOf(strings.Repeat("x", size))

		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "EchoString", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, text, batch)
				})
			})
		})
	}
}

// BenchmarkBytes is the same trip for a byte slice, which crosses as a
// Uint8Array and so copies in bulk rather than element by element.
func BenchmarkBytes(b *testing.B) {
	for _, size := range []int{16, 1024, 65536} {
		data := js.Global().Get("Uint8Array").New(size)

		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "EchoBytes", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, data, batch)
				})
			})
		})
	}
}

// BenchmarkSliceIn reads an array element by element, which is the only way
// anything but bytes can cross.
func BenchmarkSliceIn(b *testing.B) {
	for _, size := range []int{16, 1024} {
		values := js.Global().Get("Array").New(size)
		for i := range size {
			values.SetIndex(i, float64(i))
		}

		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "SumFloats", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, values, batch)
				})
			})
		})
	}
}

// BenchmarkSliceOut builds an array the other way.
func BenchmarkSliceOut(b *testing.B) {
	for _, size := range []int{16, 1024} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "MakeInts", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, size, batch)
				})
			})
		})
	}
}

// BenchmarkMapIn reads an object into a Go map, one key lookup at a time.
func BenchmarkMapIn(b *testing.B) {
	for _, size := range []int{16, 256} {
		index := js.Global().Get("Object").New()
		for i := range size {
			index.Set(strconv.Itoa(i), i)
		}

		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "CountKeys", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, index, batch)
				})
			})
		})
	}
}

// BenchmarkMapOut builds the object.
func BenchmarkMapOut(b *testing.B) {
	for _, size := range []int{16, 256} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			pair(b, "MakeMap", func(b *testing.B, fn js.Value) {
				perCall(b, batch, func() {
					drivers().call1.Invoke(fn, size, batch)
				})
			})
		})
	}
}

// BenchmarkStructsOut returns a slice of structs, the shape a real payload
// usually has and the most expensive thing to hand over.
//
// Crystalline returns live wrappers: every element gets an accessor pair per
// field and a bound function per method, each holding a slot in the Go/JS
// bridge until the wrapper is released. The hand-written line returns plain
// objects, so the gap is the price of liveness rather than overhead. Both
// release what they produced, which keeps the bridge from growing over the run.
func BenchmarkStructsOut(b *testing.B) {
	const size = 32

	pair(b, "MakePoints", func(b *testing.B, fn js.Value) {
		perCall(b, batch, func() {
			drivers().points.Invoke(fn, size, batch)
		})
	})

	// The same shape declared with r.Plain: converted once, no handles, no
	// accessors, no methods. This is the lever for an aggregate result.
	b.Run("plain", func(b *testing.B) {
		readings := api().Get("MakeReadings")

		perCall(b, batch, func() {
			drivers().points.Invoke(readings, size, batch)
		})
	})
}

// BenchmarkStructWrapper is one wrapper built and released, which is what a
// struct return costs on its own.
func BenchmarkStructWrapper(b *testing.B) {
	newPoint := api().Get("NewPoint")

	b.Run("create+release", func(b *testing.B) {
		perCall(b, batch, func() {
			drivers().released.Invoke(newPoint, "p", batch)
		})
	})
}

// BenchmarkStructField reads and writes a field through the accessors that make
// a wrapper live, against a plain JavaScript object as the floor.
func BenchmarkStructField(b *testing.B) {
	point := api().Get("NewPoint").Invoke("p")
	defer point.Call("release")

	plain := js.Global().Get("Object").New()
	plain.Set("X", 0)

	// A struct-typed field hands back a wrapper. It is cached per parent, so
	// reading it repeatedly costs a read rather than a fresh wrapper.
	b.Run("get/nested", func(b *testing.B) {
		perCall(b, batch, func() {
			drivers().get.Invoke(point, "Origin", batch)
		})
	})

	for _, target := range []struct {
		name  string
		value js.Value
	}{{"wrapper", point}, {"plain", plain}} {
		b.Run("get/"+target.name, func(b *testing.B) {
			perCall(b, batch, func() {
				drivers().get.Invoke(target.value, "X", batch)
			})
		})

		b.Run("set/"+target.name, func(b *testing.B) {
			perCall(b, batch, func() {
				drivers().set.Invoke(target.value, "X", 1.5, batch)
			})
		})
	}
}

// BenchmarkStructMethod calls a method bound onto a wrapper.
func BenchmarkStructMethod(b *testing.B) {
	point := api().Get("NewPoint").Invoke("p")
	defer point.Call("release")

	shift := point.Get("Shift")

	perCall(b, batch, func() {
		drivers().call2.Invoke(shift, 1.0, 1.0, batch)
	})
}

// BenchmarkStructArgument sends a struct the other way. A wrapper resolves
// through the handle table; an object literal is validated field by field.
func BenchmarkStructArgument(b *testing.B) {
	take := api().Get("TakePoint")

	point := api().Get("NewPoint").Invoke("p")
	defer point.Call("release")

	literal := js.Global().Get("Object").New()
	literal.Set("X", 1.0)
	literal.Set("Y", 2.0)
	literal.Set("Label", "p")

	for _, argument := range []struct {
		name  string
		value js.Value
	}{{"wrapper", point}, {"literal", literal}} {
		b.Run(argument.name, func(b *testing.B) {
			perCall(b, batch, func() {
				drivers().call1.Invoke(take, argument.value, batch)
			})
		})
	}
}

// BenchmarkResult measures what carrying a failure in the value costs, on both
// paths, plus the unwrap a caller usually writes.
func BenchmarkResult(b *testing.B) {
	mayFail := api().Get("MayFail")

	b.Run("ok", func(b *testing.B) {
		perCall(b, batch, func() {
			drivers().call1.Invoke(mayFail, true, batch)
		})
	})

	b.Run("ok+unwrap", func(b *testing.B) {
		perCall(b, batch, func() {
			drivers().unwrapped.Invoke(mayFail, true, batch)
		})
	})

	b.Run("err", func(b *testing.B) {
		perCall(b, batch, func() {
			drivers().call1.Invoke(mayFail, false, batch)
		})
	})
}

// BenchmarkPromise measures a call that is asynchronous by request rather than
// by necessity. Both lines include a trip through the event loop, so the
// difference is the goroutine and the promise crystalline adds around it.
func BenchmarkPromise(b *testing.B) {
	settle := newAwaiter()
	defer settle.release()

	pair(b, "Rounds", func(b *testing.B, fn js.Value) {
		perCall(b, asyncBatch, func() {
			settle.await(drivers().awaited.Invoke(fn, 1, asyncBatch))
		})
	})
}

// BenchmarkStream drains a channel through the async iterator. The reported
// figure is per stream of 32 items, each of which costs a promise and a turn of
// the event loop.
func BenchmarkStream(b *testing.B) {
	const size = 32

	settle := newAwaiter()
	defer settle.release()

	stream := api().Get("Stream")

	b.Run("32 items", func(b *testing.B) {
		perCall(b, asyncBatch, func() {
			settle.await(drivers().streamed.Invoke(stream, size, asyncBatch))
		})
	})
}

// BenchmarkFeed sends a channel the other way, filled from a plain array.
func BenchmarkFeed(b *testing.B) {
	const size = 32

	settle := newAwaiter()
	defer settle.release()

	values := js.Global().Get("Array").New(size)
	for i := range size {
		values.SetIndex(i, i)
	}

	drain := api().Get("Drain")

	b.Run("32 items", func(b *testing.B) {
		perCall(b, asyncBatch, func() {
			settle.await(drivers().awaited.Invoke(drain, values, asyncBatch))
		})
	})
}

// driver holds the JavaScript loops the benchmarks hand their work to.
type driver struct {
	call0     js.Value
	call1     js.Value
	call2     js.Value
	get       js.Value
	set       js.Value
	unwrapped js.Value
	released  js.Value
	points    js.Value
	awaited   js.Value
	streamed  js.Value
}

var loadDrivers = sync.OnceValue(func() *driver {
	compile := func(source string) js.Value {
		return js.Global().Call("eval", source)
	}

	return &driver{
		call0:     compile(`(fn, n) => { for (let i = 0; i < n; i++) { fn(); } }`),
		call1:     compile(`(fn, a, n) => { for (let i = 0; i < n; i++) { fn(a); } }`),
		call2:     compile(`(fn, a, b, n) => { for (let i = 0; i < n; i++) { fn(a, b); } }`),
		get:       compile(`(o, key, n) => { let v; for (let i = 0; i < n; i++) { v = o[key]; } return v; }`),
		set:       compile(`(o, key, value, n) => { for (let i = 0; i < n; i++) { o[key] = value; } }`),
		unwrapped: compile(`(fn, a, n) => { for (let i = 0; i < n; i++) { fn(a).unwrap(); } }`),
		released:  compile(`(fn, a, n) => { for (let i = 0; i < n; i++) { fn(a).release?.(); } }`),
		points:    compile(`(fn, count, n) => { for (let i = 0; i < n; i++) { const out = fn(count); for (let j = 0; j < out.length; j++) { out[j].release?.(); } } }`),
		awaited:   compile(`async (fn, a, n) => { for (let i = 0; i < n; i++) { await fn(a); } }`),
		streamed:  compile(`async (fn, count, n) => { for (let i = 0; i < n; i++) { for await (const item of fn(count)) { /* drain */ } } }`),
	}
})

func drivers() *driver {
	return loadDrivers()
}

// awaiter settles a JS promise from Go. Blocking here is what lets the event
// loop run, so the callbacks it waits on can fire.
type awaiter struct {
	settled chan js.Value
	onOK    js.Func
	onError js.Func
}

func newAwaiter() *awaiter {
	a := &awaiter{settled: make(chan js.Value, 1)}

	a.onOK = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			a.settled <- js.Undefined()

			return nil
		}

		a.settled <- args[0]

		return nil
	})

	a.onError = js.FuncOf(func(this js.Value, args []js.Value) any {
		panic("promise rejected: " + args[0].Call("toString").String())
	})

	return a
}

func (a *awaiter) await(promise js.Value) js.Value {
	promise.Call("then", a.onOK, a.onError)

	return <-a.settled
}

func (a *awaiter) release() {
	a.onOK.Release()
	a.onError.Release()
}
