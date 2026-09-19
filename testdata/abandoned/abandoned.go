// Package abandoned exposes a type that cannot cross, so the attempt to convert
// it is thrown away after it has already named the package that spells it.
package abandoned

import (
	"github.com/Vilsol/crystalline/testdata/abandoned/extra"
	"github.com/Vilsol/crystalline/testdata/abandoned/only"
)

// Carrier itself converts. Its method does not, and the attempt to bind it
// renders the first parameter before it reaches the one it cannot.
type Carrier struct{ Name string }

func (c Carrier) Mix(t only.Token, out chan<- int) string { return c.Name }

//crystalline:export
func Carry() Carrier { return Carrier{} }

//crystalline:export
func Take(t extra.Thing) string { return t.Name }

//crystalline:export
func Fine() string { return "fine" }
