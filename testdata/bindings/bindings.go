// Package bindings declares the surface the generated-binding tests exercise.
package bindings

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/sample"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(sample.Basic)
	r.Func(sample.FooBar)
	r.Func(sample.Rich)
	r.Func(sample.MayFail)
	r.Func(sample.OnlyFails)
	r.Func(sample.Stream)
	r.Func(sample.Cancellable)
	r.Func(sample.Sum)
	r.Func(sample.First)
}
