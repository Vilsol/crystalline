// Package supplied exposes a function taking an interface JavaScript supplies.
package supplied

import "github.com/Vilsol/crystalline/testdata/supplied/store"

//crystalline:export
func Use(k store.Keeper) string { return k.Take(store.Payload{}) }
