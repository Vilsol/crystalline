// Package inner declares a type that another package hands out.
package inner

// Payload is only ever reached through outer, but belongs to inner.
type Payload struct {
	Label string
}

// Mode is an enum in a package reached only through another package's
// signature, which is where the constants have to be found from.
type Mode int

const (
	ModeOff Mode = iota
	ModeOn
)
