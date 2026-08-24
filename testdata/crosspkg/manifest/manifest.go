// Package manifest exposes only outer, reaching inner transitively.
package manifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/crosspkg/outer"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(outer.Make)
}
