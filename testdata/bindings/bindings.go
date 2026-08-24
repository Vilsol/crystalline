// Package bindings declares the surface the generated-binding tests exercise.
package bindings

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/generic"
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
	r.Func(sample.Middle)
	r.Func(sample.Ticks)
	r.Func(sample.Keys)
	r.Func(sample.NewTicker)
	r.Func(sample.Readings)
	r.Func(sample.Big)
	r.Func(sample.Streamable)
	r.Func(sample.Total)
	r.Func(sample.MakeEmbedder)

	// Two instantiations of one generic type, so that the marshallers they
	// generate are compiled and run rather than only matched as text.
	r.Func(generic.Strings)
	r.Func(generic.Numbers)

	r.Plain(sample.Reading{})

	// A map whose key names a package but whose element does not.
	r.Value("Titles", map[sample.Kind]string{1: "one"})
}
