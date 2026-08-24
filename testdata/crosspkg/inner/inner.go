// Package inner declares a type that another package hands out.
package inner

// Payload is only ever reached through outer, but belongs to inner.
type Payload struct {
	Label string
}
