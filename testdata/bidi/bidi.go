// Package bidi covers channels whose type does not state a direction.
package bidi

// Made returns a bidirectional channel. A returned channel is something the
// caller reads, so the direction is not in doubt.
func Made() chan string {
	out := make(chan string, 1)
	out <- "x"
	close(out)

	return out
}

// Both takes a bidirectional channel. Whether it reads or writes is not in the
// type, so there is nothing to translate it to.
func Both(values chan int) int {
	total := 0

	for value := range values {
		total += value
	}

	return total
}
