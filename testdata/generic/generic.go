// Package generic exercises generic types, whose instantiations must stay
// distinct in the generated declarations.
package generic

// Pair carries two values of the same type.
type Pair[T any] struct {
	First  T
	Second T
}

// Swapped returns the pair the other way round.
func (p Pair[T]) Swapped() Pair[T] {
	return Pair[T]{First: p.Second, Second: p.First}
}

// Strings returns a Pair instantiated over string.
func Strings() Pair[string] {
	return Pair[string]{First: "a", Second: "b"}
}

// Numbers returns a Pair instantiated over int, which must not collide with
// the string instantiation.
func Numbers() Pair[int] {
	return Pair[int]{First: 1, Second: 2}
}
