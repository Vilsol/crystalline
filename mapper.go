package crystalline

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// Markers are registered up front but read during conversion, which happens on
// promise goroutines as well as the main one.
var (
	markerMutex sync.RWMutex
	promisified = make(map[reflect.Type]map[string]bool)
	ignored     = make(map[reflect.Type]map[string]bool)
)

// MarkIgnored keeps a method off the JS surface of entity.
//
// Both the type and the method name are checked, so a rename or a typo is
// reported rather than silently leaving the method exposed.
func MarkIgnored(entity reflect.Type, fn string) error {
	return mark(ignored, entity, fn)
}

// MarkPromise makes a method of entity return a JS Promise, running the Go
// call on its own goroutine so it does not block the JS event loop.
//
// This is the programmatic equivalent of a // crystalline:promise comment on
// the method.
func MarkPromise(entity reflect.Type, fn string) error {
	return mark(promisified, entity, fn)
}

func mark(into map[reflect.Type]map[string]bool, entity reflect.Type, fn string) error {
	if entity == nil {
		return errors.New("cannot mark a method on a nil type")
	}

	if entity.Kind() == reflect.Pointer {
		entity = entity.Elem()
	}

	if entity.Kind() != reflect.Struct {
		return fmt.Errorf("cannot mark methods on %s: only struct types have an exposed method set", entity)
	}

	if !hasMethod(entity, fn) {
		return fmt.Errorf("%s has no exported method %q", entity, fn)
	}

	markerMutex.Lock()
	defer markerMutex.Unlock()

	if _, ok := into[entity]; !ok {
		into[entity] = make(map[string]bool)
	}

	into[entity][fn] = true

	return nil
}

// hasMethod reports whether fn is in the method set of entity or of its
// pointer, which is where pointer-receiver methods live.
func hasMethod(entity reflect.Type, fn string) bool {
	if _, ok := entity.MethodByName(fn); ok {
		return true
	}

	_, ok := reflect.PointerTo(entity).MethodByName(fn)

	return ok
}

func isIgnored(entity reflect.Type, fn string) bool {
	return isMarked(ignored, entity, fn)
}

func isPromise(entity reflect.Type, fn string) bool {
	return isMarked(promisified, entity, fn)
}

func isMarked(in map[reflect.Type]map[string]bool, entity reflect.Type, fn string) bool {
	if entity != nil && entity.Kind() == reflect.Pointer {
		entity = entity.Elem()
	}

	markerMutex.RLock()
	defer markerMutex.RUnlock()

	return in[entity][fn]
}

// Struct tag options recognised on the `crystalline` key.
const (
	tagName   = "crystalline"
	tagNotNil = "not_nil"
)

var knownTagOptions = []string{tagNotNil}

// tagHasOption is the hot-path check, used per field on every conversion.
func tagHasOption(tag string, want string) bool {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		if strings.TrimSpace(option) == want {
			return true
		}
	}

	return false
}

// validateTag rejects options this version does not understand, so that a
// misspelled option fails the build instead of being quietly dropped.
func validateTag(tag string) error {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}

		if !slices.Contains(knownTagOptions, option) {
			return fmt.Errorf("unknown %s tag option %q (known options: %s)", tagName, option, strings.Join(knownTagOptions, ", "))
		}
	}

	return nil
}

// pathError annotates a conversion failure with the path to the value that
// caused it. Segments are prepended as the stack unwinds, so a successful
// conversion never builds a path at all.
type pathError struct {
	path string
	err  error
}

// Error renders the path and the underlying failure.
func (e *pathError) Error() string {
	if e.path == "" {
		return e.err.Error()
	}

	return e.path + ": " + e.err.Error()
}

// Unwrap exposes the underlying failure to errors.Is and errors.As.
func (e *pathError) Unwrap() error {
	return e.err
}

func withPath(segment string, err error) error {
	var existing *pathError
	if errors.As(err, &existing) {
		return &pathError{path: joinPath(segment, existing.path), err: existing.err}
	}

	return &pathError{path: segment, err: err}
}

func joinPath(parent string, child string) string {
	switch {
	case child == "":
		return parent
	case strings.HasPrefix(child, "["):
		return parent + child
	default:
		return parent + "." + child
	}
}

// Map converts a Go value into its JS representation.
//
// Functions become callable JS functions, structs become objects with getters,
// setters and methods, and []byte becomes a Uint8Array. Pass AsPromise to make
// a function return a JS Promise.
//
// Most callers want an Exposer instead; Map is the lower level primitive it is
// built on.
func Map(data any, opts ...Option) (any, error) {
	options := buildOptions(opts)

	return mapValue(data, options.promise)
}

// MustMap is Map, panicking instead of returning an error.
func MustMap(data any, opts ...Option) any {
	result, err := Map(data, opts...)
	if err != nil {
		panic(fmt.Errorf("failed internal mapping: %w", err))
	}

	return result
}

func mapValue(data any, promise bool) (any, error) {
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
		if value.Type().Elem().Kind() == reflect.Uint8 {
			if value.Kind() == reflect.Slice {
				return convertByteArray(value.Bytes())
			}

			// Arrays are not always addressable, so Bytes is unavailable.
			data := make([]byte, value.Len())
			reflect.Copy(reflect.ValueOf(data), value)
			return convertByteArray(data)
		}

		out := make([]interface{}, value.Len())
		for i := 0; i < value.Len(); i++ {
			val, err := mapInternal(value.Index(i), false, false)
			if err != nil {
				return nil, withPath(fmt.Sprintf("[%d]", i), err)
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
				return nil, withPath(fmt.Sprintf("[%v](key)", i.Key()), err)
			}
			val, err := mapInternal(i.Value(), false, false)
			if err != nil {
				return nil, withPath(fmt.Sprintf("[%v]", i.Key()), err)
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

			notNil := tagHasOption(structField.Tag.Get(tagName), tagNotNil)

			val, err := mapInternal(value.Field(i), false, notNil)
			if err != nil {
				return nil, withPath(structField.Name, err)
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
				return nil, withPath(method.Name+"()", err)
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

	return nil, fmt.Errorf("unsupported reflect kind: %s", value.Kind())
}
