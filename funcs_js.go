//go:build js

package crystalline

import (
	"fmt"
	"reflect"
	"runtime"
	"syscall/js"
)

var promiseConstructor js.Value

func init() {
	promiseConstructor = js.Global().Get("Promise")
}

func convertFunc(value reflect.Value, promise bool) (interface{}, error) {
	valueType := value.Type()

	// Build the argument converters up front so an unsupported signature is
	// reported to whoever exposes the function, not to whoever first calls it.
	converters := make([]converter, valueType.NumIn())
	hasPromise := false

	for i := 0; i < valueType.NumIn(); i++ {
		in := valueType.In(i)

		if in.Kind() == reflect.Func {
			hasPromise = true
		}

		conv, err := jsToGo(in)
		if err != nil {
			return nil, fmt.Errorf("argument %d (%s): %w", i+1, in, err)
		}

		converters[i] = conv
	}

	// catcher returns the panic instead of reporting it, so the caller decides
	// how to surface it. A promise must reject; a synchronous call has to go
	// through the global slot that the generated wrap() helper reads.
	catcher := func(args []reflect.Value) (out []reflect.Value, panicErr error) {
		defer func() {
			if err := recover(); err != nil {
				var stack [8192]byte
				n := runtime.Stack(stack[:], false)
				panicErr = fmt.Errorf("Panic: %s\n%s", err, stack[:n])
			}
		}()

		return value.Call(args), nil
	}

	baseFunc := func(_ js.Value, args []js.Value) (any, error) {
		if len(args) != valueType.NumIn() {
			return nil, fmt.Errorf("expected %d arguments, got %d", valueType.NumIn(), len(args))
		}

		mappedIn := make([]reflect.Value, len(args))
		for i, arg := range args {
			if converters[i] != nil {
				mappedIn[i] = converters[i](arg)
			}
		}

		out, err := catcher(mappedIn)
		if err != nil {
			return nil, err
		}

		if len(out) == 0 {
			return nil, nil
		}

		mappedOut := make([]interface{}, len(out))
		for i, v := range out {
			result, err := mapInternal(v, true, false)
			if err != nil {
				return nil, fmt.Errorf("failed internal mapping: %w", err)
			}
			mappedOut[i] = result
		}

		if len(out) == 1 {
			return mappedOut[0], nil
		}

		return mappedOut, nil
	}

	syncFunc := func(this js.Value, args []js.Value) any {
		result, err := baseFunc(this, args)
		if err != nil {
			js.Global().Set("goInternalError", err.Error())
			return nil
		}

		return result
	}

	finalFunc := syncFunc
	promiseFunc := func(this js.Value, args []js.Value) any {
		return promiseConstructor.New(js.FuncOf(func(_ js.Value, promiseArgs []js.Value) any {
			resolve := promiseArgs[0]
			reject := promiseArgs[1]

			go func() {
				defer func() {
					if err := recover(); err != nil {
						var stack [8192]byte
						n := runtime.Stack(stack[:], false)
						reject.Invoke(errorConstructor.New(fmt.Sprintf("Panic: %s\n%s", err, stack[:n])))
					}
				}()

				result, err := baseFunc(this, args)
				if err != nil {
					// Reject with a real Error so that catch blocks see the
					// same shape they get from the synchronous path.
					reject.Invoke(errorConstructor.New(err.Error()))
					return
				}

				resolve.Invoke(result)
			}()

			return nil
		}))
	}

	if promise {
		finalFunc = promiseFunc
	}

	// A callback argument cannot be serviced synchronously: the Go side has to
	// yield to the JS event loop for it, so the whole call becomes a promise.
	if hasPromise {
		finalFunc = promiseFunc
	}

	return js.FuncOf(finalFunc), nil
}
