// Package undeclared exercises the two ways a declaration could name a type
// nothing declares.
package undeclared

import "time"

// Shape is reachable only as the element of a channel.
type Shape struct{ Side int }

// Wait returns a type that is both an enum and a mapped type. The mapping wins:
// a duration crosses as a number of milliseconds, not as one of its constants.
//
//crystalline:export
func Wait() time.Duration { return time.Second }

//crystalline:export
func Feed() <-chan Shape { return nil }
