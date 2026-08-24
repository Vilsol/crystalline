// Package marshalmanifest maps a type of its own onto a JS counterpart.
package marshalmanifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/marshal"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// The two functions say everything: Colour crosses as a string, out through
	// the first and back through the second.
	r.Marshal(marshal.ColourToHex, marshal.ColourFromHex)

	r.Func(marshal.Brighten)
}
