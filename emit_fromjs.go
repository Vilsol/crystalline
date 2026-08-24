//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
)

// Conversion of JavaScript values into Go, for parameters and for writing struct
// fields back.
//
// This direction is partial in a way the other is not: the input may be missing,
// extra or mistyped, so every converter validates rather than assuming.
func (e *emitter) fromJS(expr string, t types.Type) (string, error) {
	if callback, ok := t.Underlying().(*types.Signature); ok {
		return e.callbackFromJS(expr, t, callback)
	}

	converter, err := e.ensureValueConverter(t)
	if err != nil {
		// The inner error already names the type, so restating it here would
		// only produce a message that says the same thing twice.
		return "", err
	}

	return "crystallineMust(" + converter + "(" + expr + "))", nil
}

// callbackFromJS renders a Go func that calls back into JS, awaiting whatever
// it returns so that an async callback behaves like a synchronous one.
func (e *emitter) callbackFromJS(expr string, t types.Type, sig *types.Signature) (string, error) {
	if sig.Results().Len() > 1 {
		return "", fmt.Errorf("callbacks returning %d values are not supported: JS returns one", sig.Results().Len())
	}

	params := make([]string, 0, sig.Params().Len())
	passed := make([]string, 0, sig.Params().Len())

	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)

		name := "p" + strconv.Itoa(i)

		params = append(params, name+" "+types.TypeString(param.Type(), e.qualifier))

		converted, err := e.toJS(name, param.Type(), false)
		if err != nil {
			return "", fmt.Errorf("callback parameter %d: %w", i+1, err)
		}

		passed = append(passed, converted)
	}

	invocation := "crystallineAwait(" + expr + ".Invoke(" + strings.Join(passed, ", ") + "))"

	var body strings.Builder

	body.WriteString("func(" + strings.Join(params, ", ") + ")")

	if sig.Results().Len() == 0 {
		body.WriteString(" {\n\t\t" + invocation + "\n\t}")

		return body.String(), nil
	}

	result := sig.Results().At(0).Type()

	// Through the same converters a parameter uses. The inline accessors do not
	// validate, and js.Value.String() is the one that does not even panic on
	// the wrong type: a callback returning nothing handed Go the literal
	// "<undefined>" as if it were the answer.
	converted, err := e.fromJS("crystallineCallbackResult", result)
	if err != nil {
		return "", fmt.Errorf("callback result: %w", err)
	}

	body.WriteString(" " + types.TypeString(result, e.qualifier) + " {\n")
	body.WriteString("\t\tcrystallineCallbackResult := " + invocation + "\n\n")
	body.WriteString("\t\treturn " + converted + "\n")
	body.WriteString("\t}")

	return body.String(), nil
}

// fieldSetter renders the write half of a field accessor.
//
// It goes through the same converters a parameter does. An earlier version
// handled only basic types, so a write to a slice, map, struct or pointer field
// was discarded in silence: the Go value kept its old contents and nothing was
// reported.
func (e *emitter) fieldSetter(target string, t types.Type) (string, error) {
	converted, err := e.fromJS("value", t)
	if err != nil {
		return "", err
	}

	return "func(value js.Value) {\n\t\t" + target + " = " + converted + "\n\t}", nil
}

// emitConverter renders the JS to Go conversion for a struct: a wrapper handed
// back resolves to the value it came from, anything else is built field by
// field and validated.
func (e *emitter) emitStructConverter(fn string, named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

	name := instantiatedName(named)
	goType := e.declaredName(named)

	var known []string

	var fields strings.Builder

	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		known = append(known, strconv.Quote(field.Name())+": true")

		check, err := e.fieldFromJS("out."+field.Name(), name+"."+field.Name(), field.Type())
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, field.Name(), err)
		}

		fields.WriteString("\tif property := value.Get(" + strconv.Quote(field.Name()) + "); !property.IsUndefined() && !property.IsNull() {\n")
		fields.WriteString(check)
		fields.WriteString("\t}\n\n")
	}

	var body strings.Builder

	body.WriteString("var crystallineKnown" + name + " = map[string]bool{" + strings.Join(known, ", ") + "}\n\n")

	body.WriteString("func " + fn + "(value js.Value) (" + goType + ", error) {\n")
	body.WriteString("\tvar out " + goType + "\n\n")
	// A pointer position handles its own nil before reaching here, so a null
	// arriving at a struct is a value nobody asked for rather than an absence.
	body.WriteString("\tif value.IsUndefined() || value.IsNull() {\n\t\treturn out, errors.New(" +
		strconv.Quote(name+": expected an object, got null") + ")\n\t}\n\n")
	body.WriteString("\tif handle, ok := crystallineHandleOf(value); ok {\n")
	body.WriteString("\t\tresolved, found := crystallineResolve(handle)\n")
	body.WriteString("\t\tif !found {\n\t\t\treturn out, errors.New(" + strconv.Quote(name+": the value behind this handle has been released") + ")\n\t\t}\n\n")
	body.WriteString("\t\ttyped, ok := resolved.(*" + goType + ")\n")
	body.WriteString("\t\tif !ok {\n\t\t\treturn out, errors.New(" + strconv.Quote(name+": handle refers to a different type") + ")\n\t\t}\n\n")
	body.WriteString("\t\treturn *typed, nil\n\t}\n\n")
	body.WriteString("\tif value.Type() != js.TypeObject {\n\t\treturn out, errors.New(" + strconv.Quote(name+": expected an object") + ")\n\t}\n\n")
	body.WriteString("\tif err := crystallineUnknownProperty(value, " + strconv.Quote(name) + ", crystallineKnown" + name + "); err != nil {\n\t\treturn out, err\n\t}\n\n")
	body.WriteString(fields.String())
	body.WriteString("\treturn out, nil\n}\n\n")

	return body.String(), nil
}

// fieldFromJS renders the read and assignment for one field.
func (e *emitter) fieldFromJS(target string, label string, t types.Type) (string, error) {
	converter, err := e.ensureValueConverter(t)
	if err != nil {
		return "", err
	}

	var body strings.Builder

	body.WriteString("\t\tconverted, err := " + converter + "(property)\n")
	body.WriteString("\t\tif err != nil {\n")
	body.WriteString("\t\t\treturn out, errors.New(" + strconv.Quote(label+": ") + " + err.Error())\n")
	body.WriteString("\t\t}\n\n")
	body.WriteString("\t\t" + target + " = converted\n")

	return body.String(), nil
}

// typeKey renders a type as a Go identifier fragment, so each one gets exactly
// one generated converter.
func typeKey(t types.Type) (string, error) {
	switch typed := t.(type) {
	case *types.Basic:
		return strings.Title(typed.Name()), nil //nolint:staticcheck
	case *types.Named:
		return instantiatedName(typed), nil
	case *types.Alias:
		return typed.Obj().Name(), nil
	case *types.Pointer:
		inner, err := typeKey(typed.Elem())

		return "Ptr" + inner, err
	case *types.Slice:
		if basic, ok := typed.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
			return "Bytes", nil
		}

		inner, err := typeKey(typed.Elem())

		return "SliceOf" + inner, err
	case *types.Array:
		inner, err := typeKey(typed.Elem())

		return "ArrayOf" + strconv.FormatInt(typed.Len(), 10) + inner, err
	case *types.Map:
		key, err := typeKey(typed.Key())
		if err != nil {
			return "", err
		}

		value, err := typeKey(typed.Elem())

		return "MapOf" + key + "To" + value, err
	}

	return "", fmt.Errorf("type %s cannot be read from JS", t)
}

// ensureValueConverter generates the JS to Go conversion for a type, once, and
// returns the name of the generated function.
func (e *emitter) ensureValueConverter(t types.Type) (string, error) {
	key, err := typeKey(t)
	if err != nil {
		return "", err
	}

	name := "crystallineTo" + key

	if _, done := e.converters[key]; done {
		return name, nil
	}

	// Reserve the name first so a self-referential type terminates.
	e.converters[key] = ""

	body, err := e.emitValueConverter(name, t)
	if err != nil {
		delete(e.converters, key)

		return "", err
	}

	e.converters[key] = body

	return name, nil
}

func (e *emitter) emitValueConverter(name string, t types.Type) (string, error) {
	goType := types.TypeString(t, e.qualifier)

	switch typed := t.(type) {
	case *types.Basic:
		return e.emitBasicConverter(name, goType, typed)
	case *types.Named, *types.Alias:
		if _, ok := t.Underlying().(*types.Struct); ok {
			return e.emitStructConverter(name, t.(*types.Named))
		}

		return e.emitValueConverterFor(name, goType, t.Underlying())
	case *types.Pointer:
		return e.emitPointerConverter(name, goType, typed)
	case *types.Slice:
		if basic, ok := typed.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
			return emitBytesConverter(name, goType), nil
		}

		return e.emitSliceConverter(name, goType, typed)
	case *types.Array:
		return e.emitArrayConverter(name, goType, typed)
	case *types.Map:
		return e.emitMapConverter(name, goType, typed)
	}

	return "", fmt.Errorf("type %s cannot be read from JS", t)
}

// emitValueConverterFor handles a defined type by converting through its
// underlying type and casting.
func (e *emitter) emitValueConverterFor(name string, goType string, underlying types.Type) (string, error) {
	inner, err := e.ensureValueConverter(underlying)
	if err != nil {
		return "", err
	}

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tconverted, err := " + inner + "(value)\n" +
		"\tif err != nil {\n\t\tvar zero " + goType + "\n\n\t\treturn zero, err\n\t}\n\n" +
		"\treturn " + goType + "(converted), nil\n}\n\n", nil
}

func (e *emitter) emitBasicConverter(name string, goType string, basic *types.Basic) (string, error) {
	var expected, accessor string

	switch {
	case basic.Kind() == types.Bool:
		expected, accessor = "js.TypeBoolean", "value.Bool()"
	case basic.Kind() == types.String:
		expected, accessor = "js.TypeString", "value.String()"
	case basic.Info()&types.IsNumeric != 0:
		expected, accessor = "js.TypeNumber", "value.Float()"
	default:
		return "", fmt.Errorf("type %s cannot be read from JS", basic)
	}

	want := strings.ToLower(strings.TrimPrefix(expected, "js.Type"))

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tvar zero " + goType + "\n\n" +
		"\tif value.Type() != " + expected + " {\n\t\treturn zero, errors.New(" + strconv.Quote("expected a "+want) + ")\n\t}\n\n" +
		"\treturn " + goType + "(" + accessor + "), nil\n}\n\n", nil
}

func emitBytesConverter(name string, goType string) string {
	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn nil, nil\n\t}\n\n" +
		"\tif !value.InstanceOf(js.Global().Get(\"Uint8Array\")) {\n\t\treturn nil, errors.New(\"expected a Uint8Array\")\n\t}\n\n" +
		"\tout := make(" + goType + ", value.Get(\"length\").Int())\n" +
		"\tif copied := js.CopyBytesToGo(out, value); copied != len(out) {\n\t\treturn nil, errors.New(\"expected a Uint8Array\")\n\t}\n\n" +
		"\treturn out, nil\n}\n\n"
}

func (e *emitter) emitSliceConverter(name string, goType string, typed *types.Slice) (string, error) {
	inner, err := e.ensureValueConverter(typed.Elem())
	if err != nil {
		return "", err
	}

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn nil, nil\n\t}\n\n" +
		"\tif !js.Global().Get(\"Array\").Call(\"isArray\", value).Bool() {\n\t\treturn nil, errors.New(\"expected an array\")\n\t}\n\n" +
		"\tout := make(" + goType + ", 0, value.Length())\n\n" +
		"\tfor i := 0; i < value.Length(); i++ {\n" +
		"\t\titem, err := " + inner + "(value.Index(i))\n" +
		"\t\tif err != nil {\n\t\t\treturn nil, errors.New(\"[\" + strconv.Itoa(i) + \"]: \" + err.Error())\n\t\t}\n\n" +
		"\t\tout = append(out, item)\n\t}\n\n" +
		"\treturn out, nil\n}\n\n", nil
}

func (e *emitter) emitArrayConverter(name string, goType string, typed *types.Array) (string, error) {
	inner, err := e.ensureValueConverter(typed.Elem())
	if err != nil {
		return "", err
	}

	length := strconv.FormatInt(typed.Len(), 10)

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tvar out " + goType + "\n\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn out, nil\n\t}\n\n" +
		"\tif !js.Global().Get(\"Array\").Call(\"isArray\", value).Bool() {\n\t\treturn out, errors.New(\"expected an array\")\n\t}\n\n" +
		"\tif value.Length() != " + length + " {\n\t\treturn out, errors.New(\"expected " + length + " items, got \" + strconv.Itoa(value.Length()))\n\t}\n\n" +
		"\tfor i := 0; i < " + length + "; i++ {\n" +
		"\t\titem, err := " + inner + "(value.Index(i))\n" +
		"\t\tif err != nil {\n\t\t\treturn out, errors.New(\"[\" + strconv.Itoa(i) + \"]: \" + err.Error())\n\t\t}\n\n" +
		"\t\tout[i] = item\n\t}\n\n" +
		"\treturn out, nil\n}\n\n", nil
}

func (e *emitter) emitMapConverter(name string, goType string, typed *types.Map) (string, error) {
	keyExpr, err := mapKeyFromString("key", typed.Key(), e.qualifier)
	if err != nil {
		return "", err
	}

	inner, err := e.ensureValueConverter(typed.Elem())
	if err != nil {
		return "", err
	}

	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn nil, nil\n\t}\n\n" +
		"\tif value.Type() != js.TypeObject {\n\t\treturn nil, errors.New(\"expected an object\")\n\t}\n\n" +
		"\tkeys := js.Global().Get(\"Object\").Call(\"keys\", value)\n" +
		"\tout := make(" + goType + ", keys.Length())\n\n" +
		"\tfor i := 0; i < keys.Length(); i++ {\n" +
		"\t\tkey := keys.Index(i).String()\n\n" +
		keyExpr +
		"\t\titem, err := " + inner + "(value.Get(key))\n" +
		"\t\tif err != nil {\n\t\t\treturn nil, errors.New(\"[\" + key + \"]: \" + err.Error())\n\t\t}\n\n" +
		"\t\tout[converted] = item\n\t}\n\n" +
		"\treturn out, nil\n}\n\n", nil
}

// mapKeyFromString parses a JS property name back into a Go map key.
func mapKeyFromString(expr string, t types.Type, qualifier types.Qualifier) (string, error) {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("map keys of type %s cannot be read from JS", t)
	}

	goType := types.TypeString(t, qualifier)

	switch {
	case basic.Kind() == types.String:
		return "\t\tconverted := " + goType + "(" + expr + ")\n\n", nil
	case basic.Info()&types.IsUnsigned != 0:
		return "\t\tparsed, err := strconv.ParseUint(" + expr + ", 10, 64)\n" +
			"\t\tif err != nil {\n\t\t\treturn nil, errors.New(\"key \" + " + expr + " + \" is not a number\")\n\t\t}\n\n" +
			"\t\tconverted := " + goType + "(parsed)\n\n", nil
	case basic.Info()&types.IsInteger != 0:
		return "\t\tparsed, err := strconv.ParseInt(" + expr + ", 10, 64)\n" +
			"\t\tif err != nil {\n\t\t\treturn nil, errors.New(\"key \" + " + expr + " + \" is not a number\")\n\t\t}\n\n" +
			"\t\tconverted := " + goType + "(parsed)\n\n", nil
	}

	return "", fmt.Errorf("map keys of type %s cannot be read from JS", t)
}

func (e *emitter) emitPointerConverter(name string, goType string, typed *types.Pointer) (string, error) {
	named, ok := typed.Elem().(*types.Named)
	if !ok || !isStructType(named) {
		return e.emitSimplePointerConverter(name, goType, typed)
	}

	inner, err := e.ensureValueConverter(named)
	if err != nil {
		return "", err
	}

	elem := types.TypeString(named, e.qualifier)

	// A wrapper resolves to the very pointer it was made from, so a round trip
	// through JS does not silently become a copy.
	return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
		"\tif value.IsUndefined() || value.IsNull() {\n\t\treturn nil, nil\n\t}\n\n" +
		"\tif handle, ok := crystallineHandleOf(value); ok {\n" +
		"\t\tresolved, found := crystallineResolve(handle)\n" +
		"\t\tif !found {\n\t\t\treturn nil, errors.New(\"the value behind this handle has been released\")\n\t\t}\n\n" +
		"\t\ttyped, ok := resolved.(*" + elem + ")\n" +
		"\t\tif !ok {\n\t\t\treturn nil, errors.New(\"handle refers to a different type\")\n\t\t}\n\n" +
		"\t\treturn typed, nil\n\t}\n\n" +
		"\tbuilt, err := " + inner + "(value)\n" +
		"\tif err != nil {\n\t\treturn nil, err\n\t}\n\n" +
		"\treturn &built, nil\n}\n\n", nil
}

// qualifier renders a type as generated code must spell it.
