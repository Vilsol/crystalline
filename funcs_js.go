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

	var converters []converter = nil
	hasPromise := false

	catcher := func(args []reflect.Value) []reflect.Value {
		defer func() {
			if err := recover(); err != nil {
				var stack [8192]byte
				n := runtime.Stack(stack[:], false)
				message := fmt.Sprintf("Panic: %s\n%s", err, stack[:n])
				js.Global().Set("goInternalError", message)
			}
		}()

		return value.Call(args)
	}

	baseFunc := func(_ js.Value, args []js.Value) (result any) {
		if len(args) != valueType.NumIn() {
			panic(fmt.Sprintf("expected %d arguments, got %d", valueType.NumIn(), len(args)))
		}

		mappedIn := make([]reflect.Value, len(args))
		for i, arg := range args {
			if converters[i] != nil {
				mappedIn[i] = converters[i](arg)
			}
		}

		out := catcher(mappedIn)

		if len(out) == 0 {
			return nil
		}

		mappedOut := make([]interface{}, len(out))
		for i, v := range out {
			result, err := mapInternal(v, true, false)
			if err != nil {
				panic(fmt.Errorf("failed internal mapping: %w", err))
			}
			mappedOut[i] = result
		}

		if len(out) == 1 {
			return mappedOut[0]
		}

		return mappedOut
	}

	finalFunc := baseFunc
	promiseFunc := func(this js.Value, args []js.Value) any {
		return promiseConstructor.New(js.FuncOf(func(_ js.Value, promiseArgs []js.Value) any {
			resolve := promiseArgs[0]
			reject := promiseArgs[1]

			go func() {
				defer func() {
					if err := recover(); err != nil {
						var stack [8192]byte
						n := runtime.Stack(stack[:], false)
						reject.Invoke(fmt.Sprintf("Panic: %s\n%s", err, stack[:n]))
					}
				}()

				resolve.Invoke(baseFunc(this, args))
			}()

			return nil
		}))
	}

	if promise {
		finalFunc = promiseFunc
	}

	return js.FuncOf(func(this js.Value, args []js.Value) any {
		if converters == nil {
			converters = make([]converter, valueType.NumIn())
			for i := 0; i < valueType.NumIn(); i++ {
				in := valueType.In(i)

				if in.Kind() == reflect.Func {
					hasPromise = true
				}

				var err error
				conv, err := jsToGo(in)
				converters[i] = conv
				if err != nil {
					panic(fmt.Errorf("failed conversion from js to go: %w", err))
				}
			}
		}

		if hasPromise {
			return promiseFunc(this, args)
		}

		return finalFunc(this, args)
	}), nil
}
