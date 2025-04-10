package crystalline

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var (
	promisified = make(map[string]map[string]bool)
	ignored     = make(map[string]map[string]bool)
)

func MarkIgnored(entity string, fn string) {
	if _, ok := ignored[entity]; !ok {
		ignored[entity] = make(map[string]bool)
	}

	ignored[entity][fn] = true
}

func MarkPromise(entity string, fn string) {
	if _, ok := promisified[entity]; !ok {
		promisified[entity] = make(map[string]bool)
	}

	promisified[entity][fn] = true
}

func MapOrPanic(data interface{}) interface{} {
	return MapOrPanicPromise(data, false)
}

func MapOrPanicPromise(data interface{}, promise bool) interface{} {
	result, err := mapInternal(reflect.ValueOf(data), promise, false)
	if err != nil {
		panic(fmt.Errorf("failed internal mapping: %w", err))
	}
	return result
}

func Map(data interface{}) (interface{}, error) {
	return MapPromise(data, false)
}

func MapPromise(data interface{}, promise bool) (interface{}, error) {
	return mapInternal(reflect.ValueOf(data), promise, false)
}

func mapInternal(value reflect.Value, promise bool, nonNil bool) (interface{}, error) {
	switch value.Kind() {
	case reflect.Invalid:
		return nil, errors.New("invalid value kind")
	case reflect.Chan:
		return nil, errors.New("channels cannot be converted to wasm")
	case reflect.Complex64:
		return nil, errors.New("complex64 cannot be converted to wasm")
	case reflect.Complex128:
		return nil, errors.New("complex128 cannot be converted to wasm")
	case reflect.Slice:
		if value.IsNil() {
			if nonNil {
				return make([]interface{}, 0), nil
			}
			return nil, nil
		}
		fallthrough
	case reflect.Array:
		if value.Type().String() == "[]uint8" {
			return convertByteArray(value.Interface().([]uint8))
		}

		out := make([]interface{}, value.Len())
		for i := 0; i < value.Len(); i++ {
			val, err := mapInternal(value.Index(i), false, false)
			if err != nil {
				return nil, err
			}
			out[i] = val
		}
		return out, nil
	case reflect.Func:
		if value.IsNil() {
			return nil, nil
		}
		return convertFunc(value, promise)
	case reflect.Pointer:
		fallthrough
	case reflect.Interface:
		if value.IsNil() {
			return nil, nil
		}

		if err, ok := value.Interface().(error); ok {
			return convertError(err)
		}

		return mapInternal(value.Elem(), false, false)
	case reflect.Map:
		if value.IsNil() {
			if nonNil {
				return make(map[string]interface{}), nil
			}
			return nil, nil
		}

		out := make(map[string]interface{})
		i := value.MapRange()
		for i.Next() {
			key, err := mapInternal(i.Key(), false, false)
			if err != nil {
				return nil, err
			}
			val, err := mapInternal(i.Value(), false, false)
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(key)] = val
		}
		return out, nil
	case reflect.Struct:
		if value.CanAddr() {
			return convertStruct(value)
		}

		out := make(map[string]interface{})
		for i := 0; i < value.NumField(); i++ {
			structField := value.Type().Field(i)
			if structField.PkgPath != "" {
				continue
			}

			notNil := strings.Contains(structField.Tag.Get("crystalline"), "not_nil")

			val, err := mapInternal(value.Field(i), false, notNil)
			if err != nil {
				return nil, err
			}
			out[structField.Name] = val
		}

		for i := 0; i < value.NumMethod(); i++ {
			method := value.Type().Method(i)
			if method.PkgPath != "" {
				continue
			}

			methodPromise := false
			fn := findFunction(method.Func.Pointer())
			if fn != nil && fn.Doc != nil {
				for _, comment := range fn.Doc.List {
					if comment != nil {
						if strings.Contains(comment.Text, "crystalline:promise") {
							methodPromise = true
						}
					}
				}
			}

			val, err := mapInternal(value.Method(i), methodPromise, false)
			if err != nil {
				return nil, err
			}
			out[method.Name] = val
		}

		return out, nil
	case reflect.Bool:
		return value.Bool(), nil
	case reflect.Int:
		return value.Int(), nil
	case reflect.Int8:
		return value.Int(), nil
	case reflect.Int16:
		return value.Int(), nil
	case reflect.Int32:
		return value.Int(), nil
	case reflect.Int64:
		return value.Int(), nil
	case reflect.Uint:
		return value.Uint(), nil
	case reflect.Uint8:
		return value.Uint(), nil
	case reflect.Uint16:
		return value.Uint(), nil
	case reflect.Uint32:
		return value.Uint(), nil
	case reflect.Uint64:
		return value.Uint(), nil
	case reflect.Uintptr:
		return value.Uint(), nil
	case reflect.Float32:
		return value.Float(), nil
	case reflect.Float64:
		return value.Float(), nil
	case reflect.String:
		return value.String(), nil
	case reflect.UnsafePointer:
		if value.IsNil() {
			return nil, nil
		}

		return value.Pointer(), nil
	}

	panic("unknown reflect type")
}
