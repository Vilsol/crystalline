// Package genericmanifest exposes two instantiations of one generic type.
package genericmanifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/generic"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(generic.Strings)
	r.Func(generic.Numbers)
}
