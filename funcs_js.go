//go:build js

package crystalline

import (
	"fmt"
	"reflect"
	"syscall/js"
)

func convertFunc(value reflect.Value) (interface{}, error) {
	valueType := value.Type()

	converters := make([]converter, valueType.NumIn())
	for i := 0; i < valueType.NumIn(); i++ {
		var err error
		converters[i], err = jsToGo(valueType.In(i))
		if err != nil {
			return nil, err
		}
	}

	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != valueType.NumIn() {
			panic(fmt.Sprintf("expected %d arguments, got %d", valueType.NumIn(), len(args)))
		}

		mappedIn := make([]reflect.Value, len(args))
		for i, arg := range args {
			mappedIn[i] = converters[i](arg)
		}

		out := value.Call(mappedIn)

		if len(out) == 0 {
			return nil
		}

		mappedOut := make([]interface{}, len(out))
		for i, v := range out {
			result, err := mapInternal(v)
			if err != nil {
				panic(err)
			}
			mappedOut[i] = result
		}

		if len(out) == 1 {
			return mappedOut[0]
		}

		return mappedOut
	}), nil
}
