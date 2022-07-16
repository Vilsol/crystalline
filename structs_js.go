//go:build js

package crystalline

import (
	"reflect"
	"syscall/js"
)

var defineProperties js.Value

var weakCache *WeakCache[js.Value]

func init() {
	defineProperties = js.Global().Get("Object").Get("defineProperties")
	weakCache = NewWeak[js.Value]()
}

func convertStruct(value reflect.Value) (interface{}, error) {
	return weakCache.Fetch(value.UnsafeAddr(), func() (js.Value, error) {
		definitions := make(map[string]interface{})

		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)

			getFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
				return MapOrPanic(field.Interface())
			})

			conv, err := jsToGo(field.Type())
			if err != nil {
				return js.Null(), err
			}

			setFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
				field.Set(conv(args[0]))
				return nil
			})

			fieldName := value.Type().Field(i).Name
			definitions[fieldName] = js.ValueOf(map[string]interface{}{
				"get": getFunc,
				"set": setFunc,
			})
		}

		out := make(map[string]interface{})

		addr := value.Addr()
		for i := 0; i < addr.NumMethod(); i++ {
			val, err := mapInternal(addr.Method(i))
			if err != nil {
				return js.Null(), err
			}
			out[addr.Type().Method(i).Name] = val
		}

		obj := js.ValueOf(out)

		defineProperties.Invoke(obj, definitions)

		return obj, nil
	})
}
