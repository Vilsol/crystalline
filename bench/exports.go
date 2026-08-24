// Package bench measures what crossing the Go/JavaScript boundary costs.
//
// Every operation is exposed twice: once through crystalline, and once through
// a hand-written syscall/js binding doing the same conversions. The generated
// numbers on their own are dominated by the wasm bridge, which crystalline does
// not control; the gap between the two pairs is what crystalline costs.
package bench

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/bench/payload"
)

//go:generate go tool crystalline -app bench -out . ./...

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(payload.Noop)
	r.Func(payload.AddInts)
	r.Func(payload.EchoString)
	r.Func(payload.EchoBytes)
	r.Func(payload.SumFloats)
	r.Func(payload.MakeInts)
	r.Func(payload.CountKeys)
	r.Func(payload.MakeMap)
	r.Func(payload.MakePoints)
	r.Func(payload.NewPoint)
	r.Func(payload.TakePoint)
	r.Func(payload.MayFail)
	r.Func(payload.Stream)
	r.Func(payload.Drain)

	r.Func(payload.Rounds, bind.AsPromise())
}
