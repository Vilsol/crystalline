//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
)

// Conversion of Go values into JavaScript, for returns and struct fields.
func (e *emitter) toJS(expr string, t types.Type, nonNil bool) (string, error) {
	switch typed := t.(type) {
	case *types.Basic:
		return basicToJSExpr(expr, typed)
	case *types.Named, *types.Alias:
		return e.namedToJSExpr(expr, t)
	case *types.Pointer:
		// A mapped element is asked about first: it is usually a struct, and
		// reaching for the wrapper marshaller here would emit the very shape
		// the mapping replaces.
		_, mapped := e.marks.marshallerFor(typed.Elem())

		if named, ok := typed.Elem().(*types.Named); ok && !mapped {
			if _, isStruct := named.Underlying().(*types.Struct); isStruct {
				e.queue(named)

				return marshalName(e.imports.goTypeName(named)) + "(" + expr + ")", nil
			}
		}

		// A pointer to anything else is just an optional value.
		// Parenthesised: the element's own conversion may call a method on it,
		// and *r0.UnixMilli() dereferences the result rather than the pointer.
		inner, err := e.toJS("(*"+expr+")", typed.Elem(), false)
		if err != nil {
			return "", fmt.Errorf("pointer to %s is not supported: %w", typed.Elem(), err)
		}

		return "func() any {\n\t\tif " + expr + " == nil {\n\t\t\treturn nil\n\t\t}\n\n\t\treturn " + inner + "\n\t}()", nil
	case *types.Slice:
		return e.sliceToJSExpr(expr, typed.Elem(), nonNil)
	case *types.Array:
		return e.arrayToJSExpr(expr, typed, nonNil)
	case *types.Interface:
		if isErrorType(t) {
			return "crystallineError(" + expr + ")", nil
		}

		return "", fmt.Errorf("interface %s is not supported", t)
	case *types.Map:
		return e.mapToJSExpr(expr, typed, nonNil)
	case *types.Chan:
		if typed.Dir() == types.SendOnly {
			return "", fmt.Errorf("a send-only channel has no JS counterpart")
		}

		inner, err := e.toJS("item", typed.Elem(), false)
		if err != nil {
			return "", err
		}

		// A returned stream outlives the call, so anything scoped to the call
		// is handed to the stream to tear down instead.
		stop := e.streamStop
		if stop == "" {
			stop = "func() {}"
		}

		return "crystallineIterator(func() (any, bool) {\n\t\titem, ok := <-" + expr + "\n\t\tif !ok {\n\t\t\treturn nil, false\n\t\t}\n\n\t\treturn " + inner + ", true\n\t}, " + stop + ")", nil
	}

	return "", fmt.Errorf("un-convertable type: %s", t)
}

func basicToJSExpr(expr string, basic *types.Basic) (string, error) {
	// The same refusal as the declarations: complex has no counterpart.
	if basic.Info()&types.IsComplex != 0 {
		return "", fmt.Errorf("%s cannot be converted to wasm", basic)
	}

	switch basic.Kind() {
	case types.Bool:
		return "bool(" + expr + ")", nil
	case types.String:
		return "string(" + expr + ")", nil
	case types.Complex64, types.Complex128:
		return "", fmt.Errorf("%s cannot be converted to wasm", basic)
	}

	if basic.Info()&types.IsNumeric != 0 {
		return "float64(" + expr + ")", nil
	}

	return "", fmt.Errorf("un-convertable basic type: %s", basic)
}

func (e *emitter) namedToJSExpr(expr string, t types.Type) (string, error) {
	if isErrorType(t) {
		return "crystallineError(" + expr + ")", nil
	}

	if mapped, ok := e.marks.marshallerFor(t); ok {
		// A declared mapping goes out through the function the manifest named,
		// and whatever that returns crosses on its own terms.
		if mapped.declaredByManifest() {
			call := qualified(e.qualifier(mapped.to.Pkg()), mapped.to.Name()) + "(" + expr + ")"

			return e.toJS(call, mapped.intermediate, false)
		}

		return fmt.Sprintf(mapped.toJS, expr), nil
	}

	named, ok := t.(*types.Named)
	if !ok {
		return e.toJS(expr, t.Underlying(), false)
	}

	if _, ok := named.Underlying().(*types.Struct); ok {
		e.queue(named)

		return marshalName(e.imports.goTypeName(named)) + "(&" + expr + ")", nil
	}

	return e.toJS(expr, named.Underlying(), false)
}

func (e *emitter) sliceToJSExpr(expr string, elem types.Type, nonNil bool) (string, error) {
	if basic, ok := elem.Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
		return "crystallineBytes(" + expr + ")", nil
	}

	inner, err := e.toJS("v", elem, false)
	if err != nil {
		return "", err
	}

	empty := "nil"
	if nonNil {
		empty = "[]any{}"
	}

	// Rendered inline so the generated code stays free of generics helpers.
	return "func() any {\n\t\tif " + expr + " == nil {\n\t\t\treturn " + empty + "\n\t\t}\n\n\t\tout := make([]any, 0, len(" + expr + "))\n\t\tfor _, v := range " + expr + " {\n\t\t\tout = append(out, " + inner + ")\n\t\t}\n\n\t\treturn out\n\t}()", nil
}

// arrayToJSExpr renders a fixed size array. Unlike a slice it is never nil, so
// there is nothing to guard.
func (e *emitter) arrayToJSExpr(expr string, typed *types.Array, nonNil bool) (string, error) {
	if basic, ok := typed.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
		return "crystallineBytes(" + expr + "[:])", nil
	}

	inner, err := e.toJS("v", typed.Elem(), false)
	if err != nil {
		return "", err
	}

	return "func() any {\n\t\tout := make([]any, 0, len(" + expr + "))\n\t\tfor _, v := range " + expr + " {\n\t\t\tout = append(out, " + inner + ")\n\t\t}\n\n\t\treturn out\n\t}()", nil
}

func (e *emitter) mapToJSExpr(expr string, typed *types.Map, nonNil bool) (string, error) {
	key, err := e.mapKeyToString("k", typed.Key())
	if err != nil {
		return "", err
	}

	value, err := e.toJS("v", typed.Elem(), false)
	if err != nil {
		return "", err
	}

	empty := "nil"
	if nonNil {
		empty = "map[string]any{}"
	}

	return "func() any {\n\t\tif " + expr + " == nil {\n\t\t\treturn " + empty + "\n\t\t}\n\n\t\tout := make(map[string]any, len(" + expr + "))\n\t\tfor k, v := range " + expr + " {\n\t\t\tout[" + key + "] = " + value + "\n\t\t}\n\n\t\treturn out\n\t}()", nil
}

// mapKeyToString renders a map key as a JS property name without reaching for
// fmt, which would drag reflect back into the binary.
//
// A mapped type runs its mapping first. Reading through to the underlying basic
// type instead meant the same value crossed two ways: a time.Duration was
// milliseconds as a value and nanoseconds as a key, both declared as a number.
func (e *emitter) mapKeyToString(expr string, t types.Type) (string, error) {
	if mapped, ok := e.marks.marshallerFor(t); ok {
		return e.mappedKeyToString(expr, t, mapped)
	}

	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("map keys of type %s are not supported", t)
	}

	switch {
	case basic.Kind() == types.String:
		return "string(" + expr + ")", nil
	case basic.Info()&types.IsUnsigned != 0:
		return "strconv.FormatUint(uint64(" + expr + "), 10)", nil
	case basic.Info()&types.IsInteger != 0:
		return "strconv.FormatInt(int64(" + expr + "), 10)", nil
	case basic.Info()&types.IsFloat != 0:
		return "strconv.FormatFloat(float64(" + expr + "), 'g', -1, 64)", nil
	case basic.Kind() == types.Bool:
		return "strconv.FormatBool(bool(" + expr + "))", nil
	}

	return "", fmt.Errorf("map keys of type %s are not supported", t)
}

// mappedKeyToString renders a mapped type as a property name.
//
// A property name is a string, so the mapping has to produce something that can
// become one. A mapping to a Date has no honest spelling as a key, and is
// refused rather than quietly rendered as whatever the underlying value was.
func (e *emitter) mappedKeyToString(expr string, t types.Type, mapped marshaller) (string, error) {
	crossed, err := e.toJS(expr, t, false)
	if err != nil {
		return "", err
	}

	produced := mapped.intermediate
	if produced == nil {
		// A built-in mapping says what it crosses as rather than naming a type.
		if mapped.declared == tsNumber {
			return "strconv.FormatFloat(float64(" + crossed + "), 'f', -1, 64)", nil
		}

		return "", fmt.Errorf("a %s cannot be a map key: it crosses as a %s", t, mapped.declared)
	}

	basic, ok := produced.Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("a %s cannot be a map key: it crosses as %s", t, produced)
	}

	switch {
	case basic.Kind() == types.String:
		return "string(" + crossed + ")", nil
	case basic.Info()&types.IsNumeric != 0 && basic.Info()&types.IsComplex == 0:
		return "strconv.FormatFloat(float64(" + crossed + "), 'f', -1, 64)", nil
	}

	return "", fmt.Errorf("a %s cannot be a map key: it crosses as %s", t, produced)
}

// fromJS renders a Go expression converting a js.Value into a Go value.
