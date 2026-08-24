//go:build !js

package crystalline

import (
	"go/types"
)

// The fixed support code every generated file carries.
//
// It is duplicated per generated package rather than imported, so that generated
// bindings never depend on anything of crystalline at run time.
const prelude = `// crystallineNamespace resolves globalThis.go.<app>.<pkg>, creating each level.
func crystallineNamespace(app string, pkg string) js.Value {
	global := js.Global()

	root := global.Get("go")
	if root.IsUndefined() {
		global.Set("go", map[string]any{})
		root = global.Get("go")
	}

	appNamespace := root.Get(app)
	if appNamespace.IsUndefined() {
		root.Set(app, map[string]any{})
		appNamespace = root.Get(app)
	}

	packageNamespace := appNamespace.Get(pkg)
	if packageNamespace.IsUndefined() {
		appNamespace.Set(pkg, map[string]any{})
		packageNamespace = appNamespace.Get(pkg)
	}

	return packageNamespace
}

// crystallineFail reports a failure through the slot the generated wrap()
// helper reads, matching the reflect runtime's contract.
func crystallineFail(message string) any {
	js.Global().Set("goInternalError", message)

	return nil
}

func crystallineBytes(data []byte) any {
	if data == nil {
		return nil
	}

	out := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(out, data)

	return out
}

// crystallinePromise runs body on its own goroutine and hands JS a Promise, so
// a long Go call does not block the single JS thread. A panic rejects rather
// than escaping into the wasm bridge.
func crystallinePromise(body func() any) any {
	executor := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve := args[0]
		reject := args[1]

		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					reject.Invoke(js.Global().Get("Error").New(crystallineRecovered(recovered)))
				}
			}()

			value := body()

			// A failure inside the body travels through the same slot a
			// synchronous call uses, so generated code has one way to report
			// one and needs no panic to do it. The JS wrapper cannot read the
			// slot here: it handed back the promise before the body ran.
			if failure := js.Global().Get("goInternalError"); !failure.IsUndefined() {
				js.Global().Set("goInternalError", js.Undefined())
				reject.Invoke(js.Global().Get("Error").New(failure.String()))

				return
			}

			resolve.Invoke(value)
		}()

		return nil
	})

	promise := js.Global().Get("Promise").New(executor)

	// The Promise constructor calls the executor synchronously and never again,
	// so its slot in the Go/JS bridge goes back now. Held, it leaked one slot
	// per promise-returning call for the life of the page.
	executor.Release()

	return promise
}

// crystallineRecovered renders a recovered value without reaching for fmt,
// which would drag reflect back into the binary.
func crystallineRecovered(recovered any) string {
	switch typed := recovered.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	}

	return "panic"
}

// crystallineWrapper turns the error slot into a thrown JS Error. Go cannot
// raise a JS exception from inside a callback, so the throw has to happen on
// the JS side.
var crystallineWrapper js.Value

func crystallineWrap(fn js.Func) js.Value {
	if crystallineWrapper.IsUndefined() {
		crystallineWrapper = js.Global().Call("eval", ` + "`" + `(fn) => (...args) => {
			const result = fn(...args);
			if (globalThis.goInternalError) {
				const error = new Error(globalThis.goInternalError);
				globalThis.goInternalError = undefined;
				throw error;
			}
			return result;
		}` + "`" + `)
	}

	return crystallineWrapper.Invoke(fn)
}

// crystallineIterator turns a Go channel into a JS async iterable, which is the
// same idea spelled the other language's way.
//
// next is called once per iteration and reports the value plus whether the
// channel is still open. stop tears down whatever governs the stream, and runs
// once the stream ends however it ends.
func crystallineIterator(next func() (any, bool), stop func()) any {
	var once sync.Once

	finish := func() {
		once.Do(stop)
	}

	iterator := js.Global().Get("Object").New()

	iterator.Set("next", js.FuncOf(func(this js.Value, args []js.Value) any {
		return crystallinePromise(func() any {
			result := js.Global().Get("Object").New()

			value, ok := next()
			if !ok {
				finish()

				result.Set("done", true)
				result.Set("value", nil)

				return result
			}

			result.Set("done", false)
			result.Set("value", value)

			return result
		})
	}))

	// A consumer that stops early -- breaking out of a for await -- asks the
	// iterator to return. That is the only notice Go gets that nobody is
	// reading any more, so it is where an abandoned stream is torn down.
	iterator.Set("return", js.FuncOf(func(this js.Value, args []js.Value) any {
		finish()

		result := js.Global().Get("Object").New()
		result.Set("done", true)
		result.Set("value", nil)

		return result
	}))

	iterable := js.Global().Get("Object").New()

	// Symbol.asyncIterator cannot be reached through Value.Set, which only
	// takes string keys, so the wiring is done in JS.
	js.Global().Call("eval", ` + "`" + `(iterable, iterator) => {
		iterable[Symbol.asyncIterator] = () => iterator;
		return iterable;
	}` + "`" + `).Invoke(iterable, iterator)

	return iterable
}

// crystallineSource normalises anything iterable into a stepping function, so
// an array, a generator and an async generator all work the same way.
var crystallineSourceOf js.Value

func crystallineSource(source js.Value) js.Value {
	if crystallineSourceOf.IsUndefined() {
		crystallineSourceOf = js.Global().Call("eval", ` + "`" + `(source) => {
			const iterator = source[Symbol.asyncIterator]
				? source[Symbol.asyncIterator]()
				: source[Symbol.iterator]();

			return {
				next: () => Promise.resolve(iterator.next()),
				stop: () => { if (iterator.return) { iterator.return(); } },
			};
		}` + "`" + `)
	}

	return crystallineSourceOf.Invoke(source)
}

// crystallineFeed drains a JS iterable into a Go channel on its own goroutine.
//
// The returned stop function ends the feed and tells the iterator no more will
// be read, so abandoning the channel does not strand the goroutine.
func crystallineFeed[T any](source js.Value, convert func(js.Value) (T, error)) (<-chan T, func() error) {
	out := make(chan T)
	done := make(chan struct{})
	finished := make(chan struct{})

	if source.IsUndefined() || source.IsNull() {
		close(out)

		return out, func() error { return nil }
	}

	iterator := crystallineSource(source)

	// failure is written before finished is closed and read after, so the close
	// orders the two.
	var failure error

	go func() {
		defer close(finished)
		defer close(out)

		for {
			step := crystallineAwait(iterator.Call("next"))
			if step.Get("done").Truthy() {
				return
			}

			value, err := convert(step.Get("value"))
			if err != nil {
				// Ending the stream here would look like a clean end of input,
				// and Go would answer for the values that did arrive.
				failure = err

				return
			}

			select {
			case out <- value:
			case <-done:
				return
			}
		}
	}()

	var once sync.Once

	// Idempotent, and reports whatever ended the feed. The wrapper calls it
	// after the Go function returns and fails the call if the input was bad.
	return out, func() error {
		once.Do(func() {
			close(done)
			iterator.Call("stop")
		})

		<-finished

		return failure
	}
}

// crystallineContext bridges a JS AbortSignal to a Go context, so a call that
// would otherwise hold the single JS thread can be cancelled.
func crystallineContext(signal js.Value) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	if signal.IsUndefined() || signal.IsNull() {
		return ctx, cancel
	}

	if signal.Get("aborted").Truthy() {
		cancel()

		return ctx, cancel
	}

	listener := js.FuncOf(func(this js.Value, args []js.Value) any {
		cancel()

		return nil
	})

	signal.Call("addEventListener", "abort", listener)

	return ctx, func() {
		signal.Call("removeEventListener", "abort", listener)
		listener.Release()
		cancel()
	}
}

// crystallineAwait resolves a thenable, so a Go caller can treat an async JS
// callback like a synchronous one. It blocks, which is safe only because a
// callback forces the enclosing call onto a goroutine.
// crystallineHandleKey names the hidden property carrying a wrapper's handle.
// It is non-enumerable, so Object.keys and JSON.stringify do not see it.
const crystallineHandleKey = "__crystallineHandle"

// Wrappers are backed by a handle table rather than by copying, so a value
// handed back to Go resolves to the object it came from. Entries are released
// when JS collects the wrapper.
// crystallineScope collects the js.Func values a wrapper owns. Each one holds a
// slot in the Go/JS bridge until it is released, so a wrapper that is never
// released leaks one per field and method.
type crystallineScope struct {
	funcs []js.Func
}

func (s *crystallineScope) fn(handler func(js.Value, []js.Value) any) js.Func {
	created := js.FuncOf(handler)
	s.funcs = append(s.funcs, created)

	return created
}

type crystallineEntry struct {
	value any
	scope *crystallineScope
}

var (
	crystallineHandleMutex sync.Mutex
	crystallineHandles     = make(map[int]crystallineEntry)
	crystallineHandleNext  int
	crystallineFinalizer   js.Value
)

func crystallineRetain(value any, scope *crystallineScope) int {
	crystallineHandleMutex.Lock()
	defer crystallineHandleMutex.Unlock()

	crystallineHandleNext++
	crystallineHandles[crystallineHandleNext] = crystallineEntry{value: value, scope: scope}

	return crystallineHandleNext
}

func crystallineResolve(handle int) (any, bool) {
	crystallineHandleMutex.Lock()
	defer crystallineHandleMutex.Unlock()

	entry, ok := crystallineHandles[handle]

	return entry.value, ok
}

// crystallineReleaseHandle drops a wrapper and frees the bridge slots it held.
func crystallineReleaseHandle(handle int) {
	crystallineHandleMutex.Lock()
	entry, ok := crystallineHandles[handle]
	delete(crystallineHandles, handle)
	crystallineHandleMutex.Unlock()

	if !ok || entry.scope == nil {
		return
	}

	for _, released := range entry.scope.funcs {
		released.Release()
	}

	entry.scope.funcs = nil
}

// crystallineAttach tags a wrapper with its handle and arranges for the entry
// to be dropped once JS no longer holds the wrapper.
func crystallineAttach(target js.Value, handle int, scope *crystallineScope) {
	js.Global().Get("Object").Call("defineProperty", target, crystallineHandleKey, map[string]any{
		"value":      handle,
		"enumerable": false,
	})

	// Explicit disposal, so a caller that knows when it is done need not wait
	// for the collector. Symbol.dispose cannot be reached through Value.Set,
	// which only takes string keys.
	disposer := scope.fn(func(this js.Value, args []js.Value) any {
		crystallineReleaseHandle(handle)

		return nil
	})

	target.Set("release", disposer)

	js.Global().Call("eval", ` + "`" + `(target, dispose) => {
		if (typeof Symbol.dispose !== "undefined") {
			target[Symbol.dispose] = dispose;
		}
	}` + "`" + `).Invoke(target, disposer)

	if crystallineFinalizer.IsUndefined() {
		constructor := js.Global().Get("FinalizationRegistry")
		if constructor.IsUndefined() {
			return
		}

		crystallineFinalizer = constructor.New(js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) > 0 {
				crystallineReleaseHandle(args[0].Int())
			}

			return nil
		}))
	}

	crystallineFinalizer.Call("register", target, handle)
}

// crystallineHandleOf recovers the handle a wrapper carries, if any.
func crystallineHandleOf(value js.Value) (int, bool) {
	if value.Type() != js.TypeObject {
		return 0, false
	}

	handle := value.Get(crystallineHandleKey)
	if handle.Type() != js.TypeNumber {
		return 0, false
	}

	return handle.Int(), true
}

// crystallineUnknownProperty reports a property the target type does not have,
// so that a misspelling fails instead of silently leaving a zero value.
func crystallineUnknownProperty(value js.Value, typeName string, known map[string]bool) error {
	keys := js.Global().Get("Object").Call("keys", value)

	for i := 0; i < keys.Length(); i++ {
		name := keys.Index(i).String()
		if !known[name] {
			return errors.New(typeName + ": unknown property " + strconv.Quote(name))
		}
	}

	return nil
}

func crystallineAwait(value js.Value) js.Value {
	if value.Type() != js.TypeObject || value.Get("then").Type() != js.TypeFunction {
		return value
	}

	settled := make(chan js.Value, 1)
	failed := make(chan string, 1)

	onResolved := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			settled <- js.Undefined()
		} else {
			settled <- args[0]
		}

		return nil
	})
	defer onResolved.Release()

	onRejected := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			failed <- "promise rejected"
		} else {
			failed <- args[0].String()
		}

		return nil
	})
	defer onRejected.Release()

	value.Call("then", onResolved, onRejected)

	select {
	case result := <-settled:
		return result
	case message := <-failed:
		panic(message)
	}
}

// crystallineDefine installs accessors so that reads and writes from JS reach
// the Go value, rather than operating on a detached copy.
func crystallineDefine(scope *crystallineScope, target js.Value, name string, get func() any, set func(js.Value)) {
	js.Global().Get("Object").Call("defineProperty", target, name, map[string]any{
		"enumerable": true,
		"get": scope.fn(func(this js.Value, args []js.Value) any {
			return get()
		}),
		// Wrapped, because a write now validates: handing a string to a number
		// field has to throw where the write happened rather than poison the
		// next unrelated call.
		"set": crystallineWrap(scope.fn(func(this js.Value, args []js.Value) (result any) {
			defer crystallineRecover(&result)

			if len(args) > 0 {
				set(args[0])
			}

			return nil
		})),
	})
}

// crystallineRecover turns a panic into a reported failure, so a bad argument
// surfaces as a thrown Error rather than tearing down the wasm runtime.
func crystallineRecover(result *any) {
	if recovered := recover(); recovered != nil {
		*result = crystallineFail(crystallineRecovered(recovered))
	}
}

// crystallineMust unwraps a conversion, turning a failure into a panic that the
// wrapper's recover reports back to JS as a thrown Error.
func crystallineMust[T any](value T, err error) T {
	if err != nil {
		panic(err.Error())
	}

	return value
}

// crystallineOk and crystallineErr build the Result a fallible call returns.
// Failure travels in the value, so a call that can fail need not be async.
// crystallineResultProto carries the Result methods. They live on a prototype
// built once rather than on each result: attaching js.Func values per call
// would allocate a bridge slot for every fallible call and leak it.
var crystallineResultProto js.Value

func crystallineResultPrototype() js.Value {
	if crystallineResultProto.IsUndefined() {
		crystallineResultProto = js.Global().Call("eval", ` + "`" + `({
			unwrap() {
				if (this.ok) {
					return this.value;
				}
				throw this.error;
			},
			unwrapOr(fallback) {
				return this.ok ? this.value : fallback;
			},
		})` + "`" + `)
	}

	return crystallineResultProto
}

func crystallineOk(value any) any {
	out := js.Global().Get("Object").Call("create", crystallineResultPrototype())
	out.Set("ok", true)
	out.Set("value", value)

	return out
}

func crystallineErr(err error) any {
	out := js.Global().Get("Object").Call("create", crystallineResultPrototype())
	out.Set("ok", false)
	out.Set("error", js.Global().Get("Error").New(err.Error()))

	return out
}

func crystallineError(err error) any {
	if err == nil {
		return nil
	}

	return js.Global().Get("Error").New(err.Error())
}

`

// contextPackage is the import path of the only package whose type is given a
// meaning of its own.
const contextPackage = "context"

func isStructType(named *types.Named) bool {
	_, ok := named.Underlying().(*types.Struct)

	return ok
}

// emitSimplePointerConverter handles a pointer to something that is not a
// struct, where there is no handle to recover and null simply means absent.
func (e *emitter) emitSimplePointerConverter(name string, goType string, typed *types.Pointer) (string, error) {
	inner, err := e.ensureValueConverter(typed.Elem())
	if err != nil {
		return "", err
	}

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn nil, nil\n\t}\n\n" +
		"\tconverted, err := " + inner + "(value)\n" +
		"\tif err != nil {\n\t\treturn nil, err\n\t}\n\n" +
		"\treturn &converted, nil\n}\n\n", nil
}
