// Package unbindable holds members that generated bindings can never support,
// so that the reporting of gaps can be tested without depending on whichever
// conversions happen to be implemented.
package unbindable

// Send takes a channel it writes into. JavaScript would have to supply a sink
// rather than a source, which has no counterpart, so this is unsupported by
// design rather than by omission.
func Send(values chan<- int) {
	close(values)
}

// Fine is bindable, and must still be bound when a sibling is not.
func Fine() string {
	return "ok"
}
