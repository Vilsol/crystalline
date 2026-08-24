// Package badtypepromise asks for a type itself to return a promise.
package badtypepromise

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// A type does not return, so the bare form says nothing. Its methods do.
	r.Type(sample.FnSample{}, bind.AsPromise())
}
