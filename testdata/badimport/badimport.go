// Package badimport imports something that is not an interface.
package badimport

import "github.com/Vilsol/crystalline/bind"

// Counter is a concrete type, which cannot describe an object JavaScript
// already has.
var Counter int

//crystalline:exports
func Exports(r bind.Registry) {
	r.Import(&Counter, bind.At("counter"))
}
