// Package manifest is a fixture declaring a JS surface the way a consumer
// would, including the awkward shapes real projects use.
package manifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

// Lookup is a package level value exposed as-is.
var Lookup = map[string]int{"a": 1}

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(sample.Basic)
	r.Func(sample.FooBar, bind.AsPromise())

	// A method kept off the surface, on a type declared elsewhere.
	r.Ignore(sample.FnSample{}, "Three")

	// A method promoted to a promise without touching its source.
	r.Promise(sample.FnSample{}, "Two")

	// A plain package level value.
	r.Value("Lookup", Lookup)

	// A value built by ordinary Go. Only its type is read by the generator,
	// so loops and locals need no special support.
	index := make(map[uint32]string)
	for i := uint32(0); i < 3; i++ {
		index[i] = "entry"
	}

	r.Value("Index", index, bind.InNamespace("data"))

	// A value whose type comes from another package belongs beside that
	// package, not beside the manifest that happened to declare it.
	r.Value("Prototype", sample.FooBar())

	// A conversion, which is still just an expression with a type.
	r.Value("Name", string(sample.FooBar().FirstValue), bind.InNamespace("data"))
}
