// Package badmarshal maps one type with another type's functions.
package badmarshal

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/marshal"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// The functions describe Colour, the declaration says Reading. Nothing
	// else compares the two, so they are compared here.
	r.Type(sample.Reading{}, bind.MarshalledBy(marshal.ColourToHex, marshal.ColourFromHex))
}
