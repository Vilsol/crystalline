// Package store declares an interface JavaScript supplies, whose methods name
// a type nothing else mentions.
package store

// Payload is reachable only through Keeper's method signatures.
type Payload struct {
	Name string
}

type Keeper interface {
	Take(p Payload) string
	Give() Payload
}
