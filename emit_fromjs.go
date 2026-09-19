//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Conversion of JavaScript values into Go, for parameters and for writing struct
// fields back.
//
// This direction is partial in a way the other is not: the input may be missing,
// extra or mistyped, so every converter validates rather than assuming.
// checkedFromJS renders a conversion as statements rather than as an
// expression: the value read out of JavaScript, and the branch reporting a
// failure to whoever asked for it.
//
// A panic is only the answer where the Go signature belongs to the consumer — what a callback
// returned, what a method of a supplied object returned. Everywhere else the
// generated code can report the failure itself, which is both one step shorter
// and the difference between working and aborting under a toolchain whose
// recover does nothing.
//
// returnsValue says which of the two shapes the enclosing function has: a
// wrapper returns the failure, a field setter reports it and stops.
// converter names the conversion to use, or is empty to work it out from the
// type. A field tagged bigint is the one case where the tag decides and the
// type cannot.
func (e *emitter) checkedFromJS(name string, expr string, t types.Type, returnsValue bool, converter string) (string, error) {
	// A callback is built rather than converted, and building cannot fail. Its
	// own conversions happen later, inside the callback.
	if callback, ok := t.Underlying().(*types.Signature); ok && converter == "" {
		built, err := e.callbackFromJS(expr, t, callback)
		if err != nil {
			return "", err
		}

		return "\t" + name + " := " + built + "\n\n", nil
	}

	if converter == "" {
		resolved, err := e.ensureValueConverter(t)
		if err != nil {
			return "", err
		}

		converter = resolved
	}

	var out strings.Builder

	out.WriteString("\t" + name + ", err := " + converter + "(" + expr + ")\n")
	out.WriteString("\tif err != nil {\n")

	if returnsValue {
		out.WriteString("\t\treturn crystallineFail(err.Error())\n")
	} else {
		out.WriteString("\t\tcrystallineFail(err.Error())\n\n\t\treturn\n")
	}

	out.WriteString("\t}\n\n")

	return out.String(), nil
}

// panickingFromJS renders a conversion whose only answer to failure is a panic.
//
// It is reached where the Go signature belongs to the consumer — what a
// callback returned, what a method of a supplied object returned — so there is
// no error to return and no slot to report through. Statements rather than an
// expression, because the message has to name where it came from and Go will
// not take a two-valued call alongside another argument.
func (e *emitter) panickingFromJS(name string, expr string, t types.Type, subject string) (string, error) {
	converter, err := e.ensureValueConverter(t)
	if err != nil {
		return "", err
	}

	var out strings.Builder

	out.WriteString("\t" + name + ", err := " + converter + "(" + expr + ")\n")
	out.WriteString("\tif err != nil {\n")
	out.WriteString("\t\tpanic(" + strconv.Quote(subject+": ") + " + err.Error())\n")
	out.WriteString("\t}\n\n")

	return out.String(), nil
}

// mustSubject names whatever a panicking conversion belongs to. It is only ever
// a callback result or a supplied object's result, both of which happen inside
// a member the emitter is already in the middle of.
func (e *emitter) mustSubject() string {
	if e.subject == "" {
		return "a value from JavaScript"
	}

	return e.subject
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
	converted, err := e.panickingFromJS("crystallineConverted", "crystallineCallbackResult", result,
		e.mustSubject()+": callback result")
	if err != nil {
		return "", fmt.Errorf("callback result: %w", err)
	}

	body.WriteString(" " + types.TypeString(result, e.qualifier) + " {\n")
	body.WriteString("\t\tcrystallineCallbackResult := " + invocation + "\n\n")
	body.WriteString(converted)
	body.WriteString("\t\treturn crystallineConverted\n")
	body.WriteString("\t}")

	return body.String(), nil
}

// fieldSetter renders the write half of a field accessor.
//
// It goes through the same converters a parameter does. An earlier version
// handled only basic types, so a write to a slice, map, struct or pointer field
// was discarded in silence: the Go value kept its old contents and nothing was
// reported.
func (e *emitter) fieldSetter(target string, t types.Type, converter string) (string, error) {
	statements, err := e.checkedFromJS("converted", "value", t, false, converter)
	if err != nil {
		return "", err
	}

	// A named type still has to arrive as itself, and the bigint converters
	// produce the plain width they parsed.
	assigned := "converted"
	if converter != "" {
		assigned = types.TypeString(t, e.qualifier) + "(converted)"
	}

	return "func(value js.Value) {\n" + statements + "\t" + target + " = " + assigned + "\n\t}", nil
}

// emitConverter renders the JS to Go conversion for a struct: a wrapper handed
// back resolves to the value it came from, anything else is built field by
// field and validated.
func (e *emitter) emitStructConverter(fn string, named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

	name := e.imports.goTypeName(named)
	goType := e.declaredName(named)

	var known []string

	var fields strings.Builder

	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
		jsField := e.gen.jsMemberName(field.Name(), tag)

		known = append(known, strconv.Quote(jsField)+": true")

		check, err := e.fieldFromJS("out."+field.Name(), typeIdentity(named)+"."+jsField, field.Type())
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, field.Name(), err)
		}

		fields.WriteString("\tif property := value.Get(" + strconv.Quote(jsField) + "); !property.IsUndefined() && !property.IsNull() {\n")
		fields.WriteString(check)
		fields.WriteString("\t}\n\n")
	}

	var body strings.Builder

	body.WriteString("var crystallineKnown" + name + " = map[string]bool{" + strings.Join(known, ", ") + "}\n\n")

	// The identifier and the name a person reads are different jobs. The
	// identifier carries the package so that two packages may each declare a
	// Config, which spells it SampleFnSample: a name that appears nowhere in
	// the Go and cannot be searched for.
	subject := typeIdentity(named)

	body.WriteString("func " + fn + "(value js.Value) (" + goType + ", error) {\n")
	body.WriteString("\tvar out " + goType + "\n\n")
	// A pointer position handles its own nil before reaching here, so a null
	// arriving at a struct is a value nobody asked for rather than an absence.
	body.WriteString("\tif value.IsUndefined() || value.IsNull() {\n\t\treturn out, errors.New(" +
		strconv.Quote(subject+": expected an object, got null") + ")\n\t}\n\n")
	body.WriteString("\tif handle, ok := crystallineHandleOf(value); ok {\n")
	body.WriteString("\t\tresolved, found := crystallineResolve(handle)\n")
	body.WriteString("\t\tif !found {\n\t\t\treturn out, errors.New(" + strconv.Quote(subject+": the value behind this handle has been released") + ")\n\t\t}\n\n")
	body.WriteString("\t\ttyped, ok := resolved.(*" + goType + ")\n")
	body.WriteString("\t\tif !ok {\n\t\t\treturn out, errors.New(" + strconv.Quote(subject+": handle refers to a different type") + ")\n\t\t}\n\n")
	body.WriteString("\t\treturn *typed, nil\n\t}\n\n")
	body.WriteString("\tif value.Type() != js.TypeObject {\n\t\treturn out, errors.New(" + strconv.Quote(subject+": expected an object") + ")\n\t}\n\n")
	body.WriteString("\tif err := crystallineUnknownProperty(value, " + strconv.Quote(subject) + ", crystallineKnown" + name + "); err != nil {\n\t\treturn out, err\n\t}\n\n")
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
func (e *emitter) typeKey(t types.Type) (string, error) {
	t = unaliased(t)

	switch typed := t.(type) {
	case *types.Basic:
		return strings.Title(typed.Name()), nil //nolint:staticcheck
	case *types.Named:
		return e.imports.goTypeName(typed), nil
	case *types.Alias:
		return typed.Obj().Name(), nil
	case *types.Pointer:
		inner, err := e.typeKey(typed.Elem())

		return "Ptr" + inner, err
	case *types.Slice:
		if basic, ok := typed.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
			return "Bytes", nil
		}

		inner, err := e.typeKey(typed.Elem())

		return "SliceOf" + inner, err
	case *types.Array:
		inner, err := e.typeKey(typed.Elem())

		return "ArrayOf" + strconv.FormatInt(typed.Len(), 10) + inner, err
	case *types.Map:
		key, err := e.typeKey(typed.Key())
		if err != nil {
			return "", err
		}

		value, err := e.typeKey(typed.Elem())

		return "MapOf" + key + "To" + value, err
	}

	return "", fmt.Errorf("type %s cannot be read from JS", t)
}

// ensureValueConverter generates the JS to Go conversion for a type, once, and
// returns the name of the generated function.
// ensureImportConverter is ensureValueConverter for the direction Go reaches
// out in. It is a separate entry point because the cache is keyed by type, and
// one interface may legitimately be both supplied and imported.
func (e *emitter) ensureImportConverter(named *types.Named, promised []string, called map[string]string) (string, error) {
	declared, ok := named.Underlying().(*types.Interface)
	if !ok {
		return "", fmt.Errorf("%s is not an interface", named)
	}

	key, err := e.typeKey(named)
	if err != nil {
		return "", err
	}

	// Keyed by what the adapter does, not only by the type it adapts.
	key += adapterSuffix(promised, called)
	name := "crystallineImport" + key
	cached := "import:" + key

	if _, done := e.converters[cached]; done {
		return name, nil
	}

	e.converters[cached] = ""

	body, err := e.emitSuppliedConverter(name, named, declared, importMode(promised, called))
	if err != nil {
		delete(e.converters, cached)

		return "", err
	}

	e.converters[cached] = body

	return name, nil
}

func (e *emitter) ensureValueConverter(t types.Type) (string, error) {
	key, err := e.typeKey(t)
	if err != nil {
		return "", err
	}

	name := "crystallineTo" + key

	if _, done := e.converters[key]; done {
		return name, nil
	}

	// Reserve the name first so a self-referential type terminates.
	e.converters[key] = ""

	e.imports.begin()

	body, err := e.emitValueConverter(name, t)
	if err != nil {
		e.imports.rollback()
		delete(e.converters, key)

		return "", err
	}

	e.imports.commit()

	e.converters[key] = body

	return name, nil
}

func (e *emitter) emitValueConverter(name string, t types.Type) (string, error) {
	t = unaliased(t)

	goType := types.TypeString(t, e.qualifier)

	if mapped, ok := e.marks.marshallerFor(t); ok {
		// A declared mapping reads what it crosses as, then hands that to the
		// function the manifest named.
		if mapped.declaredByManifest() {
			inner, err := e.ensureValueConverter(mapped.intermediate)
			if err != nil {
				return "", fmt.Errorf("%s crosses as %s: %w", goType, mapped.intermediate, err)
			}

			return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
				"\tcrystallineCrossed, err := " + inner + "(value)\n" +
				"\tif err != nil {\n\t\tvar zero " + goType + "\n\n\t\treturn zero, err\n\t}\n\n" +
				"\treturn " + qualified(e.qualifier(mapped.from.Pkg()), mapped.from.Name()) + "(crystallineCrossed)\n}\n\n", nil
		}

		pkg := ""
		if path := marshallerPackage(t); path != "" {
			pkg = e.imports.add(path, lastSegment(path))
		}

		return "func " + name + "(value js.Value) (" + goType + ", error) {\n" +
			mapped.fromJS(goType, pkg) + "}\n\n", nil
	}

	switch typed := t.(type) {
	case *types.Basic:
		return e.emitBasicConverter(name, goType, typed)
	case *types.Named, *types.Alias:
		if _, ok := t.Underlying().(*types.Struct); ok {
			return e.emitStructConverter(name, t.(*types.Named))
		}

		// An interface Go declares is supplied by JavaScript: the object
		// provides the methods, and Go calls out through them. Checked before
		// unwrapping, because the adapter needs the name.
		if declared, ok := t.Underlying().(*types.Interface); ok && !isErrorType(t) {
			if named, ok := t.(*types.Named); ok {
				return e.emitSuppliedConverter(name, named, declared, suppliedMode())
			}
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

	// IsNumeric includes the complex kinds, which have no JS counterpart and
	// which the declarations refuse. Accepting them here emitted
	// complex128(value.Float()), and left the two sides disagreeing about
	// whether the member could be bound at all.
	case basic.Info()&types.IsComplex != 0:
		return "", fmt.Errorf("%s cannot be converted to wasm", basic)
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

// emitSuppliedConverter renders an adapter letting Go call an object that came
// from JavaScript.
//
// Each method is what a callback parameter already is, so the machinery is the
// same one method at a time: arguments convert out, the result is awaited in
// case JavaScript returned a promise, and it converts back.
// adapterMode is the one difference between an object JavaScript hands in and
// one Go reaches out for: whether a result is awaited.
//
// A supplied parameter makes its whole call a promise, so awaiting is free
// there. An import has no promise above it, so awaiting blocks a goroutine
// inside whatever synchronous call is running.
type adapterMode struct {
	prefix   string
	awaitAll bool
	promised map[string]bool
	called   map[string]string
}

func suppliedMode() adapterMode {
	return adapterMode{prefix: "crystallineJS", awaitAll: true}
}

func importMode(promised []string, called map[string]string) adapterMode {
	allowed := make(map[string]bool, len(promised))
	for _, method := range promised {
		allowed[method] = true
	}

	return adapterMode{
		prefix:   "crystallineImported" + adapterSuffix(promised, called),
		promised: allowed,
		called:   called,
	}
}

// adapterSuffix distinguishes two imports of one interface that ask for
// different things from it.
//
// Without it the first one generated wins and the second silently reuses it,
// which is the same defect as any other pair of decisions sharing one key. It
// covers everything the adapter's body depends on — which methods are awaited
// and what each is called — because covering only some of that is how the bug
// came back the first time.
//
// The order a manifest happens to list options in is not a difference, so both
// are sorted.
func adapterSuffix(promised []string, called map[string]string) string {
	var out strings.Builder

	if len(promised) > 0 {
		sorted := slices.Clone(promised)
		slices.Sort(sorted)

		out.WriteString("Awaiting" + strings.Join(sorted, "And"))
	}

	for _, method := range sortedKeys(called) {
		out.WriteString("Calling" + capitalise(method) + "As" + capitalise(called[method]))
	}

	return out.String()
}

// jsMethodName is what an adapter calls a method, which is the name the surface
// uses unless the import spelled it out.
func (m adapterMode) jsMethodName(e *emitter, method string) string {
	if named, ok := m.called[method]; ok {
		return named
	}

	return e.gen.jsMemberName(method, "")
}

func (e *emitter) emitSuppliedConverter(name string, named *types.Named, declared *types.Interface, mode adapterMode) (string, error) {
	adapter := mode.prefix + named.Obj().Name()
	goType := e.declaredName(named)

	var out strings.Builder

	out.WriteString("type " + adapter + " struct{ value js.Value }\n\n")

	required := make([]string, 0, declared.NumMethods())

	for i := range declared.NumMethods() {
		method := declared.Method(i)

		// Skipping it would leave a type that does not satisfy the interface it
		// claims to, which only the consumer's own build would notice.
		if !method.Exported() {
			return "", fmt.Errorf("%s has the unexported method %s, which JavaScript cannot provide",
				named.Obj().Name(), method.Name())
		}

		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		if sig.Results().Len() > 1 {
			return "", fmt.Errorf("%s.%s returns %d values, and an object supplied from JavaScript returns one",
				named.Obj().Name(), method.Name(), sig.Results().Len())
		}

		// A JavaScript function takes its arguments one at a time, so there is
		// no honest shape for a variadic method: rendering it as taking a slice
		// declares a different method from the one being implemented.
		if sig.Variadic() {
			return "", fmt.Errorf("%s.%s is variadic, which an object supplied from JavaScript cannot be",
				named.Obj().Name(), method.Name())
		}

		required = append(required, mode.jsMethodName(e, method.Name()))

		params := make([]string, 0, sig.Params().Len())
		passed := make([]string, 0, sig.Params().Len())

		for j := range sig.Params().Len() {
			param := sig.Params().At(j)
			local := "p" + strconv.Itoa(j)

			params = append(params, local+" "+types.TypeString(param.Type(), e.qualifier))

			converted, err := e.toJS(local, param.Type(), false)
			if err != nil {
				return "", fmt.Errorf("%s.%s parameter %d: %w", named.Obj().Name(), method.Name(), j+1, err)
			}

			passed = append(passed, converted)
		}

		call := "c.value.Call(" + strconv.Quote(mode.jsMethodName(e, method.Name()))
		if len(passed) > 0 {
			call += ", " + strings.Join(passed, ", ")
		}

		call += ")"

		invocation := "crystallineAwait(" + call + ")"
		if !mode.awaitAll && !mode.promised[method.Name()] {
			invocation = "crystallineDirect(" + call + ", " +
				strconv.Quote(memberIdentity(named, method.Name())) + ")"
		}

		out.WriteString("func (c " + adapter + ") " + method.Name() + "(" + strings.Join(params, ", ") + ")")

		if sig.Results().Len() == 0 {
			out.WriteString(" {\n\t" + invocation + "\n}\n\n")

			continue
		}

		result := sig.Results().At(0).Type()

		converted, err := e.panickingFromJS("crystallineConverted", "crystallineSupplied", result,
			memberIdentity(named, method.Name())+": result")
		if err != nil {
			return "", fmt.Errorf("%s.%s result: %w", named.Obj().Name(), method.Name(), err)
		}

		out.WriteString(" " + types.TypeString(result, e.qualifier) + " {\n")
		out.WriteString("\tcrystallineSupplied := " + invocation + "\n\n")
		out.WriteString(converted)
		out.WriteString("\treturn crystallineConverted\n}\n\n")
	}

	out.WriteString("func " + name + "(value js.Value) (" + goType + ", error) {\n")
	out.WriteString("\tif value.Type() != js.TypeObject {\n\t\treturn nil, errors.New(" +
		strconv.Quote(typeIdentity(named)+": expected an object") + ")\n\t}\n\n")

	for _, method := range required {
		out.WriteString("\tif value.Get(" + strconv.Quote(method) + ").Type() != js.TypeFunction {\n")
		out.WriteString("\t\treturn nil, errors.New(" +
			strconv.Quote(typeIdentity(named)+": the object has no "+method+" method") + ")\n\t}\n\n")
	}

	out.WriteString("\treturn " + adapter + "{value: value}, nil\n}\n\n")

	return out.String(), nil
}
