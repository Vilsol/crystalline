package unbindable

// Scale takes a complex number, which has no JavaScript counterpart. The
// declarations refused it and the bindings accepted it, so the two disagreed
// about whether it could be bound at all.
func Scale(c complex128) int {
	return int(real(c))
}
