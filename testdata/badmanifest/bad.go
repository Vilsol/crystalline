// Package badmanifest declares a value under a computed name, which the
// generator cannot resolve without running the manifest.
package badmanifest

import "github.com/Vilsol/crystalline/bind"

var Prefix = "Computed"

//crystalline:exports
func Exports(r bind.Registry) {
	r.Value(Prefix+"Name", 1)
}
