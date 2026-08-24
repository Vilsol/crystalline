// Package nobind declares a surface containing something that can never be
// bound, alongside something that can.
package nobind

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/unbindable"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(unbindable.Send)
	r.Func(unbindable.Fine)
}
