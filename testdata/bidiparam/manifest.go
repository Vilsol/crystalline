// Package bidiparam exposes a bidirectional channel parameter, which is not.
package bidiparam

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/bidi"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(bidi.Both)
}
