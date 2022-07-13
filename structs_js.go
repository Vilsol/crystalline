//go:build js

package crystalline

import (
	"reflect"
	"syscall/js"
)

var defineProperties js.Value

func init() {
	defineProperties = js.Global().Get("Object").Get("defineProperties")
}

func convertStruct(value reflect.Value) (interface{}, error) {
	out := make(map[string]interface{})
	definitions := make(map[string]interface{})

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		val, err := mapInternal(field)
		if err != nil {
			return nil, err
		}

		fieldName := value.Type().Field(i).Name
		hiddenName := "_" + fieldName
		out[hiddenName] = val

		getFunc := js.FuncOf(func(this js.Value, _ []js.Value) any {
			return this.Get(hiddenName)
		})

		conv, err := jsToGo(field.Type())
		if err != nil {
			return nil, err
		}

		setFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
			this.Set(hiddenName, args[0])
			field.Set(conv(args[0]))
			return nil
		})

		definitions[fieldName] = js.ValueOf(map[string]interface{}{
			"get": getFunc,
			"set": setFunc,
		})
	}

	addr := value.Addr()
	for i := 0; i < addr.NumMethod(); i++ {
		val, err := mapInternal(addr.Method(i))
		if err != nil {
			return nil, err
		}
		out[addr.Type().Method(i).Name] = val
	}

	obj := js.ValueOf(out)

	defineProperties.Invoke(obj, definitions)

	return obj, nil
}
