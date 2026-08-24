// Command ledger crosses the boundary in both directions: Go reaches out to the
// browser's storage, and JavaScript reads a 64-bit identifier exactly.
//
// It is also the one example generated with -case camel, so its surface is
// spelled the way JavaScript usually is: ledger.add rather than ledger.Add.
package main

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/examples/05-ledger/ledger"
)

//go:generate go tool crystalline -app ledger -case camel -out . ./...

//crystalline:exports
func Exports(r bind.Registry) {
	// The other direction. Go declares the two methods it needs and they are
	// filled from globalThis.localStorage while the bindings start. Every one
	// is checked then, so a browser without storage fails the boot with a
	// message rather than a nil interface at the first click.
	r.Import(&ledger.Saved, bind.At("localStorage"))

	r.Func(ledger.Add)
	r.Func(ledger.Balance)
	r.Func(ledger.Forget)
}

func main() {
	select {}
}
