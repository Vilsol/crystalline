// Package badtypeplainresult asks for a type's result to be plain.
package badtypeplainresult

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// A type does not return, so there is no result to copy without a method.
	r.Type(sample.Tree{}, bind.PlainResult())
}
