package crystalline

import (
	"errors"
	"fmt"
	"reflect"
)

func MapOrPanic(data interface{}) interface{} {
	result, err := mapInternal(reflect.ValueOf(data))
	if err != nil {
		panic(err)
	}
	return result
}

func Map(data interface{}) (interface{}, error) {
	return mapInternal(reflect.ValueOf(data))
}

func mapInternal(value reflect.Value) (interface{}, error) {
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
			return nil, nil
		}
		fallthrough
	case reflect.Array:
		out := make([]interface{}, value.Len())
		for i := 0; i < value.Len(); i++ {
			val, err := mapInternal(value.Index(i))
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
		return convertFunc(value)
	case reflect.Pointer:
		fallthrough
	case reflect.Interface:
		if value.IsNil() {
			return nil, nil
		}

		if err, ok := value.Interface().(error); ok {
			return convertError(err)
		}

		return mapInternal(value.Elem())
	case reflect.Map:
		if value.IsNil() {
			return nil, nil
		}

		out := make(map[string]interface{})
		i := value.MapRange()
		for i.Next() {
			key, err := mapInternal(i.Key())
			if err != nil {
				return nil, err
			}
			val, err := mapInternal(i.Value())
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
			val, err := mapInternal(value.Field(i))
			if err != nil {
				return nil, err
			}
			out[value.Type().Field(i).Name] = val
		}

		for i := 0; i < value.NumMethod(); i++ {
			val, err := mapInternal(value.Method(i))
			if err != nil {
				return nil, err
			}
			out[value.Type().Method(i).Name] = val
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
