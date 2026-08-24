package manifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/collide2/alpha"
	"github.com/Vilsol/crystalline/testdata/collide2/beta"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(alpha.Alpha)
	r.Func(beta.Beta)
}
