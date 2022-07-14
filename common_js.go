//go:build js

package crystalline

import (
	"errors"
	"reflect"
	"strconv"
	"syscall/js"
)

type converter = func(data js.Value) reflect.Value

func jsToGo(hint reflect.Type) (converter, error) {
	switch hint.Kind() {
	case reflect.Invalid:
		return nil, errors.New("invalid value kind")
	case reflect.Bool:
		return func(data js.Value) reflect.Value {
			newValue := reflect.New(hint).Elem()
			newValue.SetBool(data.Bool())
			return newValue
		}, nil
	case reflect.Int:
		return intToGo(hint), nil
	case reflect.Int8:
		return intToGo(hint), nil
	case reflect.Int16:
		return intToGo(hint), nil
	case reflect.Int32:
		return intToGo(hint), nil
	case reflect.Int64:
		return intToGo(hint), nil
	case reflect.Uint:
		return uintToGo(hint), nil
	case reflect.Uint8:
		return uintToGo(hint), nil
	case reflect.Uint16:
		return uintToGo(hint), nil
	case reflect.Uint32:
		return uintToGo(hint), nil
	case reflect.Uint64:
		return uintToGo(hint), nil
	case reflect.Uintptr:
		return uintToGo(hint), nil
	case reflect.Float32:
		return floatToGo(hint), nil
	case reflect.Float64:
		return floatToGo(hint), nil
	case reflect.Complex64:
		panic("complex64 is not supported as argument type")
	case reflect.Complex128:
		panic("complex128 is not supported as argument type")
	case reflect.Array:
		elementConverter, err := jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return func(data js.Value) reflect.Value {
			outArray := reflect.New(hint).Elem()

			for i := 0; i < data.Length(); i++ {
				outArray.Index(i).Set(elementConverter(data.Index(i)))
			}

			return outArray
		}, nil
	case reflect.Chan:
		panic("channels are not supported as argument types")
	case reflect.Func:
		converters := make([]converter, hint.NumOut())
		for i := 0; i < hint.NumOut(); i++ {
			var err error
			converters[i], err = jsToGo(hint.Out(i))
			if err != nil {
				return nil, err
			}
		}

		isArrayFn := js.Global().Get("Array").Get("isArray")

		return func(data js.Value) reflect.Value {
			return reflect.MakeFunc(hint, func(in []reflect.Value) []reflect.Value {
				inMapped := make([]interface{}, len(in))
				for i, value := range in {
					var err error
					inMapped[i], err = mapInternal(value)
					if err != nil {
						panic(err)
					}
				}

				response := data.Invoke(inMapped...)

				outMapped := make([]reflect.Value, hint.NumOut())
				if isArrayFn.Invoke(response).Bool() {
					for i := 0; i < response.Length(); i++ {
						outMapped[i] = converters[i](response.Index(i))
					}
				} else {
					outMapped[0] = converters[0](response)
				}

				return outMapped
			})
		}, nil
	case reflect.Interface:
		panic("interfaces are not supported as argument types")
	case reflect.Map:
		keyConverter, err := jsToGo(hint.Key())
		if err != nil {
			return nil, err
		}

		elementConverter, err := jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		entriesFunc := js.Global().Get("Object").Get("entries")

		return func(data js.Value) reflect.Value {
			entryValues := entriesFunc.Invoke(data)

			outMap := reflect.MakeMap(hint)
			for i := 0; i < entryValues.Length(); i++ {
				key := entryValues.Index(i).Index(0)
				value := entryValues.Index(i).Index(1)
				cKey := keyConverter(key)
				cVal := elementConverter(value)
				outMap.SetMapIndex(cKey, cVal)
			}

			return outMap
		}, nil
	case reflect.Pointer:
		valueConverter, err := jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return func(data js.Value) reflect.Value {
			if data.IsNull() {
				return reflect.Zero(hint)
			}
			newValue := reflect.New(hint.Elem())
			newValue.Elem().Set(valueConverter(data))
			return newValue
		}, nil
	case reflect.Slice:
		elementConverter, err := jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return func(data js.Value) reflect.Value {
			outSlice := reflect.MakeSlice(hint, data.Length(), data.Length())

			for i := 0; i < data.Length(); i++ {
				outSlice.Index(i).Set(elementConverter(data.Index(i)))
			}

			return outSlice
		}, nil
	case reflect.String:
		return func(data js.Value) reflect.Value {
			if hint.String() != "string" {
				newValue := reflect.New(hint).Elem()
				newValue.SetString(data.String())
				return newValue
			}
			return reflect.ValueOf(data.String())
		}, nil
	case reflect.Struct:
		converters := make(map[string]converter, hint.NumField())
		for i := 0; i < hint.NumField(); i++ {
			field := hint.Field(i)
			var err error
			converters[field.Name], err = jsToGo(field.Type)
			if err != nil {
				return nil, err
			}
		}

		return func(data js.Value) reflect.Value {
			outStruct := reflect.New(hint).Elem()
			for i := 0; i < hint.NumField(); i++ {
				field := hint.Field(i)
				outStruct.Field(i).Set(converters[field.Name](data.Get(field.Name)))
			}
			return outStruct
		}, nil
	case reflect.UnsafePointer:
		panic("unsafe pointers are not supported as argument types")
	}

	return func(_ js.Value) reflect.Value {
		return reflect.ValueOf(nil)
	}, nil
}

func intToGo(hint reflect.Type) converter {
	return func(data js.Value) reflect.Value {
		var value int64

		switch data.Type() {
		case js.TypeString:
			var err error
			value, err = strconv.ParseInt(data.String(), 10, 64)
			if err != nil {
				panic(err)
			}
		case js.TypeNumber:
			value = int64(data.Int())
		default:
			panic("provided value is not an int")
		}

		newValue := reflect.New(hint).Elem()
		newValue.SetInt(value)
		return newValue
	}
}

func uintToGo(hint reflect.Type) converter {
	return func(data js.Value) reflect.Value {
		var value uint64

		switch data.Type() {
		case js.TypeString:
			var err error
			value, err = strconv.ParseUint(data.String(), 10, 64)
			if err != nil {
				panic(err)
			}
		case js.TypeNumber:
			value = uint64(data.Int())
		default:
			panic("provided value is not a uint")
		}

		newValue := reflect.New(hint).Elem()
		newValue.SetUint(value)
		return newValue
	}
}

func floatToGo(hint reflect.Type) converter {
	return func(data js.Value) reflect.Value {
		var value float64

		switch data.Type() {
		case js.TypeString:
			var err error
			value, err = strconv.ParseFloat(data.String(), 64)
			if err != nil {
				panic(err)
			}
		case js.TypeNumber:
			value = data.Float()
		default:
			panic("provided value is not a float")
		}

		newValue := reflect.New(hint).Elem()
		newValue.SetFloat(value)
		return newValue
	}
}
