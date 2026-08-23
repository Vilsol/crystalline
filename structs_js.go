//go:build js

package crystalline

import (
	"reflect"
	"strings"
	"syscall/js"
)

var defineProperties js.Value

// weakCache gives each Go struct a stable JS wrapper. Entries are keyed by the
// struct's address, which stays valid because an entry cannot outlive its owner.
//
// Note that in practice entries are never evicted: the getters below capture
// the struct through js.FuncOf, and those funcs are never Released, so the
// struct stays reachable. Dropping them needs JS-side finalization.
var weakCache *WeakCache[js.Value]

func init() {
	defineProperties = js.Global().Get("Object").Get("defineProperties")
	weakCache = NewWeak[js.Value]()
}

func convertStruct(value reflect.Value) (interface{}, error) {
	return weakCache.Fetch(value.Addr().UnsafePointer(), func() (js.Value, error) {
		definitions := make(map[string]interface{})

		for i := 0; i < value.NumField(); i++ {
			structField := value.Type().Field(i)
			if structField.PkgPath != "" {
				continue
			}

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

			fieldName := structField.Name
			definitions[fieldName] = js.ValueOf(map[string]interface{}{
				"get": getFunc,
				"set": setFunc,
			})
		}

		out := make(map[string]interface{})

		promiseFuncs := make(map[string]bool)
		for i := 0; i < value.NumMethod(); i++ {
			method := value.Type().Method(i)
			if method.PkgPath != "" {
				continue
			}

			fn := findFunction(method.Func.Pointer())
			if fn != nil && fn.Doc != nil {
				for _, comment := range fn.Doc.List {
					if comment != nil {
						if strings.Contains(comment.Text, "crystalline:promise") {
							name := method.Name
							promiseFuncs[name] = true
						}
					}
				}
			}
		}

		addr := value.Addr()
		for i := 0; i < addr.NumMethod(); i++ {
			method := addr.Type().Method(i)
			if method.PkgPath != "" {
				continue
			}

			name := method.Name
			promise := promiseFuncs[name]

			if isIgnored(value.Type().String(), name) {
				continue
			}

			if !promise {
				fn := findFunction(method.Func.Pointer())
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

			if !promise {
				promise = isPromise(value.Type().String(), name)
			}

			val, err := mapInternal(addr.Method(i), promise, false)
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
