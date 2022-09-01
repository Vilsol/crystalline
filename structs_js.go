//go:build js

package crystalline

import (
	"reflect"
	"strings"
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
				if conv != nil {
					field.Set(conv(args[0]))
				}
				return nil
			})

			fieldName := value.Type().Field(i).Name
			definitions[fieldName] = js.ValueOf(map[string]interface{}{
				"get": getFunc,
				"set": setFunc,
			})
		}

		out := make(map[string]interface{})

		promiseFuncs := make(map[string]bool)
		for i := 0; i < value.NumMethod(); i++ {
			fn := findFunction(value.Type().Method(i).Func.Pointer())
			if fn != nil && fn.Doc != nil {
				for _, comment := range fn.Doc.List {
					if comment != nil {
						if strings.Contains(comment.Text, "crystalline:promise") {
							name := value.Type().Method(i).Name
							promiseFuncs[name] = true
						}
					}
				}
			}
		}

		addr := value.Addr()
		for i := 0; i < addr.NumMethod(); i++ {
			name := addr.Type().Method(i).Name
			promise := promiseFuncs[name]

			if !promise {
				fn := findFunction(addr.Type().Method(i).Func.Pointer())
				if fn != nil && fn.Doc != nil {
					for _, comment := range fn.Doc.List {
						if comment != nil {
							if strings.Contains(comment.Text, "crystalline:promise") {
								promise = true
							}
						}
					}
				}
			}

			val, err := mapInternal(addr.Method(i), promise)
			if err != nil {
				return js.Null(), err
			}
			out[name] = val
		}

		obj := js.ValueOf(out)

		defineProperties.Invoke(obj, definitions)

		return obj, nil
	})
}
