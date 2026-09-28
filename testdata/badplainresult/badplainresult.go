// Package badplainresult names a method on a function's plain result.
package badplainresult

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	// A function has no methods to name: its own result is what is plain.
	r.Func(sample.TreeData, bind.PlainResult("Copy"))
}
