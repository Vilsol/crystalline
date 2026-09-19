// Package aliased exposes a type through a Go alias.
package aliased

// Shape is what the alias denotes.
type Shape struct{ Side int }

// Alias is a plain type alias. Aliases are transparent in Go's type system:
// Alias and Shape are the same type, not two types.
type Alias = Shape

//crystalline:export
func Take(a Alias) int { return a.Side }

//crystalline:export
func Make() Alias { return Alias{Side: 3} }
