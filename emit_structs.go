//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Emission of struct wrappers: the live object JavaScript sees, its methods, and
// the handle that keeps it identified across the boundary.
func (e *emitter) qualifier(pkg *types.Package) string {
	return e.imports.qualifier(pkg)
}

func (e *emitter) isIgnored(named *types.Named, method string) bool {
	return e.marks.ignored[markKey(named, method)]
}

func (e *emitter) isPromised(named *types.Named, method string, obj types.Object) bool {
	return e.marks.promised[markKey(named, method)] || e.gen.isPromise(obj)
}

// fieldValue renders what a field hands to JavaScript, honouring its tag.
//
// One answer for the live wrapper and the plain-data snapshot alike, so a tag
// cannot mean one thing in a wrapper and something else in a copy of it.
func (e *emitter) fieldValue(target string, t types.Type, tag string) (string, error) {
	if tagHasOption(tag, tagBigInt) {
		if isUnsignedWide(t) {
			return "crystallineBigUint(uint64(" + target + "))", nil
		}

		return "crystallineBigInt(int64(" + target + "))", nil
	}

	return e.toJS(target, t, tagHasOption(tag, tagNotNil))
}

// fieldConverter names the conversion a write to a field goes through, or is
// empty for whatever the field's type implies on its own.
func fieldConverter(t types.Type, tag string) string {
	if !tagHasOption(tag, tagBigInt) {
		return ""
	}

	if isUnsignedWide(t) {
		return "crystallineToBigUint"
	}

	return "crystallineToBigInt"
}

func (e *emitter) emitMarshaller(named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

	name := e.imports.goTypeName(named)
	identity := typeIdentity(named)

	var body strings.Builder

	body.WriteString("func " + marshalName(name) + "(v *" + e.declaredName(named) + ") any {\n")
	body.WriteString("\tif v == nil {\n\t\treturn nil\n\t}\n\n")
	body.WriteString("\tout := js.Global().Get(\"Object\").New()\n")
	body.WriteString("\tscope := &crystallineScope{}\n\n")

	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
		if err := validateTag(tag, field.Type()); err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, field.Name(), err)
		}

		expr, err := e.fieldValue("v."+field.Name(), field.Type(), tag)
		if err != nil {
			e.skipAt(field.Pos(), identity+"."+field.Name(), err.Error())

			continue
		}

		setter, err := e.fieldSetter("v."+field.Name(), field.Type(), fieldConverter(field.Type(), tag))
		if err != nil {
			// A field that cannot be written back is still readable. Saying so
			// on the way past beats accepting the write and dropping it.
			e.readonly[identity+"."+field.Name()] = true

			setter = "func(js.Value) {\n\t\tcrystallineFail(" +
				strconv.Quote(identity+"."+field.Name()+" cannot be written from JavaScript: "+err.Error()) + ")\n\t}"
		}

		preamble, getter := e.fieldGetter(field.Name(), "v."+field.Name(), field.Type(), expr)

		body.WriteString(preamble)
		body.WriteString("\tcrystallineDefine(scope, out, " + strconv.Quote(e.gen.jsMemberName(field.Name(), tag)) + ", " + getter + ", " + setter + ")\n")
	}

	methods := exportedMethods(named)

	for _, method := range methods {
		if e.isIgnored(named, method.Name()) {
			continue
		}

		bound, err := e.emitMethod(named, method)
		if err != nil {
			e.skipAt(method.Pos(), identity+"."+method.Name(), err.Error())

			continue
		}

		body.WriteString("\tout.Set(" + strconv.Quote(e.gen.jsMemberName(method.Name(), "")) + ", " + bound + ")\n")
	}

	body.WriteString("\n\tcrystallineAttach(out, crystallineRetain(v, scope), scope)\n\n")
	body.WriteString("\treturn out\n}\n\n")

	return body.String(), nil
}

// emitPlainMarshaller renders a struct as ordinary JavaScript data.
//
// A wrapper is a live view: a call across the boundary per field read, a bridge
// slot per field and method, and a handle to release. For a value that is only
// read, converting once and handing back a plain object is far cheaper, and the
// object behaves like any other JavaScript data.
//
// The trade is stated rather than hidden: the result is a snapshot, it cannot
// be written back, and it carries no methods.
// exportedMethods returns everything callable on a value of the type, in a
// stable order.
//
// Named.NumMethods reports only what the type declares, so a method promoted
// from an embedded field was dropped without a word. Go's own promotion rules
// are what a caller expects: if e.Promoted() compiles in Go, it should exist in
// JavaScript. The method set of the pointer is used because it is the wider of
// the two, and a wrapper is backed by an addressable value either way.
func exportedMethods(named *types.Named) []*types.Func {
	set := types.NewMethodSet(types.NewPointer(named))

	methods := make([]*types.Func, 0, set.Len())

	for i := range set.Len() {
		fn, ok := set.At(i).Obj().(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}

		methods = append(methods, fn)
	}

	sort.Slice(methods, func(i, j int) bool { return methods[i].Name() < methods[j].Name() })

	return methods
}

func (e *emitter) emitPlainMarshaller(named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

	name := e.imports.goTypeName(named)
	identity := typeIdentity(named)

	var body strings.Builder

	body.WriteString("func " + marshalName(name) + "(v *" + e.declaredName(named) + ") any {\n")
	body.WriteString("\tif v == nil {\n\t\treturn nil\n\t}\n\n")

	// Built field by field rather than from a Go map, so that the property
	// order is the declaration order rather than whatever the map iterated.
	body.WriteString("\tout := js.Global().Get(\"Object\").New()\n\n")

	for i := range structType.NumFields() {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
		if err := validateTag(tag, field.Type()); err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, field.Name(), err)
		}

		expr, err := e.fieldValue("v."+field.Name(), field.Type(), tag)
		if err != nil {
			e.skipAt(field.Pos(), identity+"."+field.Name(), err.Error())

			continue
		}

		body.WriteString("\tout.Set(" + strconv.Quote(e.gen.jsMemberName(field.Name(), tag)) + ", " + expr + ")\n")
	}

	// Methods have nowhere to live on plain data. Saying which ones went is the
	// difference between a documented trade and a silent one.
	for _, method := range exportedMethods(named) {
		if !e.isIgnored(named, method.Name()) {
			e.skipAt(method.Pos(), identity+"."+method.Name(), "not bound: "+identity+" is marshalled as plain data")
		}
	}

	body.WriteString("\n\treturn out\n}\n\n")

	return body.String(), nil
}

// fieldGetter renders the read half of a field accessor, caching the wrapper a
// struct-typed field hands back.
//
// Rebuilding it on every read allocated a handle and a set of bridge slots each
// time, and meant a.Field !== a.Field, which breaks ===, Map keys and every
// framework's memo comparison. A struct field has a stable address, so a
// wrapper over it stays a live view however the field is reassigned. A pointer
// field is keyed on the pointer, so it refreshes when Go points elsewhere.
//
// Slices and maps are deliberately not cached: converting one produces a
// snapshot rather than a view, so a cached copy would hide a later change.
func (e *emitter) fieldGetter(name string, target string, t types.Type, expr string) (string, string) {
	t = unaliased(t)

	plain := "func() any {\n\t\treturn " + expr + "\n\t}"

	cache := "crystallineCache" + name
	cached := "crystallineCached" + name

	switch typed := t.(type) {
	case *types.Named, *types.Alias:
		if _, ok := t.Underlying().(*types.Struct); !ok {
			return "", plain
		}

		preamble := "\tvar " + cache + " any\n\tvar " + cached + " bool\n\n"

		return preamble, "func() any {\n" +
			"\t\tif !" + cached + " {\n" +
			"\t\t\t" + cached + " = true\n" +
			"\t\t\t" + cache + " = " + expr + "\n" +
			"\t\t}\n\n" +
			"\t\treturn " + cache + "\n\t}"

	case *types.Pointer:
		if _, ok := typed.Elem().Underlying().(*types.Struct); !ok {
			return "", plain
		}

		if _, ok := typed.Elem().(*types.Named); !ok {
			return "", plain
		}

		holder := "crystallineCacheFor" + name

		preamble := "\tvar " + cache + " any\n\tvar " + cached + " bool\n\tvar " + holder + " " +
			types.TypeString(t, e.qualifier) + "\n\n"

		return preamble, "func() any {\n" +
			"\t\tif !" + cached + " || " + holder + " != " + target + " {\n" +
			"\t\t\t" + cached + " = true\n" +
			"\t\t\t" + holder + " = " + target + "\n" +
			"\t\t\t" + cache + " = " + expr + "\n" +
			"\t\t}\n\n" +
			"\t\treturn " + cache + "\n\t}"
	}

	return "", plain
}

func (e *emitter) emitMethod(named *types.Named, method *types.Func) (string, error) {
	sig := method.Type().(*types.Signature)

	e.subject = memberIdentity(named, method.Name())
	defer func() { e.subject = "" }()

	returns, err := e.emitCall(sig, func(call []string) string {
		return "v." + method.Name() + "(" + spread(sig, call) + ")"
	})
	if err != nil {
		return "", err
	}

	var body strings.Builder

	body.WriteString("crystallineWrap(scope.fn(func(this js.Value, args []js.Value) (result any) {\n")
	body.WriteString("\t\tdefer crystallineRecover(&result)\n\n")
	body.WriteString("\t\tif len(args) != " + strconv.Itoa(sig.Params().Len()) + " {\n")
	body.WriteString("\t\t\treturn crystallineFail(" + strconv.Quote(e.gen.jsMemberName(method.Name(), "")+": expected "+strconv.Itoa(sig.Params().Len())+" arguments, got ") + " + strconv.Itoa(len(args)))\n")
	body.WriteString("\t\t}\n\n")
	body.WriteString(wrapPromise(returns, e.isPromised(named, method.Name(), method) || asyncSignature(sig), 2))
	body.WriteString("\t}))")

	return body.String(), nil
}

func (e *emitter) queue(named *types.Named) {
	if _, done := e.marshallers[e.imports.goTypeName(named)]; done {
		return
	}

	e.pending = append(e.pending, named)
}

func isErrorType(t types.Type) bool {
	obj := namedObject(t)

	return obj != nil && obj.Pkg() == nil && obj.Name() == "error"
}

// prelude is the fixed support code every generated file carries. It is
// duplicated per package rather than imported so that generated bindings never
// depend on the reflect-based runtime.
