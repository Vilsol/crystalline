// Package manifest exposes two same-named packages without disambiguating.
package manifest

import (
	"github.com/Vilsol/crystalline/bind"
	alpha "github.com/Vilsol/crystalline/testdata/collide/alpha"
	beta "github.com/Vilsol/crystalline/testdata/collide/beta"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(alpha.Alpha)
	r.Func(beta.Beta)
}
