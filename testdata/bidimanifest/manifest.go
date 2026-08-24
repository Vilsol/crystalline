// Package bidimanifest exposes a bidirectional channel return, which is
// unambiguous.
package bidimanifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/bidi"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(bidi.Made)
}
