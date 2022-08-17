//go:build js

package crystalline

import (
	"errors"
	"reflect"
	"strconv"
	"syscall/js"

	"github.com/rs/zerolog/log"
)

type converter = func(data js.Value) reflect.Value

var jsToGoCache map[reflect.Type]converter

func init() {
	jsToGoCache = make(map[reflect.Type]converter)
}

func jsToGo(hint reflect.Type) (converter, error) {
	if found, ok := jsToGoCache[hint]; ok {
		return found, nil
	}

	switch hint.Kind() {
	case reflect.Invalid:
		return nil, errors.New("invalid value kind")
	case reflect.Bool:
		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}
			newValue := reflect.New(hint).Elem()
			newValue.SetBool(data.Bool())
			return newValue
		}
		return jsToGoCache[hint], nil
	case reflect.Int:
		jsToGoCache[hint] = intToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Int8:
		jsToGoCache[hint] = intToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Int16:
		jsToGoCache[hint] = intToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Int32:
		jsToGoCache[hint] = intToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Int64:
		jsToGoCache[hint] = intToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uint:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uint8:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uint16:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uint32:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uint64:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Uintptr:
		jsToGoCache[hint] = uintToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Float32:
		jsToGoCache[hint] = floatToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Float64:
		jsToGoCache[hint] = floatToGo(hint)
		return jsToGoCache[hint], nil
	case reflect.Complex64:
		log.Error().Msg("complex64 is not supported as argument type. value will not get converted")
		return nil, nil
	case reflect.Complex128:
		log.Error().Msg("complex128 is not supported as argument type. value will not get converted")
		return nil, nil
	case reflect.Array:
		var elementConverter converter

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}

			outArray := reflect.New(hint).Elem()

			if elementConverter != nil {
				for i := 0; i < data.Length(); i++ {
					outArray.Index(i).Set(elementConverter(data.Index(i)))
				}
			}

			return outArray
		}

		var err error
		elementConverter, err = jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return jsToGoCache[hint], nil
	case reflect.Chan:
		log.Error().Msg("channels are not supported as argument types. value will not get converted")
		return nil, nil
	case reflect.Func:
		converters := make([]converter, hint.NumOut())

		isArrayFn := js.Global().Get("Array").Get("isArray")

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}

			return reflect.MakeFunc(hint, func(in []reflect.Value) []reflect.Value {
				inMapped := make([]interface{}, len(in))
				for i, value := range in {
					var err error
					inMapped[i], err = mapInternal(value, false)
					if err != nil {
						panic(err)
					}
				}

				response := data.Invoke(inMapped...)
				realResponse := response

				if response.Type() == js.TypeObject && response.Get("then").Type() == js.TypeFunction {
					res, err := await(response)

					if err != nil {
						panic(err)
					}

					realResponse = res[0]
				}

				outMapped := make([]reflect.Value, hint.NumOut())
				if isArrayFn.Invoke(realResponse).Bool() && hint.NumOut() > 1 {
					for i := 0; i < realResponse.Length(); i++ {
						if converters[i] != nil {
							outMapped[i] = converters[i](realResponse.Index(i))
						}
					}
				} else if hint.NumOut() > 0 {
					if converters[0] != nil {
						outMapped[0] = converters[0](realResponse)
					}
				}

				return outMapped
			})
		}

		for i := 0; i < hint.NumOut(); i++ {
			var err error
			converters[i], err = jsToGo(hint.Out(i))
			if err != nil {
				return nil, err
			}
		}

		return jsToGoCache[hint], nil
	case reflect.Interface:
		log.Error().Str("hint", hint.String()).Msg("interfaces are not supported as argument types. value will not get converted")
		jsToGoCache[hint] = nil
		return nil, nil
	case reflect.Map:
		var keyConverter converter
		var elementConverter converter

		entriesFunc := js.Global().Get("Object").Get("entries")

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}

			outMap := reflect.MakeMap(hint)

			if keyConverter != nil {
				entryValues := entriesFunc.Invoke(data)
				for i := 0; i < entryValues.Length(); i++ {
					key := entryValues.Index(i).Index(0)
					cKey := keyConverter(key)

					var cVal = reflect.ValueOf(nil)
					if elementConverter != nil {
						value := entryValues.Index(i).Index(1)
						cVal = elementConverter(value)
					}

					outMap.SetMapIndex(cKey, cVal)
				}
			}

			return outMap
		}

		var err error
		keyConverter, err = jsToGo(hint.Key())
		if err != nil {
			return nil, err
		}

		elementConverter, err = jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return jsToGoCache[hint], nil
	case reflect.Pointer:
		var valueConverter converter

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}
			newValue := reflect.New(hint.Elem())
			if valueConverter != nil {
				newValue.Elem().Set(valueConverter(data))
			}
			return newValue
		}

		var err error
		valueConverter, err = jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return jsToGoCache[hint], nil
	case reflect.Slice:
		if hint.String() == "[]uint8" {
			return func(data js.Value) reflect.Value {
				outSlice := reflect.MakeSlice(hint, data.Length(), data.Length())
				js.CopyBytesToGo(outSlice.Interface().([]uint8), data)
				return outSlice
			}, nil
		}

		var elementConverter converter

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}

			length := 0
			if !data.IsNull() && !data.IsUndefined() && !data.IsNaN() {
				length = data.Length()
			}

			outSlice := reflect.MakeSlice(hint, length, length)

			if elementConverter != nil {
				for i := 0; i < length; i++ {
					outSlice.Index(i).Set(elementConverter(data.Index(i)))
				}
			}

			return outSlice
		}

		var err error
		elementConverter, err = jsToGo(hint.Elem())
		if err != nil {
			return nil, err
		}

		return jsToGoCache[hint], nil
	case reflect.String:
		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}
			if hint.String() != "string" {
				newValue := reflect.New(hint).Elem()
				newValue.SetString(data.String())
				return newValue
			}
			return reflect.ValueOf(data.String())
		}

		return jsToGoCache[hint], nil
	case reflect.Struct:
		converters := make(map[string]converter, hint.NumField())

		jsToGoCache[hint] = func(data js.Value) reflect.Value {
			if data.IsUndefined() || data.IsNull() {
				return reflect.Zero(hint)
			}

			outStruct := reflect.New(hint).Elem()
			for i := 0; i < hint.NumField(); i++ {
				field := hint.Field(i)
				if converters[field.Name] != nil {
					outStruct.Field(i).Set(converters[field.Name](data.Get(field.Name)))
				}
			}
			return outStruct
		}

		for i := 0; i < hint.NumField(); i++ {
			field := hint.Field(i)
			var err error
			converters[field.Name], err = jsToGo(field.Type)
			if err != nil {
				return nil, err
			}
		}

		return jsToGoCache[hint], nil
	case reflect.UnsafePointer:
		log.Error().Msg("unsafe pointers are not supported as argument types. value will not get converted")
		return nil, nil
	}

	return func(_ js.Value) reflect.Value {
		return reflect.ValueOf(nil)
	}, nil
}

func intToGo(hint reflect.Type) converter {
	return func(data js.Value) reflect.Value {
		if data.IsUndefined() || data.IsNull() {
			return reflect.Zero(hint)
		}

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
		if data.IsUndefined() || data.IsNull() {
			return reflect.Zero(hint)
		}

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
		if data.IsUndefined() || data.IsNull() {
			return reflect.Zero(hint)
		}

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

func await(awaitable js.Value) ([]js.Value, []js.Value) {
	then := make(chan []js.Value)
	defer close(then)
	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		then <- args
		return nil
	})
	defer thenFunc.Release()

	catch := make(chan []js.Value)
	defer close(catch)
	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		catch <- args
		return nil
	})
	defer catchFunc.Release()

	awaitable.Call("then", thenFunc).Call("catch", catchFunc)

	select {
	case result := <-then:
		return result, nil
	case err := <-catch:
		return nil, err
	}
}
