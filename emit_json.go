//go:build !js

package crystalline

import (
	"go/types"
	"reflect"
	"strconv"
	"strings"
)

// Plain data crossing as one JSON string, which JavaScript parses.
//
// Every Go string that crosses on its own is a TextDecoder call of its own, and
// that includes each field name and map key handed to Value.Set, not only the
// string values. Measured, a call like that costs more than the crossing it
// rides on, so a struct or a map built property by property is dominated by
// them. JSON pays for one decode, however many there are.
//
// Numbers alone are the exception: formatting a float in wasm costs more than
// the crossing it saves, so an array of them is left as it was.

// jsonRoot renders a conversion that crosses as JSON, falling back to the
// ordinary one for a value JSON cannot spell: a NaN or an infinity, which only
// show up at run time.
func (e *emitter) jsonRoot(expr string, t types.Type, nonNil bool) (string, bool, error) {
	if e.inJSON || !jsonWorth(t, map[*types.Named]bool{}) || !e.jsonSafe(t, map[*types.Named]bool{}) {
		return "", false, nil
	}

	e.inJSON = true
	fallback, err := e.toJS(expr, t, nonNil)
	e.inJSON = false

	if err != nil {
		return "", false, err
	}

	encode := e.appendJSON(expr, t, nonNil, 0)

	return "crystallineJSON(func(b []byte, ok *bool) []byte {\n" + encode + "\t\treturn b\n\t}, func() any {\n\t\treturn " + fallback + "\n\t})", true, nil
}

// jsonWorth reports whether a value holds something JSON carries cheaper than
// a direct conversion does: an object, or a string.
func jsonWorth(t types.Type, seen map[*types.Named]bool) bool {
	t = unaliased(t)

	switch typed := t.(type) {
	case *types.Named:
		if _, ok := typed.Underlying().(*types.Struct); ok {
			return true
		}

		return jsonWorth(typed.Underlying(), seen)
	case *types.Map:
		return true
	case *types.Pointer:
		return jsonWorth(typed.Elem(), seen)
	case *types.Slice:
		return isString(typed.Elem()) || jsonWorth(typed.Elem(), seen)
	case *types.Array:
		return isString(typed.Elem()) || jsonWorth(typed.Elem(), seen)
	}

	return false
}

// jsonNull is how JSON spells an absent slice, map or pointer, as js.ValueOf
// does.
const jsonNull = "null"

func isString(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)

	return ok && basic.Info()&types.IsString != 0
}

// jsonSafe reports whether a value converts to exactly the JavaScript that a
// direct conversion produces, spelled as JSON. Anything with a counterpart JSON
// has no spelling for is not: a wrapper, a Date, a Uint8Array, a bigint, an
// Error, an iterator.
func (e *emitter) jsonSafe(t types.Type, seen map[*types.Named]bool) bool {
	t = unaliased(t)

	switch typed := t.(type) {
	case *types.Basic:
		return typed.Info()&(types.IsBoolean|types.IsString) != 0 ||
			(typed.Info()&types.IsNumeric != 0 && typed.Info()&types.IsComplex == 0)
	case *types.Named:
		if _, mapped := e.marks.marshallerFor(typed); mapped || isErrorType(typed) {
			return false
		}

		structType, isStruct := typed.Underlying().(*types.Struct)
		if !isStruct {
			return e.jsonSafe(typed.Underlying(), seen)
		}

		// A struct is data only where it crosses as data.
		if !e.plain && !e.marks.isPlain(typed) {
			return false
		}

		// A type that contains itself is safe if the rest of it is.
		if seen[typed] {
			return true
		}

		seen[typed] = true

		for i := range structType.NumFields() {
			field := structType.Field(i)
			if !field.Exported() {
				continue
			}

			tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
			if tagHasOption(tag, tagBigInt) || validateTag(tag, field.Type()) != nil || !e.jsonSafe(field.Type(), seen) {
				return false
			}
		}

		return true
	case *types.Pointer:
		return e.jsonSafe(typed.Elem(), seen)
	case *types.Slice:
		return !isByte(typed.Elem()) && e.jsonSafe(typed.Elem(), seen)
	case *types.Array:
		return !isByte(typed.Elem()) && e.jsonSafe(typed.Elem(), seen)
	case *types.Map:
		if _, mapped := e.marks.marshallerFor(typed.Key()); mapped {
			return false
		}

		if _, err := e.mapKeyToString("k", typed.Key()); err != nil {
			return false
		}

		return e.jsonSafe(typed.Elem(), seen)
	}

	return false
}

func isByte(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)

	return ok && basic.Kind() == types.Uint8
}

// appendJSON renders statements appending a value to b as JSON, the same value
// toJS would have built. Only called on what jsonSafe accepted.
func (e *emitter) appendJSON(expr string, t types.Type, nonNil bool, depth int) string {
	t = unaliased(t)
	indent := strings.Repeat("\t", depth+2)
	n := strconv.Itoa(depth)

	switch typed := t.(type) {
	case *types.Basic:
		switch {
		case typed.Info()&types.IsBoolean != 0:
			return indent + "b = strconv.AppendBool(b, bool(" + expr + "))\n"
		case typed.Info()&types.IsString != 0:
			return indent + "b = crystallineAppendString(b, string(" + expr + "))\n"
		}

		return indent + "b = crystallineAppendNumber(b, float64(" + expr + "), ok)\n"
	case *types.Named:
		if _, ok := typed.Underlying().(*types.Struct); ok {
			return indent + "b = " + e.appendFunc(typed) + "(b, &" + expr + ", ok)\n"
		}

		return e.appendJSON(expr, typed.Underlying(), nonNil, depth)
	case *types.Pointer:
		if named, ok := unaliased(typed.Elem()).(*types.Named); ok {
			if _, isStruct := named.Underlying().(*types.Struct); isStruct {
				return indent + "b = " + e.appendFunc(named) + "(b, " + expr + ", ok)\n"
			}
		}

		return indent + "if " + expr + " == nil {\n" + indent + "\tb = append(b, \"null\"...)\n" + indent + "} else {\n" +
			e.appendJSON("(*"+expr+")", typed.Elem(), false, depth+1) + indent + "}\n"
	case *types.Slice:
		empty := jsonNull
		if nonNil {
			empty = "[]"
		}

		items, i := "items"+n, "i"+n

		return indent + "if " + items + " := " + expr + "; " + items + " == nil {\n" +
			indent + "\tb = append(b, \"" + empty + "\"...)\n" +
			indent + "} else {\n" +
			e.appendSequence(items, i, typed.Elem(), depth+1) +
			indent + "}\n"
	case *types.Array:
		items, i := "items"+n, "i"+n

		return indent + "{\n" + indent + "\t" + items + " := &" + expr + "\n" +
			e.appendSequence(items, i, typed.Elem(), depth+1) + indent + "}\n"
	case *types.Map:
		empty := jsonNull
		if nonNil {
			empty = "{}"
		}

		entries, first, k, v := "entries"+n, "first"+n, "k"+n, "v"+n

		// Checked by jsonSafe, which refuses a key that cannot be rendered.
		key, _ := e.mapKeyToString(k, typed.Key())

		return indent + "if " + entries + " := " + expr + "; " + entries + " == nil {\n" +
			indent + "\tb = append(b, \"" + empty + "\"...)\n" +
			indent + "} else {\n" +
			indent + "\tb = append(b, '{')\n" +
			indent + "\t" + first + " := true\n" +
			indent + "\tfor " + k + ", " + v + " := range " + entries + " {\n" +
			indent + "\t\tif !" + first + " {\n" + indent + "\t\t\tb = append(b, ',')\n" + indent + "\t\t}\n\n" +
			indent + "\t\t" + first + " = false\n" +
			indent + "\t\tb = crystallineAppendString(b, " + key + ")\n" +
			indent + "\t\tb = append(b, ':')\n" +
			e.appendJSON(v, typed.Elem(), false, depth+2) +
			indent + "\t}\n\n" +
			indent + "\tb = append(b, '}')\n" +
			indent + "}\n"
	}

	// jsonSafe accepted nothing else.
	return indent + "*ok = false\n"
}

// appendSequence renders the elements of a slice or array, indexed so that a
// struct element is addressed where it lives.
func (e *emitter) appendSequence(items string, i string, elem types.Type, depth int) string {
	indent := strings.Repeat("\t", depth+2)

	return indent + "b = append(b, '[')\n" +
		indent + "for " + i + " := range " + items + " {\n" +
		indent + "\tif " + i + " > 0 {\n" + indent + "\t\tb = append(b, ',')\n" + indent + "\t}\n\n" +
		e.appendJSON(items+"["+i+"]", elem, false, depth+1) +
		indent + "}\n\n" +
		indent + "b = append(b, ']')\n"
}

// appendFunc names the encoder for a struct, queueing it. One encoder serves a
// type whether it is plain or copied for a plain result: the JSON is the same.
func (e *emitter) appendFunc(named *types.Named) string {
	name := appendName(e.imports.goTypeName(named))

	if _, done := e.marshallers[name]; !done {
		e.pendingAppend = append(e.pendingAppend, named)
	}

	return name
}

func appendName(name string) string {
	return "crystallineAppend" + name
}

// emitAppender renders the JSON encoder for a struct.
func (e *emitter) emitAppender(named *types.Named) (string, error) {
	structType := named.Underlying().(*types.Struct)

	var body strings.Builder

	body.WriteString("func " + appendName(e.imports.goTypeName(named)) + "(b []byte, v *" + e.declaredName(named) + ", ok *bool) []byte {\n")
	body.WriteString("\tif v == nil {\n\t\treturn append(b, \"null\"...)\n\t}\n\n")

	separator := "{"

	for i := range structType.NumFields() {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
		name := e.gen.jsMemberName(field.Name(), tag)

		body.WriteString("\tb = append(b, " + strconv.Quote(separator+jsonQuote(name)+":") + "...)\n")
		body.WriteString(e.appendJSON("v."+field.Name(), field.Type(), tagHasOption(tag, tagNotNil), 0))

		separator = ","
	}

	if separator == "{" {
		body.WriteString("\treturn append(b, \"{}\"...)\n}\n\n")
	} else {
		body.WriteString("\n\treturn append(b, '}')\n}\n\n")
	}

	return body.String(), nil
}

// jsonQuote renders a property name as a JSON string. A name is a JavaScript
// identifier, so only the characters JSON must escape are handled.
func jsonQuote(name string) string {
	var out strings.Builder

	out.WriteByte('"')

	for _, r := range name {
		switch {
		case r == '"' || r == '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case r < 0x20:
			out.WriteString(`\u00`)
			out.WriteByte("0123456789abcdef"[r>>4])
			out.WriteByte("0123456789abcdef"[r&15])
		default:
			out.WriteRune(r)
		}
	}

	out.WriteByte('"')

	return out.String()
}
