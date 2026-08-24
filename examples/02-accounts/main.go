// Command accounts shows structs crossing the boundary: live fields, bound
// methods, fallible calls and deterministic release.
package main

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/examples/02-accounts/account"
)

//go:generate go tool crystalline -app accounts -out . ./...

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(account.Open)
	r.Func(account.Transfer)
	r.Func(account.Summarise)
	r.Func(account.ParseAmount)

	// Exported in Go, absent from JavaScript.
	r.Ignore(account.Account{}, "Audit")
}

func main() {
	select {}
}
