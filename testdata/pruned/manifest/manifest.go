// Package manifest excludes a method and marks a type plain, so neither
// method's types are part of the surface.
package manifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/pruned/vendorish"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Type(vendorish.Doc{}, bind.Without("Wire"))
	r.Type(vendorish.Plainish{}, bind.Plain())
}
