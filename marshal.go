//go:build !js

package crystalline

import (
	"go/types"
)

// Types that cross as something other than their own structure.
//
// A struct is bound as a live view of its exported fields, which is wrong for a
// type whose value is not its fields. time.Time has no exported fields at all,
// so it used to arrive as a wrapper carrying thirty methods and no readable
// data, and its converter had nothing to validate against: a real JS Date has
// no own enumerable keys, so it was accepted and silently became the zero time.
//
// The mapping is per type and applies wherever the type appears.
type marshaller struct {
	// declared is the TypeScript type the value crosses as.
	declared string

	// toJS renders a Go expression producing the JS value. %s is the value.
	toJS string

	// fromJS renders the body of a converter taking value js.Value and
	// returning (T, error). %s is the Go type name, %q the package alias.
	fromJS func(goType string, pkg string) string
}

// builtinMarshallers are the standard library types worth mapping. Each is a
// type a JavaScript program already has a natural counterpart for.
var builtinMarshallers = map[string]marshaller{
	"time.Time": {
		declared: "Date",
		toJS:     `js.Global().Get("Date").New(float64(%s.UnixMilli()))`,
		fromJS: func(goType string, pkg string) string {
			return "\tif !value.InstanceOf(js.Global().Get(\"Date\")) {\n" +
				"\t\treturn " + goType + "{}, errors.New(\"expected a Date\")\n\t}\n\n" +
				"\treturn " + pkg + ".UnixMilli(int64(value.Call(\"getTime\").Float())).UTC(), nil\n"
		},
	},
	"time.Duration": {
		declared: "number",
		toJS:     `float64(%s) / 1e6`,
		fromJS: func(goType string, pkg string) string {
			return "\tif value.Type() != js.TypeNumber {\n" +
				"\t\treturn 0, errors.New(\"expected a number of milliseconds\")\n\t}\n\n" +
				"\treturn " + goType + "(value.Float() * 1e6), nil\n"
		},
	},
}

// marshallerFor reports the mapping for a type, if it has one.
func marshallerFor(t types.Type) (marshaller, bool) {
	obj := namedObject(t)
	if obj == nil || obj.Pkg() == nil {
		return marshaller{}, false
	}

	found, ok := builtinMarshallers[obj.Pkg().Path()+"."+obj.Name()]

	return found, ok
}

// marshallerPackage names the package a mapping's conversion needs, empty when
// it needs none.
func marshallerPackage(t types.Type) string {
	obj := namedObject(t)
	if obj == nil || obj.Pkg() == nil {
		return ""
	}

	return obj.Pkg().Path()
}
