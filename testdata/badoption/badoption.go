// Package badoption asks for a type's option on a function.
package badoption

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// Plain says how a type crosses, and a function is not one. Accepting it
	// here would compile, generate and do nothing.
	r.Func(sample.Basic, bind.Plain())
}
